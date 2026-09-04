package scenarioworkflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"webmcp-backend/internal/application/ports"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
	"webmcp-backend/internal/domain/scenario"
)

var (
	ErrScenarioNotFound     = errors.New("scenario not found")
	ErrGoalNotFound         = errors.New("goal not found")
	ErrScenarioContext      = errors.New("scenario context mismatch")
	ErrScenarioConflict     = errors.New("scenario changed before update")
	ErrScenarioStale        = errors.New("scenario is based on a stale business version")
	ErrScenarioUnauthorized = errors.New("scenario actor is not authorized")
)

type API interface {
	CreateScenario(context.Context, CreateInput) (scenario.Scenario, error)
	SimulateScenario(context.Context, SimulateInput) (scenario.Scenario, error)
	CompareScenarios(context.Context, CompareInput) (scenario.Comparison, error)
}

type Service struct {
	snapshots ports.BusinessSnapshotReader
	goals     ports.GoalReader
	scenarios ports.ScenarioRepository
}

func NewService(snapshots ports.BusinessSnapshotReader, goals ports.GoalReader, scenarios ports.ScenarioRepository) Service {
	return Service{snapshots: snapshots, goals: goals, scenarios: scenarios}
}

type CreateInput struct {
	OrganizationID core.ID
	ActorID        core.ID
	BusinessID     core.ID
	GoalID         core.ID
	Name           string
	Objective      string
	Actions        []scenario.Action
	RequestID      string
}

type SimulateInput struct {
	OrganizationID core.ID
	ActorID        core.ID
	BusinessID     core.ID
	ScenarioID     core.ID
	RequestID      string
}

type CompareInput struct {
	OrganizationID core.ID
	BusinessID     core.ID
	ScenarioIDs    []core.ID
}

func (s Service) CreateScenario(ctx context.Context, input CreateInput) (scenario.Scenario, error) {
	if err := validateIDs(map[string]core.ID{"organization_id": input.OrganizationID, "actor_id": input.ActorID, "business_id": input.BusinessID, "goal_id": input.GoalID}); err != nil {
		return scenario.Scenario{}, err
	}
	if strings.TrimSpace(input.RequestID) == "" {
		return scenario.Scenario{}, fmt.Errorf("request_id is required")
	}
	if s.snapshots == nil || s.goals == nil || s.scenarios == nil {
		return scenario.Scenario{}, fmt.Errorf("scenario workflow dependencies are unavailable")
	}
	snapshot, err := s.snapshots.GetSnapshot(ctx, input.OrganizationID, input.BusinessID)
	if err != nil {
		return scenario.Scenario{}, err
	}
	goal, err := s.goals.GetGoal(ctx, input.OrganizationID, input.BusinessID, input.GoalID)
	if err != nil {
		return scenario.Scenario{}, err
	}
	if goal.BusinessID != input.BusinessID || (goal.Status != planning.GoalDraft && goal.Status != planning.GoalActive) {
		return scenario.Scenario{}, ErrScenarioContext
	}
	id, err := core.NewID("scenario")
	if err != nil {
		return scenario.Scenario{}, err
	}
	candidate := scenario.Scenario{
		ID: id, GoalID: input.GoalID, BusinessID: input.BusinessID,
		BaseBusinessVersion: snapshot.Version, Name: strings.TrimSpace(input.Name),
		Objective: strings.TrimSpace(input.Objective), ProposedActions: input.Actions,
		Status: scenario.StatusDraft, Version: 1,
	}
	if err := candidate.Validate(); err != nil {
		return scenario.Scenario{}, err
	}
	if err := s.scenarios.CreateScenario(ctx, input.OrganizationID, input.ActorID, candidate, input.RequestID); err != nil {
		return scenario.Scenario{}, err
	}
	return candidate, nil
}

func (s Service) SimulateScenario(ctx context.Context, input SimulateInput) (scenario.Scenario, error) {
	if err := validateIDs(map[string]core.ID{"organization_id": input.OrganizationID, "actor_id": input.ActorID, "business_id": input.BusinessID, "scenario_id": input.ScenarioID}); err != nil {
		return scenario.Scenario{}, err
	}
	if strings.TrimSpace(input.RequestID) == "" {
		return scenario.Scenario{}, fmt.Errorf("request_id is required")
	}
	if s.snapshots == nil || s.goals == nil || s.scenarios == nil {
		return scenario.Scenario{}, fmt.Errorf("scenario workflow dependencies are unavailable")
	}
	candidate, err := s.scenarios.GetScenario(ctx, input.OrganizationID, input.BusinessID, input.ScenarioID)
	if err != nil {
		return scenario.Scenario{}, err
	}
	goal, err := s.goals.GetGoal(ctx, input.OrganizationID, input.BusinessID, candidate.GoalID)
	if err != nil {
		return scenario.Scenario{}, err
	}
	snapshot, err := s.snapshots.GetSnapshot(ctx, input.OrganizationID, input.BusinessID)
	if err != nil {
		return scenario.Scenario{}, err
	}
	calculated, err := scenario.Simulate(snapshot, goal, candidate)
	if err != nil {
		return scenario.Scenario{}, err
	}
	if err := s.scenarios.UpdateSimulation(ctx, input.OrganizationID, calculated, candidate.Version, input.ActorID, input.RequestID); err != nil {
		return scenario.Scenario{}, err
	}
	return calculated, nil
}

func (s Service) CompareScenarios(ctx context.Context, input CompareInput) (scenario.Comparison, error) {
	if err := validateIDs(map[string]core.ID{"organization_id": input.OrganizationID, "business_id": input.BusinessID}); err != nil {
		return scenario.Comparison{}, err
	}
	if len(input.ScenarioIDs) < 2 || len(input.ScenarioIDs) > 8 {
		return scenario.Comparison{}, fmt.Errorf("between two and eight scenario IDs are required")
	}
	seen := make(map[core.ID]struct{}, len(input.ScenarioIDs))
	for _, id := range input.ScenarioIDs {
		if err := id.Validate("scenario_id"); err != nil {
			return scenario.Comparison{}, err
		}
		if _, exists := seen[id]; exists {
			return scenario.Comparison{}, fmt.Errorf("scenario IDs must be unique")
		}
		seen[id] = struct{}{}
	}
	if s.scenarios == nil {
		return scenario.Comparison{}, fmt.Errorf("scenario workflow dependencies are unavailable")
	}
	candidates, err := s.scenarios.ListScenarios(ctx, input.OrganizationID, input.BusinessID, input.ScenarioIDs)
	if err != nil {
		return scenario.Comparison{}, err
	}
	if len(candidates) != len(input.ScenarioIDs) {
		return scenario.Comparison{}, ErrScenarioNotFound
	}
	baseVersion := candidates[0].BaseBusinessVersion
	for _, candidate := range candidates {
		if candidate.BusinessID != input.BusinessID || candidate.BaseBusinessVersion != baseVersion {
			return scenario.Comparison{}, ErrScenarioContext
		}
	}
	return scenario.Compare(candidates)
}

func validateIDs(ids map[string]core.ID) error {
	for label, id := range ids {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	return nil
}
