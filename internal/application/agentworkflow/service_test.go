package agentworkflow

import (
	"context"
	"errors"
	"testing"

	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
)

type testSnapshotReader struct{ snapshot business.Snapshot }

func (r testSnapshotReader) GetSnapshot(context.Context, core.ID, core.ID) (business.Snapshot, error) {
	return r.snapshot, nil
}

type testGoalReader struct{ goal planning.Goal }

func (r testGoalReader) GetGoal(context.Context, core.ID, core.ID, core.ID) (planning.Goal, error) {
	return r.goal, nil
}

type testProvider struct {
	response ProviderResponse
	err      error
}

func (p testProvider) Generate(context.Context, Prompt) (ProviderResponse, error) {
	return p.response, p.err
}

func testChatContext() (business.Snapshot, planning.Goal) {
	return business.Snapshot{
			BusinessID: "business-1", Version: 7,
			Products:  []business.Product{{ID: "product-1", SKU: "sku-1", Name: "Starter", PriceCents: 1200, CostCents: 400, InventoryUnits: 8, Active: true, Version: 1}},
			Customers: []business.Customer{{ID: "customer-1", Name: "Hidden name", Segment: "repeat", Active: true, Version: 1}},
			Orders:    []business.Order{{ID: "order-1", CustomerID: "customer-1", ProductID: "product-1", Quantity: 2, RevenueCents: 2400, CostCents: 800, Version: 1}},
			Metrics:   []business.Metric{{Key: "conversion_rate", Value: 4.2, Unit: "percent", Period: "30d", Source: "analytics", Version: 1}},
		}, planning.Goal{
			ID: "goal-1", BusinessID: "business-1", Objective: "Grow repeat revenue safely", TimeHorizonDays: 90,
			Preference: planning.PreferenceBalanced, Status: planning.GoalActive, Version: 3,
			Constraints: []planning.Constraint{{ID: "constraint-1", GoalID: "goal-1", Key: "budget", Value: "8000 cents", Hard: true, CreatedBy: core.ActorHuman}},
		}
}

func testService(primary, fallback Provider) Service {
	snapshot, goal := testChatContext()
	return NewService(testSnapshotReader{snapshot: snapshot}, testGoalReader{goal: goal}, primary, fallback)
}

func testInput(message, stage string) Input {
	return Input{OrganizationID: "org-1", ActorID: "user-1", BusinessID: "business-1", GoalID: "goal-1", Message: message, CurrentStage: stage}
}

func TestChatGroundsDeterministicFallbackAndSuggestsTypedCapability(t *testing.T) {
	result, err := testService(nil, DeterministicProvider{}).Chat(context.Background(), testInput("Analyze the safest growth opportunity", "investigate"))
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if result.Provider != "deterministic_fallback" || result.Grounded.BusinessVersion != 7 {
		t.Fatalf("unexpected grounding result: %+v", result)
	}
	if result.ToolSuggestion == nil || result.ToolSuggestion.Name != "analyze_business" {
		t.Fatalf("expected typed analysis suggestion, got %+v", result.ToolSuggestion)
	}
	if result.ToolSuggestion.Arguments["focus"] != "Analyze the safest growth opportunity" {
		t.Fatalf("focus was not preserved as typed argument: %+v", result.ToolSuggestion.Arguments)
	}
	if result.Message == "" || len(result.Warnings) == 0 {
		t.Fatal("safe response metadata is missing")
	}
}

func TestChatWithholdsCommitSuggestion(t *testing.T) {
	primary := testProvider{response: ProviderResponse{Message: "Commit it now.", ToolSuggestion: &ToolSuggestion{Name: "commit_approved_proposal", Arguments: map[string]any{"proposal_version_id": "proposal-version-1"}}}}
	input := testInput("commit the proposal", "approved")
	input.AvailableTools = []string{"commit_approved_proposal", "verify_committed_result"}
	result, err := testService(primary, DeterministicProvider{}).Chat(context.Background(), input)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if result.ToolSuggestion != nil {
		t.Fatal("chat exposed a consequential commit suggestion")
	}
	if len(result.Warnings) < 2 {
		t.Fatalf("expected withheld suggestion warning, got %+v", result.Warnings)
	}
}

func TestChatRejectsOversizedOrInvalidContext(t *testing.T) {
	input := testInput("hello", "orient")
	input.RecentMessages = make([]ConversationTurn, 9)
	_, err := testService(nil, DeterministicProvider{}).Chat(context.Background(), input)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}
