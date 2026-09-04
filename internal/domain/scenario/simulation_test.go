package scenario

import (
	"reflect"
	"testing"

	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/planning"
)

func simulationFixture() (business.Snapshot, planning.Goal, Scenario) {
	snapshot := business.Snapshot{
		BusinessID: "business-1", Version: 4,
		Products: []business.Product{{ID: "product-1", SKU: "p1", Name: "Starter", PriceCents: 1000, CostCents: 400, InventoryUnits: 20, Active: true, Version: 1}},
		Orders:   []business.Order{{ID: "order-1", CustomerID: "customer-1", ProductID: "product-1", Quantity: 2, RevenueCents: 10000, CostCents: 4000, OccurredAtUnix: 1, Version: 1}},
	}
	goal := planning.Goal{ID: "goal-1", BusinessID: "business-1", Objective: "Grow safely", TimeHorizonDays: 90, Preference: planning.PreferenceBalanced, Status: planning.GoalActive, Version: 1}
	candidate := Scenario{ID: "scenario-1", GoalID: "goal-1", BusinessID: "business-1", BaseBusinessVersion: 4, Name: "Balanced growth", Objective: "Increase contribution margin", ProposedActions: []Action{
		{Type: ActionPriceAdjustment, TargetID: "product-1", Value: 10, Unit: "percent"},
		{Type: ActionMarketingBudget, Value: 1000, Unit: "cents"},
	}, Status: StatusDraft, Version: 1}
	return snapshot, goal, candidate
}

func TestSimulateIsDeterministicAndNeverChangesCanonicalSnapshot(t *testing.T) {
	snapshot, goal, candidate := simulationFixture()
	original := snapshot
	first, err := Simulate(snapshot, goal, candidate)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Simulate(snapshot, goal, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Result, second.Result) || !reflect.DeepEqual(first.Risks, second.Risks) {
		t.Fatal("same verified inputs produced different simulation output")
	}
	if !reflect.DeepEqual(snapshot, original) {
		t.Fatal("simulation mutated the canonical snapshot")
	}
	if first.Status != StatusSimulated || first.Version != 2 {
		t.Fatalf("simulation did not produce an explicit simulated revision: %#v", first)
	}
	if got := first.Result.ProjectedMetrics["revenue_cents"]; got != 13000 {
		t.Fatalf("unexpected projected revenue: %v", got)
	}
	if got := first.Result.ProjectedMetrics["margin_cents"]; got != 8000 {
		t.Fatalf("unexpected projected margin: %v", got)
	}
}

func TestSimulateSurfacesHardBudgetRisk(t *testing.T) {
	snapshot, goal, candidate := simulationFixture()
	goal.Constraints = []planning.Constraint{{ID: "constraint-1", GoalID: goal.ID, Key: "budget", Value: "500", Hard: true, CreatedBy: "human"}}
	result, err := Simulate(snapshot, goal, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Risks) != 1 || result.Risks[0].Key != "budget_exceeded" {
		t.Fatalf("expected explicit budget risk, got %#v", result.Risks)
	}
	if len(result.Result.Warnings) == 0 {
		t.Fatal("expected human-readable warning for hard constraint violation")
	}
}

func TestSimulateRejectsContextMismatchAndInvalidAction(t *testing.T) {
	snapshot, goal, candidate := simulationFixture()
	candidate.BaseBusinessVersion = 3
	if _, err := Simulate(snapshot, goal, candidate); err == nil {
		t.Fatal("expected stale scenario context to be rejected")
	}
	candidate.BaseBusinessVersion = snapshot.Version
	candidate.ProposedActions = []Action{{Type: ActionPriceAdjustment, TargetID: "missing", Value: 10, Unit: "percent"}}
	if _, err := Simulate(snapshot, goal, candidate); err == nil {
		t.Fatal("expected unknown product target to be rejected")
	}
}

func TestCompareRecommendsHighestSafeProjectedMargin(t *testing.T) {
	snapshot, goal, first := simulationFixture()
	first, err := Simulate(snapshot, goal, first)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = "scenario-2"
	second.Name = "Higher return"
	second.Status = StatusDraft
	second.Version = 1
	second.ProposedActions = []Action{
		{Type: ActionPriceAdjustment, TargetID: "product-1", Value: 20, Unit: "percent"},
		{Type: ActionMarketingBudget, Value: 2000, Unit: "cents"},
	}
	second, err = Simulate(snapshot, goal, second)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := Compare([]Scenario{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if !comparison.RecommendationAvailable || comparison.RecommendedScenarioID != second.ID {
		t.Fatalf("unexpected recommendation: %#v", comparison)
	}
	if len(comparison.Items) != 2 {
		t.Fatalf("expected two comparison items, got %d", len(comparison.Items))
	}
}

func TestCompareDoesNotRecommendHighRiskOnlyCandidates(t *testing.T) {
	snapshot, goal, first := simulationFixture()
	goal.Constraints = []planning.Constraint{{ID: "constraint-1", GoalID: goal.ID, Key: "budget", Value: "1", Hard: true, CreatedBy: "human"}}
	first, err := Simulate(snapshot, goal, first)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = "scenario-2"
	second.Status = StatusDraft
	second.Version = 1
	second.ProposedActions = []Action{{Type: ActionMarketingBudget, Value: 2000, Unit: "cents"}}
	second, err = Simulate(snapshot, goal, second)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := Compare([]Scenario{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if comparison.RecommendationAvailable || len(comparison.Warnings) == 0 {
		t.Fatalf("high-risk candidates must not receive an automatic recommendation: %#v", comparison)
	}
}
