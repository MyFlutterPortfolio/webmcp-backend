package scenarioworkflow

import (
	"context"
	"testing"

	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
	"webmcp-backend/internal/domain/scenario"
)

type workflowSnapshotReader struct{ snapshot business.Snapshot }

func (r workflowSnapshotReader) GetSnapshot(context.Context, core.ID, core.ID) (business.Snapshot, error) {
	return r.snapshot, nil
}

type workflowScenarioRepository struct {
	goal      planning.Goal
	scenario  scenario.Scenario
	scenarios []scenario.Scenario
	created   scenario.Scenario
	updated   scenario.Scenario
}

func (r *workflowScenarioRepository) GetGoal(context.Context, core.ID, core.ID, core.ID) (planning.Goal, error) {
	return r.goal, nil
}

func (r *workflowScenarioRepository) CreateScenario(_ context.Context, _ core.ID, _ core.ID, candidate scenario.Scenario, _ string) error {
	r.created = candidate
	return nil
}

func (r *workflowScenarioRepository) GetScenario(context.Context, core.ID, core.ID, core.ID) (scenario.Scenario, error) {
	return r.scenario, nil
}

func (r *workflowScenarioRepository) ListScenarios(context.Context, core.ID, core.ID, []core.ID) ([]scenario.Scenario, error) {
	return r.scenarios, nil
}

func (r *workflowScenarioRepository) UpdateSimulation(_ context.Context, _ core.ID, candidate scenario.Scenario, _ uint64, _ core.ID, _ string) error {
	r.updated = candidate
	return nil
}

func workflowFixture() (business.Snapshot, planning.Goal, *workflowScenarioRepository) {
	snapshot := business.Snapshot{
		BusinessID: "business-1", Version: 2,
		Products: []business.Product{{ID: "product-1", SKU: "p1", Name: "Starter", PriceCents: 1000, CostCents: 400, InventoryUnits: 10, Active: true, Version: 1}},
		Orders:   []business.Order{{ID: "order-1", CustomerID: "customer-1", ProductID: "product-1", Quantity: 1, RevenueCents: 1000, CostCents: 400, OccurredAtUnix: 1, Version: 1}},
	}
	goal := planning.Goal{ID: "goal-1", BusinessID: "business-1", Objective: "Grow", TimeHorizonDays: 90, Preference: planning.PreferenceBalanced, Status: planning.GoalActive, Version: 1}
	repository := &workflowScenarioRepository{goal: goal}
	return snapshot, goal, repository
}

func TestServiceCreatesScenarioPinnedToCurrentSnapshot(t *testing.T) {
	snapshot, goal, repository := workflowFixture()
	service := NewService(workflowSnapshotReader{snapshot: snapshot}, repository, repository)
	candidate, err := service.CreateScenario(context.Background(), CreateInput{
		OrganizationID: "org-1", ActorID: "user-1", BusinessID: snapshot.BusinessID, GoalID: goal.ID,
		Name: "Balanced", Objective: "Improve margin", Actions: []scenario.Action{{Type: scenario.ActionMarketingBudget, Value: 100, Unit: "cents"}},
		RequestID: "request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.created.ID == "" || candidate.ID != repository.created.ID || candidate.BaseBusinessVersion != snapshot.Version {
		t.Fatalf("scenario was not pinned and persisted correctly: %#v", repository.created)
	}
}

func TestServiceSimulationPersistsOnlyCalculatedRevision(t *testing.T) {
	snapshot, goal, repository := workflowFixture()
	repository.scenario = scenario.Scenario{
		ID: "scenario-1", GoalID: goal.ID, BusinessID: snapshot.BusinessID, BaseBusinessVersion: snapshot.Version,
		Name: "Balanced", Objective: "Improve margin",
		ProposedActions: []scenario.Action{{Type: scenario.ActionPriceAdjustment, TargetID: "product-1", Value: 10, Unit: "percent"}},
		Status:          scenario.StatusDraft, Version: 1,
	}
	service := NewService(workflowSnapshotReader{snapshot: snapshot}, repository, repository)
	result, err := service.SimulateScenario(context.Background(), SimulateInput{
		OrganizationID: "org-1", ActorID: "user-1", BusinessID: snapshot.BusinessID, ScenarioID: repository.scenario.ID, RequestID: "request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != scenario.StatusSimulated || !result.Result.Calculated || repository.updated.Status != scenario.StatusSimulated {
		t.Fatalf("calculated revision was not persisted: %#v", repository.updated)
	}
}
