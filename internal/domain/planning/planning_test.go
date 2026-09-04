package planning

import (
	"testing"

	"webmcp-backend/internal/domain/core"
)

func TestGoalTransitionsAreExplicit(t *testing.T) {
	goal := Goal{ID: "goal-1", BusinessID: "business-1", Objective: "grow", TimeHorizonDays: 90, Preference: PreferenceBalanced, Status: GoalDraft, Version: 1}
	active, err := goal.Transition(GoalActive)
	if err != nil || active.Status != GoalActive || active.Version != 2 {
		t.Fatalf("unexpected activation: %#v, %v", active, err)
	}
	if _, err := active.Transition(GoalDraft); err == nil {
		t.Fatal("expected reverse transition to be rejected")
	}
}

func TestConstraintRequiresHumanOrAgentAttribution(t *testing.T) {
	constraint := Constraint{ID: "c-1", GoalID: "g-1", Key: "budget", Value: "1000", CreatedBy: core.ActorHuman}
	if err := constraint.Validate(); err != nil {
		t.Fatalf("expected valid constraint: %v", err)
	}
	constraint.CreatedBy = "unknown"
	if err := constraint.Validate(); err == nil {
		t.Fatal("expected invalid actor")
	}
}
