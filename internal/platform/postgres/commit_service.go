package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"webmcp-backend/internal/application/ports"
	"webmcp-backend/internal/domain/approval"
	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/commit"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/proposal"
)

var (
	ErrCommitInProgress    = errors.New("commit is already in progress")
	ErrIdempotencyConflict = errors.New("idempotency key belongs to another operation")
	ErrBusinessNotFound    = errors.New("business not found")
	ErrProposalNotFound    = errors.New("approved proposal version not found")
	ErrApprovalNotFound    = errors.New("matching approval not found")
	ErrCommitUnauthorized  = errors.New("commit actor is not authorized")
)

type CommitInput = ports.CommitInput
type CommitOutcome = ports.CommitOutcome

type CommitService struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s CommitService) Commit(ctx context.Context, input CommitInput) (CommitOutcome, error) {
	if err := validateCommitInput(input); err != nil {
		return CommitOutcome{}, err
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	return WithTenantTxResult(ctx, s.Pool, input.OrganizationID, func(ctx context.Context, tx pgx.Tx) (CommitOutcome, error) {
		return s.commitTx(ctx, tx, input, now)
	})
}

func (s CommitService) commitTx(ctx context.Context, tx pgx.Tx, input CommitInput, now time.Time) (CommitOutcome, error) {
	decisionID := deterministicArtifactID("decision", input.OperationID)
	auditID := deterministicArtifactID("audit", input.OperationID)
	var existing struct {
		id, idempotencyKey, proposalID, proposalVersionID, businessID string
		status                                                        string
		committedVersion                                              *int64
	}
	err := tx.QueryRow(ctx, `
		SELECT id, idempotency_key, proposal_id, proposal_version_id, business_id, status, committed_business_version
		FROM commit_operations WHERE idempotency_key = $1 FOR UPDATE`, input.IdempotencyKey).
		Scan(&existing.id, &existing.idempotencyKey, &existing.proposalID, &existing.proposalVersionID, &existing.businessID, &existing.status, &existing.committedVersion)
	if err == nil {
		if existing.id != string(input.OperationID) || existing.proposalID != string(input.ProposalID) || existing.proposalVersionID != string(input.ProposalVersionID) || existing.businessID != string(input.BusinessID) {
			return CommitOutcome{}, ErrIdempotencyConflict
		}
		switch commit.Status(existing.status) {
		case commit.StatusSucceeded:
			return CommitOutcome{OperationID: input.OperationID, DecisionID: decisionID, Status: commit.StatusSucceeded, CommittedBusinessVersion: int64ToUint64(existing.committedVersion), Replayed: true}, nil
		case commit.StatusRequested:
			return CommitOutcome{}, ErrCommitInProgress
		case commit.StatusFailed:
			return CommitOutcome{OperationID: input.OperationID, Status: commit.StatusFailed, Replayed: true}, nil
		default:
			return CommitOutcome{}, fmt.Errorf("unknown persisted commit status %q", existing.status)
		}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CommitOutcome{}, fmt.Errorf("lock idempotency record: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO commit_operations (id, organization_id, business_id, proposal_id, proposal_version_id, idempotency_key, expected_business_version, status)
		SELECT $1, organization_id, $2, $3, $4, $5, state_version, $6
		FROM businesses WHERE id = $2`, input.OperationID, input.BusinessID, input.ProposalID, input.ProposalVersionID, input.IdempotencyKey, commit.StatusRequested); err != nil {
		if isUniqueViolation(err) {
			return CommitOutcome{}, ErrIdempotencyConflict
		}
		return CommitOutcome{}, fmt.Errorf("create commit operation: %w", err)
	}

	var currentVersion int64
	if err := tx.QueryRow(ctx, `SELECT state_version FROM businesses WHERE id = $1 FOR UPDATE`, input.BusinessID).Scan(&currentVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommitOutcome{}, ErrBusinessNotFound
		}
		return CommitOutcome{}, fmt.Errorf("lock business: %w", err)
	}
	var actorRole string
	if err := tx.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, input.ActorID).Scan(&actorRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommitOutcome{}, ErrCommitUnauthorized
		}
		return CommitOutcome{}, fmt.Errorf("read commit actor: %w", err)
	}
	if actorRole != "owner" && actorRole != "operator" {
		return CommitOutcome{}, ErrCommitUnauthorized
	}

	var proposalID, proposalBusinessID, proposalVersionID, summary, proposalStatus string
	var proposalVersionNumber int
	var baseVersion int64
	err = tx.QueryRow(ctx, `
		SELECT p.id, p.business_id, pv.id, pv.number, pv.base_business_version, pv.summary, pv.status
		FROM proposals p
		JOIN proposal_versions pv ON pv.proposal_id = p.id AND pv.number = p.current_version_number
		WHERE p.id = $1 AND pv.id = $2 AND p.business_id = $3
		FOR UPDATE`, input.ProposalID, input.ProposalVersionID, input.BusinessID).
		Scan(&proposalID, &proposalBusinessID, &proposalVersionID, &proposalVersionNumber, &baseVersion, &summary, &proposalStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return CommitOutcome{}, ErrProposalNotFound
	}
	if err != nil {
		return CommitOutcome{}, fmt.Errorf("read proposal: %w", err)
	}

	var approvalID, approvalProposalID, approvalVersionID, approvalBusinessID, decidedBy, approvalStatus, reason string
	var expiresAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT a.id, a.proposal_id, a.proposal_version_id, a.business_id, a.decided_by, a.status, a.reason, a.expires_at
		FROM approvals a
		JOIN users u ON u.id = a.decided_by AND u.organization_id = a.organization_id
		WHERE a.proposal_version_id = $1 AND a.proposal_id = $2 AND a.business_id = $3 AND a.status = $4
		  AND u.role IN ('owner', 'operator')
		ORDER BY a.created_at DESC LIMIT 1 FOR UPDATE`, input.ProposalVersionID, input.ProposalID, input.BusinessID, approval.StatusApproved).
		Scan(&approvalID, &approvalProposalID, &approvalVersionID, &approvalBusinessID, &decidedBy, &approvalStatus, &reason, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CommitOutcome{}, ErrApprovalNotFound
	}
	if err != nil {
		return CommitOutcome{}, fmt.Errorf("read approval: %w", err)
	}

	proposalAggregate := proposal.Proposal{
		ID: core.ID(proposalID), BusinessID: core.ID(proposalBusinessID), Version: 1,
		Current: proposal.Version{ID: core.ID(proposalVersionID), ProposalID: core.ID(proposalID), Number: proposalVersionNumber, BaseBusinessVersion: uint64(baseVersion), Summary: summary, Status: proposal.Status(proposalStatus)},
	}
	approvalAggregate := approval.Decision{
		ID: core.ID(approvalID), ProposalID: core.ID(approvalProposalID), ProposalVersionID: core.ID(approvalVersionID), BusinessID: core.ID(approvalBusinessID), DecidedBy: core.ID(decidedBy), Status: approval.Status(approvalStatus), Reason: reason,
	}
	if expiresAt != nil {
		approvalAggregate.ExpiresAt = expiresAt.UTC()
	}
	operation := commit.Operation{ID: input.OperationID, IdempotencyKey: input.IdempotencyKey, ProposalID: input.ProposalID, ProposalVersionID: input.ProposalVersionID, BusinessID: input.BusinessID, ExpectedBusinessVersion: uint64(currentVersion), Status: commit.StatusRequested}
	businessSnapshot := business.Snapshot{BusinessID: input.BusinessID, Version: uint64(currentVersion)}
	if err := commit.Validate(commit.ValidationInput{Now: now, Business: businessSnapshot, Proposal: proposalAggregate, Approval: approvalAggregate, Operation: operation, Authorized: input.Authorized}); err != nil {
		return CommitOutcome{}, err
	}

	newVersion := currentVersion + 1
	if _, err := tx.Exec(ctx, `
		INSERT INTO committed_decisions (id, organization_id, business_id, proposal_id, proposal_version_id, committed_business_version, summary)
		VALUES ($1, (SELECT organization_id FROM businesses WHERE id = $2), $2, $3, $4, $5, $6)`, decisionID, input.BusinessID, input.ProposalID, input.ProposalVersionID, newVersion, summary); err != nil {
		return CommitOutcome{}, fmt.Errorf("persist committed decision: %w", err)
	}
	if tag, err := tx.Exec(ctx, `UPDATE businesses SET state_version = $1 WHERE id = $2 AND state_version = $3`, newVersion, input.BusinessID, currentVersion); err != nil || tag.RowsAffected() != 1 {
		if err != nil {
			return CommitOutcome{}, fmt.Errorf("increment business version: %w", err)
		}
		return CommitOutcome{}, fmt.Errorf("business version changed during commit")
	}
	if _, err := tx.Exec(ctx, `UPDATE proposal_versions SET status = $1 WHERE id = $2 AND status = $3`, proposal.StatusCommitted, input.ProposalVersionID, proposal.StatusApproved); err != nil {
		return CommitOutcome{}, fmt.Errorf("mark proposal committed: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE commit_operations SET status = $1, committed_business_version = $2, completed_at = $3 WHERE id = $4`, commit.StatusSucceeded, newVersion, now, input.OperationID); err != nil {
		return CommitOutcome{}, fmt.Errorf("complete commit operation: %w", err)
	}
	metadata, _ := json.Marshal(map[string]string{"proposal_id": string(input.ProposalID), "proposal_version_id": string(input.ProposalVersionID)})
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (id, organization_id, actor_type, actor_id, action, aggregate, aggregate_id, request_id, operation_id, occurred_at, metadata)
		VALUES ($1, (SELECT organization_id FROM businesses WHERE id = $2), $3, $4, $5, $6, $7, $8, $9, $10, $11)`, auditID, input.BusinessID, core.ActorHuman, input.ActorID, "commit_executed", "business", input.BusinessID, input.RequestID, input.OperationID, now, metadata); err != nil {
		return CommitOutcome{}, fmt.Errorf("append commit audit event: %w", err)
	}
	return CommitOutcome{OperationID: input.OperationID, DecisionID: decisionID, Status: commit.StatusSucceeded, CommittedBusinessVersion: uint64(newVersion)}, nil
}

func validateCommitInput(input CommitInput) error {
	for label, id := range map[string]core.ID{"organization_id": input.OrganizationID, "operation_id": input.OperationID, "business_id": input.BusinessID, "proposal_id": input.ProposalID, "proposal_version_id": input.ProposalVersionID, "actor_id": input.ActorID} {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.RequestID) == "" {
		return fmt.Errorf("idempotency key and request id are required")
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func int64ToUint64(value *int64) uint64 {
	if value == nil || *value < 0 {
		return 0
	}
	return uint64(*value)
}

func deterministicArtifactID(prefix string, operationID core.ID) core.ID {
	digest := sha256.Sum256([]byte(operationID))
	return core.ID(prefix + "-" + hex.EncodeToString(digest[:16]))
}
