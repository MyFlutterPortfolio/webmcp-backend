package analysisworkflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
)

type snapshotReader struct{ snapshot business.Snapshot }

func (r snapshotReader) GetSnapshot(context.Context, core.ID, core.ID) (business.Snapshot, error) {
	return r.snapshot, nil
}

type goalReader struct{ goal planning.Goal }

func (r goalReader) GetGoal(context.Context, core.ID, core.ID, core.ID) (planning.Goal, error) {
	return r.goal, nil
}

func TestAnalyzeReturnsVerifiedEvidenceAndSnakeCaseGuardrails(t *testing.T) {
	service := NewService(snapshotReader{snapshot: business.Snapshot{
		BusinessID: "business-1", Version: 7,
		Products:  []business.Product{{ID: "product-1", SKU: "sku-1", Name: "Starter", PriceCents: 1000, CostCents: 400, InventoryUnits: 12, Active: true, Version: 1}},
		Customers: []business.Customer{{ID: "customer-1", Name: "Repeat buyer", Segment: "repeat", Active: true, Version: 1}},
		Orders:    []business.Order{{ID: "order-1", CustomerID: "customer-1", ProductID: "product-1", Quantity: 1, RevenueCents: 1000, CostCents: 400, Version: 1}},
	}}, goalReader{goal: planning.Goal{
		ID: "goal-1", BusinessID: "business-1", Objective: "Grow repeat revenue", TimeHorizonDays: 90,
		Preference: planning.PreferenceBalanced, Status: planning.GoalActive, Version: 1,
		Constraints: []planning.Constraint{{ID: "constraint-1", GoalID: "goal-1", Key: "budget", Value: "8000", Hard: true, CreatedBy: core.ActorHuman}},
	}})
	result, err := service.Analyze(context.Background(), Input{OrganizationID: "org-1", ActorID: "user-1", BusinessID: "business-1", GoalID: "goal-1", Focus: "retention"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if result.BusinessVersion != 7 || len(result.Evidence) == 0 || len(result.Signals) == 0 || result.Guardrails[0].Key != "budget" {
		t.Fatalf("unexpected analysis result: %#v", result)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if !strings.Contains(string(payload), `"goal_id":"goal-1"`) || strings.Contains(string(payload), `"GoalID"`) {
		t.Fatalf("analysis contract is not stable JSON: %s", payload)
	}
}

func TestAnalyzeRejectsUnboundedFocus(t *testing.T) {
	service := NewService(snapshotReader{}, goalReader{})
	_, err := service.Analyze(context.Background(), Input{OrganizationID: "org-1", ActorID: "user-1", BusinessID: "business-1", GoalID: "goal-1", Focus: strings.Repeat("x", 1001)})
	if err == nil {
		t.Fatal("expected oversized focus to be rejected")
	}
}
