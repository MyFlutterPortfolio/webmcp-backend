package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"webmcp-backend/internal/application/scenarioworkflow"
	"webmcp-backend/internal/auth"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
	"webmcp-backend/internal/domain/scenario"
)

type ScenarioRepository struct {
	Pool *pgxpool.Pool
}

func (r ScenarioRepository) AddConstraint(ctx context.Context, organizationID, actorID core.ID, constraint planning.Constraint, requestID string) error {
	if err := constraint.Validate(); err != nil {
		return err
	}
	return WithTenantTx(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			INSERT INTO goal_constraints (id, organization_id, goal_id, key, value, hard, created_by)
			SELECT $1, g.organization_id, g.id, $4, $5, $6, u.id
			FROM goals g
			JOIN users u ON u.organization_id = g.organization_id AND u.id = $2
			WHERE g.id = $3 AND u.role IN ('owner', 'operator')
			ON CONFLICT (goal_id, key) DO UPDATE
			SET value = EXCLUDED.value, hard = EXCLUDED.hard, created_by = EXCLUDED.created_by`,
			constraint.ID, actorID, constraint.GoalID, constraint.Key, constraint.Value, constraint.Hard)
		if err != nil {
			return fmt.Errorf("save goal constraint: %w", err)
		}
		if result.RowsAffected() != 1 {
			return scenarioworkflow.ErrGoalNotFound
		}
		metadata, _ := json.Marshal(map[string]any{"goal_id": constraint.GoalID, "key": constraint.Key, "hard": constraint.Hard})
		auditID := deterministicArtifactID("goal-constraint-audit", core.ID(string(constraint.GoalID)+":"+constraint.Key+":"+requestID))
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events
				(id, organization_id, actor_type, actor_id, action, aggregate, aggregate_id, request_id, occurred_at, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), $9)`,
			auditID, organizationID, core.ActorHuman, actorID, "goal_constraint_updated", "goal", constraint.GoalID, requestID, metadata); err != nil {
			return fmt.Errorf("append constraint audit event: %w", err)
		}
		return nil
	})
}

func (r ScenarioRepository) GetGoal(ctx context.Context, organizationID, businessID, goalID core.ID) (planning.Goal, error) {
	return WithTenantTxResult(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) (planning.Goal, error) {
		var goal planning.Goal
		var status, preference string
		if err := tx.QueryRow(ctx, `
			SELECT id, business_id, objective, time_horizon_days, preference, status, version
			FROM goals WHERE id = $1 AND business_id = $2`, goalID, businessID).
			Scan(&goal.ID, &goal.BusinessID, &goal.Objective, &goal.TimeHorizonDays, &preference, &status, &goal.Version); err != nil {
			if err == pgx.ErrNoRows {
				return planning.Goal{}, scenarioworkflow.ErrGoalNotFound
			}
			return planning.Goal{}, fmt.Errorf("read goal: %w", err)
		}
		goal.Preference = planning.Preference(preference)
		goal.Status = planning.GoalStatus(status)

		rows, err := tx.Query(ctx, `
			SELECT id, key, value, hard
			FROM goal_constraints WHERE goal_id = $1 ORDER BY id`, goalID)
		if err != nil {
			return planning.Goal{}, fmt.Errorf("query goal constraints: %w", err)
		}
		for rows.Next() {
			var constraint planning.Constraint
			if err := rows.Scan(&constraint.ID, &constraint.Key, &constraint.Value, &constraint.Hard); err != nil {
				rows.Close()
				return planning.Goal{}, fmt.Errorf("scan goal constraint: %w", err)
			}
			constraint.GoalID = goalID
			constraint.CreatedBy = core.ActorHuman
			goal.Constraints = append(goal.Constraints, constraint)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return planning.Goal{}, fmt.Errorf("iterate goal constraints: %w", err)
		}
		rows.Close()
		if err := goal.Validate(); err != nil {
			return planning.Goal{}, fmt.Errorf("invalid persisted goal: %w", err)
		}
		return goal, nil
	})
}

func (r ScenarioRepository) CreateScenario(ctx context.Context, organizationID, actorID core.ID, candidate scenario.Scenario, requestID string) error {
	actions, err := json.Marshal(candidate.ProposedActions)
	if err != nil {
		return fmt.Errorf("encode scenario actions: %w", err)
	}
	return WithTenantTx(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			INSERT INTO scenarios
				(id, organization_id, business_id, goal_id, base_business_version, name, objective,
				 proposed_actions, result, risks, status, version, created_by)
			SELECT $1, b.organization_id, b.id, g.id, b.state_version, $4, $5,
				$6::jsonb, '{}'::jsonb, '[]'::jsonb, $7, 1, u.id
			FROM businesses b
			JOIN goals g ON g.organization_id = b.organization_id AND g.id = $3 AND g.business_id = b.id
			JOIN users u ON u.organization_id = b.organization_id AND u.id = $2
			WHERE b.id = $8 AND b.state_version = $9 AND g.status IN ('draft', 'active')
				AND (u.role IN ('owner', 'operator') OR (u.role = 'viewer' AND $10))`,
			candidate.ID, actorID, candidate.GoalID, candidate.Name, candidate.Objective,
			string(actions), scenario.StatusDraft, candidate.BusinessID, candidate.BaseBusinessVersion, auth.IsGuest(ctx))
		if err != nil {
			return fmt.Errorf("create scenario: %w", err)
		}
		if result.RowsAffected() != 1 {
			return scenarioworkflow.ErrScenarioContext
		}
		metadata, _ := json.Marshal(map[string]any{"base_business_version": candidate.BaseBusinessVersion, "scenario_version": candidate.Version})
		auditID := deterministicArtifactID("scenario-created-audit", core.ID(string(candidate.ID)+":"+requestID))
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events
				(id, organization_id, actor_type, actor_id, action, aggregate, aggregate_id, request_id, occurred_at, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), $9)`,
			auditID, organizationID, core.ActorHuman, actorID, "scenario_created", "scenario", candidate.ID, requestID, metadata); err != nil {
			return fmt.Errorf("append scenario creation audit event: %w", err)
		}
		return nil
	})
}

func (r ScenarioRepository) GetScenario(ctx context.Context, organizationID, businessID, scenarioID core.ID) (scenario.Scenario, error) {
	return WithTenantTxResult(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) (scenario.Scenario, error) {
		candidate, err := scanScenario(tx.QueryRow(ctx, scenarioSelect+` WHERE id = $1 AND business_id = $2`, scenarioID, businessID))
		if err != nil {
			if err == pgx.ErrNoRows {
				return scenario.Scenario{}, scenarioworkflow.ErrScenarioNotFound
			}
			return scenario.Scenario{}, fmt.Errorf("read scenario: %w", err)
		}
		return candidate, nil
	})
}

func (r ScenarioRepository) ListScenarios(ctx context.Context, organizationID, businessID core.ID, ids []core.ID) ([]scenario.Scenario, error) {
	return WithTenantTxResult(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) ([]scenario.Scenario, error) {
		if len(ids) == 0 {
			return []scenario.Scenario{}, nil
		}
		placeholders := make([]string, len(ids))
		args := make([]any, 0, len(ids)+1)
		args = append(args, businessID)
		for index, id := range ids {
			placeholders[index] = fmt.Sprintf("$%d", index+2)
			args = append(args, id)
		}
		rows, err := tx.Query(ctx, scenarioSelect+` WHERE business_id = $1 AND id IN (`+strings.Join(placeholders, ",")+") ORDER BY id", args...)
		if err != nil {
			return nil, fmt.Errorf("query scenarios: %w", err)
		}
		defer rows.Close()
		result := make([]scenario.Scenario, 0, len(ids))
		for rows.Next() {
			candidate, err := scanScenario(rows)
			if err != nil {
				return nil, fmt.Errorf("scan scenario: %w", err)
			}
			result = append(result, candidate)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate scenarios: %w", err)
		}
		return result, nil
	})
}

func (r ScenarioRepository) UpdateSimulation(ctx context.Context, organizationID core.ID, candidate scenario.Scenario, expectedVersion uint64, actorID core.ID, requestID string) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	if candidate.Status != scenario.StatusSimulated || candidate.Version != expectedVersion+1 {
		return scenarioworkflow.ErrScenarioConflict
	}
	result, err := json.Marshal(candidate.Result)
	if err != nil {
		return fmt.Errorf("encode simulation result: %w", err)
	}
	risks, err := json.Marshal(candidate.Risks)
	if err != nil {
		return fmt.Errorf("encode simulation risks: %w", err)
	}
	return WithTenantTx(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) error {
		var actorAuthorized bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM users
			WHERE id = $1 AND organization_id = $2 AND (role IN ('owner', 'operator') OR (role = 'viewer' AND $3))
			)`, actorID, organizationID, auth.IsGuest(ctx)).Scan(&actorAuthorized); err != nil {
			return fmt.Errorf("check simulation actor: %w", err)
		}
		if !actorAuthorized {
			return scenarioworkflow.ErrScenarioUnauthorized
		}

		var currentBusinessVersion uint64
		if err := tx.QueryRow(ctx, `SELECT state_version FROM businesses WHERE id = $1`, candidate.BusinessID).Scan(&currentBusinessVersion); err != nil {
			return fmt.Errorf("read simulation business version: %w", err)
		}
		if currentBusinessVersion != candidate.BaseBusinessVersion {
			return scenarioworkflow.ErrScenarioStale
		}
		updated, err := tx.Exec(ctx, `
			UPDATE scenarios
			SET result = $1::jsonb, risks = $2::jsonb, status = $3, version = $4, updated_at = now()
			WHERE id = $5 AND business_id = $6 AND status = $7 AND version = $8
				AND base_business_version = $9`,
			string(result), string(risks), candidate.Status, candidate.Version, candidate.ID,
			candidate.BusinessID, scenario.StatusDraft, expectedVersion, candidate.BaseBusinessVersion)
		if err != nil {
			return fmt.Errorf("persist simulation: %w", err)
		}
		if updated.RowsAffected() != 1 {
			return scenarioworkflow.ErrScenarioConflict
		}
		metadata, _ := json.Marshal(map[string]any{"scenario_version": candidate.Version, "base_business_version": candidate.BaseBusinessVersion})
		auditID := deterministicArtifactID("scenario-simulation-audit", core.ID(string(candidate.ID)+fmt.Sprintf(":%d", candidate.Version)))
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events
				(id, organization_id, actor_type, actor_id, action, aggregate, aggregate_id, request_id, occurred_at, metadata)
			VALUES ($1, (SELECT organization_id FROM scenarios WHERE id = $2), $3, $4, $5, $6, $7, $8, now(), $9)`,
			auditID, candidate.ID, core.ActorHuman, actorID, "scenario_simulated", "scenario", candidate.ID, requestID, metadata); err != nil {
			return fmt.Errorf("append scenario audit event: %w", err)
		}
		return nil
	})
}

const scenarioSelect = `
	SELECT id, goal_id, business_id, base_business_version, name, objective,
	       proposed_actions, result, risks, status, version
	FROM scenarios`

type rowScanner interface {
	Scan(...any) error
}

func scanScenario(row rowScanner) (scenario.Scenario, error) {
	var candidate scenario.Scenario
	var actionsJSON, resultJSON, risksJSON []byte
	var status string
	if err := row.Scan(&candidate.ID, &candidate.GoalID, &candidate.BusinessID, &candidate.BaseBusinessVersion,
		&candidate.Name, &candidate.Objective, &actionsJSON, &resultJSON, &risksJSON, &status, &candidate.Version); err != nil {
		return scenario.Scenario{}, err
	}
	candidate.Status = scenario.Status(status)
	if err := json.Unmarshal(actionsJSON, &candidate.ProposedActions); err != nil {
		return scenario.Scenario{}, fmt.Errorf("decode scenario actions: %w", err)
	}
	if len(resultJSON) > 0 && string(resultJSON) != "{}" {
		if err := json.Unmarshal(resultJSON, &candidate.Result); err != nil {
			return scenario.Scenario{}, fmt.Errorf("decode scenario result: %w", err)
		}
	}
	if len(risksJSON) > 0 && string(risksJSON) != "[]" {
		if err := json.Unmarshal(risksJSON, &candidate.Risks); err != nil {
			return scenario.Scenario{}, fmt.Errorf("decode scenario risks: %w", err)
		}
	}
	return candidate, nil
}
