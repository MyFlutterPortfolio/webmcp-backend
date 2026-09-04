package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"webmcp-backend/internal/application/proposalworkflow"
	"webmcp-backend/internal/auth"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/proposal"
)

type ProposalRepository struct {
	Pool *pgxpool.Pool
}

func (r ProposalRepository) CreateProposal(ctx context.Context, organizationID, actorID core.ID, candidate proposal.Proposal, requestID string) error {
	changes, err := json.Marshal(candidate.Current.Changes)
	if err != nil {
		return fmt.Errorf("encode proposal changes: %w", err)
	}
	return WithTenantTx(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) error {
		created, err := tx.Exec(ctx, `
			INSERT INTO proposals (id, organization_id, business_id, goal_id, current_version_number)
			SELECT $1, s.organization_id, s.business_id, s.goal_id, 1
			FROM scenarios s
			JOIN users u ON u.organization_id = s.organization_id AND u.id = $2
			WHERE s.id = $3 AND s.business_id = $4 AND s.goal_id = $5
				AND (u.role IN ('owner', 'operator') OR (u.role = 'viewer' AND $6))
				AND s.status IN ('simulated', 'compared')`,
			candidate.ID, actorID, candidate.Current.ScenarioID, candidate.BusinessID, candidate.GoalID, auth.IsGuest(ctx))
		if err != nil {
			return fmt.Errorf("create proposal: %w", err)
		}
		if created.RowsAffected() != 1 {
			return proposalworkflow.ErrProposalContext
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO proposal_versions
				(id, organization_id, proposal_id, number, scenario_id, base_business_version, summary, changes, status)
			VALUES ($1, (SELECT organization_id FROM proposals WHERE id = $2), $2, $3, $4, $5, $6, $7::jsonb, $8)`,
			candidate.Current.ID, candidate.ID, candidate.Current.Number, candidate.Current.ScenarioID,
			candidate.Current.BaseBusinessVersion, candidate.Current.Summary, string(changes), candidate.Current.Status); err != nil {
			return fmt.Errorf("create proposal version: %w", err)
		}
		return appendProposalAudit(ctx, tx, candidate.ID, actorID, "proposal_created", requestID, map[string]any{"proposal_version_id": candidate.Current.ID})
	})
}

func (r ProposalRepository) GetProposal(ctx context.Context, organizationID, businessID, proposalID core.ID) (proposal.Proposal, error) {
	return WithTenantTxResult(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) (proposal.Proposal, error) {
		candidate, err := scanProposal(tx.QueryRow(ctx, `
			SELECT p.id, p.business_id, p.goal_id, p.current_version_number,
			       pv.id, pv.number, pv.scenario_id, pv.base_business_version, pv.summary, pv.changes, pv.status
			FROM proposals p
			JOIN proposal_versions pv ON pv.proposal_id = p.id AND pv.number = p.current_version_number
			WHERE p.id = $1 AND ($2 = '' OR p.business_id = $2)`, proposalID, businessID))
		if err != nil {
			if err == pgx.ErrNoRows {
				return proposal.Proposal{}, proposalworkflow.ErrProposalNotFound
			}
			return proposal.Proposal{}, fmt.Errorf("read proposal: %w", err)
		}
		return candidate, nil
	})
}

func (r ProposalRepository) SaveRevision(ctx context.Context, organizationID, actorID core.ID, next proposal.Proposal, expectedCurrentNumber uint64, requestID string) error {
	changes, err := json.Marshal(next.Current.Changes)
	if err != nil {
		return fmt.Errorf("encode revised proposal changes: %w", err)
	}
	return WithTenantTx(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) error {
		if err := requireProposalOperator(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		var currentBusinessID core.ID
		var currentNumber uint64
		if err := tx.QueryRow(ctx, `SELECT business_id, current_version_number FROM proposals WHERE id = $1 FOR UPDATE`, next.ID).Scan(&currentBusinessID, &currentNumber); err != nil {
			if err == pgx.ErrNoRows {
				return proposalworkflow.ErrProposalNotFound
			}
			return fmt.Errorf("lock proposal for revision: %w", err)
		}
		if currentNumber != expectedCurrentNumber || currentBusinessID != next.BusinessID {
			return proposalworkflow.ErrProposalConflict
		}
		updated, err := tx.Exec(ctx, `
			UPDATE proposal_versions SET status = $1
			WHERE proposal_id = $2 AND number = $3 AND status = $4`,
			proposal.StatusSuperseded, next.ID, expectedCurrentNumber, proposal.StatusDraft)
		if err != nil {
			return fmt.Errorf("supersede proposal version: %w", err)
		}
		if updated.RowsAffected() != 1 {
			return proposalworkflow.ErrProposalState
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO proposal_versions
				(id, organization_id, proposal_id, number, scenario_id, base_business_version, summary, changes, status)
			VALUES ($1, (SELECT organization_id FROM proposals WHERE id = $2), $2, $3, $4, $5, $6, $7::jsonb, $8)`,
			next.Current.ID, next.ID, next.Current.Number, next.Current.ScenarioID, next.Current.BaseBusinessVersion,
			next.Current.Summary, string(changes), next.Current.Status); err != nil {
			return fmt.Errorf("insert revised proposal version: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE proposals SET current_version_number = $1, updated_at = now() WHERE id = $2`, next.Current.Number, next.ID); err != nil {
			return fmt.Errorf("advance proposal revision: %w", err)
		}
		return appendProposalAudit(ctx, tx, next.ID, actorID, "proposal_revised", requestID, map[string]any{"proposal_version_id": next.Current.ID, "superseded_version": expectedCurrentNumber})
	})
}

func (r ProposalRepository) SubmitReview(ctx context.Context, organizationID, actorID, proposalID core.ID, expectedCurrentNumber uint64, requestID string) error {
	return WithTenantTx(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) error {
		if err := requireProposalOperator(ctx, tx, organizationID, actorID); err != nil {
			return err
		}
		updated, err := tx.Exec(ctx, `
			UPDATE proposal_versions pv SET status = $1
			FROM proposals p
			WHERE p.id = $2 AND p.id = pv.proposal_id AND pv.number = $3 AND pv.status = $4`,
			proposal.StatusInReview, proposalID, expectedCurrentNumber, proposal.StatusDraft)
		if err != nil {
			return fmt.Errorf("submit proposal review: %w", err)
		}
		if updated.RowsAffected() != 1 {
			return proposalworkflow.ErrProposalState
		}
		if _, err := tx.Exec(ctx, `UPDATE proposals SET updated_at = now() WHERE id = $1`, proposalID); err != nil {
			return fmt.Errorf("update proposal review timestamp: %w", err)
		}
		return appendProposalAudit(ctx, tx, proposalID, actorID, "proposal_submitted_for_review", requestID, map[string]any{"proposal_version_number": expectedCurrentNumber})
	})
}

func requireProposalOperator(ctx context.Context, tx pgx.Tx, organizationID, actorID core.ID) error {
	var authorized bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM users
			WHERE id = $1 AND organization_id = $2 AND (role IN ('owner', 'operator') OR (role = 'viewer' AND $3))
		)`, actorID, organizationID, auth.IsGuest(ctx)).Scan(&authorized); err != nil {
		return fmt.Errorf("check proposal actor: %w", err)
	}
	if !authorized {
		return proposalworkflow.ErrProposalUnauthorized
	}
	return nil
}

type proposalRowScanner interface {
	Scan(...any) error
}

func scanProposal(row proposalRowScanner) (proposal.Proposal, error) {
	var candidate proposal.Proposal
	var current proposal.Version
	var currentNumber int
	var changesJSON []byte
	var status string
	if err := row.Scan(&candidate.ID, &candidate.BusinessID, &candidate.GoalID, &currentNumber, &current.ID,
		&current.Number, &current.ScenarioID, &current.BaseBusinessVersion, &current.Summary, &changesJSON, &status); err != nil {
		return proposal.Proposal{}, err
	}
	current.ProposalID = candidate.ID
	current.Status = proposal.Status(status)
	if err := json.Unmarshal(changesJSON, &current.Changes); err != nil {
		return proposal.Proposal{}, fmt.Errorf("decode proposal changes: %w", err)
	}
	candidate.Current = current
	candidate.Version = uint64(currentNumber)
	if err := candidate.Validate(); err != nil {
		return proposal.Proposal{}, fmt.Errorf("invalid persisted proposal: %w", err)
	}
	return candidate, nil
}

func appendProposalAudit(ctx context.Context, tx pgx.Tx, proposalID, actorID core.ID, action, requestID string, metadata map[string]any) error {
	if strings.TrimSpace(requestID) == "" {
		return fmt.Errorf("request_id is required for proposal audit")
	}
	encoded, _ := json.Marshal(metadata)
	auditID := deterministicArtifactID("proposal-audit", core.ID(string(proposalID)+":"+action+":"+requestID))
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events
			(id, organization_id, actor_type, actor_id, action, aggregate, aggregate_id, request_id, occurred_at, metadata)
		VALUES ($1, (SELECT organization_id FROM proposals WHERE id = $2), $3, $4, $5, $6, $7, $8, now(), $9)`,
		auditID, proposalID, core.ActorHuman, actorID, action, "proposal", proposalID, requestID, encoded); err != nil {
		return fmt.Errorf("append proposal audit event: %w", err)
	}
	return nil
}
