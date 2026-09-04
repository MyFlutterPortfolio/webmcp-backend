package scenario

import "testing"

func TestScenarioMustBeSimulatedBeforeComparison(t *testing.T) {
	s := Scenario{ID: "s-1", GoalID: "g-1", BusinessID: "b-1", BaseBusinessVersion: 1, Name: "balanced", Objective: "grow", ProposedActions: []Action{{Type: ActionMarketingBudget, Value: 1000, Unit: "cents"}}, Status: StatusDraft, Version: 1}
	if _, err := s.Transition(StatusCompared); err == nil {
		t.Fatal("expected comparison before simulation to be rejected")
	}
	s, err := s.Transition(StatusSimulated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Transition(StatusCompared); err != nil {
		t.Fatalf("expected simulated scenario to compare: %v", err)
	}
}
