package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"webmcp-backend/internal/application/approvalworkflow"
	"webmcp-backend/internal/domain/approval"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/proposal"
)

// ApprovalRepository persists a single human decision together with the
// proposal transition and audit event. The transaction is deliberately kept
// here, next to PostgreSQL row locking, so no caller can observe a decision
// without the corresponding proposal state transition.
type ApprovalRepository struct {
	Pool *pgxpool.Pool
}

func (r ApprovalRepository) RecordDecision(ctx context.Context, organizationID core.ID, decision approval.Decision, requestID string) error {
	return WithTenantTx(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) error {
		var currentBusinessVersion int64
		if err := tx.QueryRow(ctx, `
			SELECT state_version
			FROM businesses
			WHERE id = $1
			FOR UPDATE`, decision.BusinessID).Scan(&currentBusinessVersion); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return approvalworkflow.ErrApprovalContext
			}
			return fmt.Errorf("lock approval business: %w", err)
		}
		if currentBusinessVersion <= 0 {
			return fmt.Errorf("invalid current business version %d", currentBusinessVersion)
		}

		var actorRole string
		if err := tx.QueryRow(ctx, `
			SELECT role
			FROM users
			WHERE id = $1 AND organization_id = $2`, decision.DecidedBy, organizationID).Scan(&actorRole); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return approvalworkflow.ErrApprovalUnauthorized
			}
			return fmt.Errorf("read approval actor: %w", err)
		}
		if actorRole != "owner" && actorRole != "operator" {
			return approvalworkflow.ErrApprovalUnauthorized
		}

		var proposalID, businessID, versionID, versionStatus string
		var baseBusinessVersion int64
		if err := tx.QueryRow(ctx, `
			SELECT p.id, p.business_id, pv.id, pv.base_business_version, pv.status
			FROM proposals p
			JOIN proposal_versions pv
			  ON pv.proposal_id = p.id
			 AND pv.number = p.current_version_number
			WHERE p.id = $1
			  AND p.business_id = $2
			  AND pv.id = $3
			FOR UPDATE`, decision.ProposalID, decision.BusinessID, decision.ProposalVersionID).
			Scan(&proposalID, &businessID, &versionID, &baseBusinessVersion, &versionStatus); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return approvalworkflow.ErrApprovalContext
			}
			return fmt.Errorf("lock proposal for approval: %w", err)
		}
		if proposal.Status(versionStatus) != proposal.StatusInReview {
			return approvalworkflow.ErrApprovalState
		}
		if baseBusinessVersion <= 0 || uint64(baseBusinessVersion) != uint64(currentBusinessVersion) {
			return approvalworkflow.ErrApprovalStale
		}

		var expiresAt *time.Time
		if !decision.ExpiresAt.IsZero() {
			expiresAt = &decision.ExpiresAt
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO approvals
				(id, organization_id, business_id, proposal_id, proposal_version_id, decided_by, status, reason, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			decision.ID, organizationID, businessID, proposalID, versionID, decision.DecidedBy,
			decision.Status, decision.Reason, expiresAt); err != nil {
			return fmt.Errorf("persist approval decision: %w", err)
		}

		updated, err := tx.Exec(ctx, `
			UPDATE proposal_versions
			SET status = $1
			WHERE id = $2 AND proposal_id = $3 AND status = $4`,
			proposal.Status(decision.Status), versionID, proposalID, proposal.StatusInReview)
		if err != nil {
			return fmt.Errorf("advance proposal approval state: %w", err)
		}
		if updated.RowsAffected() != 1 {
			return approvalworkflow.ErrApprovalState
		}
		if _, err := tx.Exec(ctx, `
			UPDATE proposals
			SET updated_at = now()
			WHERE id = $1`, proposalID); err != nil {
			return fmt.Errorf("update proposal approval timestamp: %w", err)
		}

		action := "proposal_rejected"
		if decision.Status == approval.StatusApproved {
			action = "proposal_approved"
		}
		metadata, err := json.Marshal(map[string]any{
			"business_id":         decision.BusinessID,
			"proposal_version_id": decision.ProposalVersionID,
			"decision":            decision.Status,
		})
		if err != nil {
			return fmt.Errorf("encode approval audit metadata: %w", err)
		}
		auditID := deterministicArtifactID("approval-audit", core.ID(string(decision.ProposalID)+":"+string(decision.ProposalVersionID)+":"+string(decision.DecidedBy)+":"+action+":"+requestID))
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events
				(id, organization_id, actor_type, actor_id, action, aggregate, aggregate_id, request_id, occurred_at, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), $9)`,
			auditID, organizationID, core.ActorHuman, decision.DecidedBy, action, "proposal", decision.ProposalID, requestID, metadata); err != nil {
			return fmt.Errorf("append approval audit event: %w", err)
		}
		return nil
	})
}
