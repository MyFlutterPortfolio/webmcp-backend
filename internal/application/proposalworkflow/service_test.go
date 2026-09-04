package proposalworkflow

import (
	"context"
	"testing"

	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/proposal"
	"webmcp-backend/internal/domain/scenario"
)

type proposalScenarioReader struct {
	candidate scenario.Scenario
}

func (r proposalScenarioReader) GetScenario(context.Context, core.ID, core.ID, core.ID) (scenario.Scenario, error) {
	return r.candidate, nil
}

func (r proposalScenarioReader) CreateScenario(context.Context, core.ID, core.ID, scenario.Scenario, string) error {
	return nil
}

func (r proposalScenarioReader) ListScenarios(context.Context, core.ID, core.ID, []core.ID) ([]scenario.Scenario, error) {
	return nil, nil
}

func (r proposalScenarioReader) UpdateSimulation(context.Context, core.ID, scenario.Scenario, uint64, core.ID, string) error {
	return nil
}

type proposalRepositoryFake struct {
	current       proposal.Proposal
	created       bool
	revisionSaved bool
	reviewed      bool
}

func (r *proposalRepositoryFake) CreateProposal(context.Context, core.ID, core.ID, proposal.Proposal, string) error {
	r.created = true
	return nil
}

func (r *proposalRepositoryFake) GetProposal(context.Context, core.ID, core.ID, core.ID) (proposal.Proposal, error) {
	return r.current, nil
}

func (r *proposalRepositoryFake) SaveRevision(_ context.Context, _ core.ID, _ core.ID, next proposal.Proposal, _ uint64, _ string) error {
	r.revisionSaved = true
	r.current = next
	return nil
}

func (r *proposalRepositoryFake) SubmitReview(context.Context, core.ID, core.ID, core.ID, uint64, string) error {
	r.reviewed = true
	return nil
}

func proposalCandidate() scenario.Scenario {
	return scenario.Scenario{
		ID: "scenario-1", GoalID: "goal-1", BusinessID: "business-1", BaseBusinessVersion: 3,
		Name: "Balanced", Objective: "Grow margin", Status: scenario.StatusSimulated, Version: 2,
		ProposedActions: []scenario.Action{{Type: scenario.ActionMarketingBudget, Value: 100, Unit: "cents"}},
		Result:          scenario.Result{Calculated: true, BaselineMetrics: map[string]float64{}, ProjectedMetrics: map[string]float64{}, MetricDeltas: map[string]float64{}, Warnings: []string{}},
	}
}

func proposalDraft() proposal.Proposal {
	return proposal.Proposal{ID: "proposal-1", BusinessID: "business-1", GoalID: "goal-1", Version: 1, Current: proposal.Version{
		ID: "proposal-version-1", ProposalID: "proposal-1", Number: 1, ScenarioID: "scenario-1", BaseBusinessVersion: 3,
		Summary: "Increase margin", Changes: []proposal.Change{}, Status: proposal.StatusDraft,
	}}
}

func TestServiceCreatesProposalOnlyFromCalculatedScenario(t *testing.T) {
	repository := &proposalRepositoryFake{}
	service := NewService(proposalScenarioReader{candidate: proposalCandidate()}, repository)
	created, err := service.CreateProposal(context.Background(), CreateInput{
		OrganizationID: "org-1", ActorID: "user-1", BusinessID: "business-1", GoalID: "goal-1", ScenarioID: "scenario-1", Summary: "Increase margin", RequestID: "request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !repository.created || created.Current.Status != proposal.StatusDraft || created.Current.ScenarioID != "scenario-1" {
		t.Fatalf("proposal was not created as a reviewable draft: %#v", created)
	}
}

func TestServiceRevisionSupersedesConceptuallyAndReviewIsSeparate(t *testing.T) {
	repository := &proposalRepositoryFake{current: proposalDraft()}
	service := NewService(proposalScenarioReader{}, repository)
	revised, err := service.ReviseProposal(context.Background(), ReviseInput{OrganizationID: "org-1", ActorID: "user-1", ProposalID: "proposal-1", Feedback: "Reduce spend", RequestID: "request-2"})
	if err != nil {
		t.Fatal(err)
	}
	if !repository.revisionSaved || revised.Current.Number != 2 || revised.Current.Status != proposal.StatusDraft || len(revised.Current.Changes) != 1 {
		t.Fatalf("revision did not create a new draft version: %#v", revised)
	}

	repository.current = proposalDraft()
	reviewed, err := service.RequestHumanReview(context.Background(), ReviewInput{OrganizationID: "org-1", ActorID: "user-1", BusinessID: "business-1", ProposalID: "proposal-1", RequestID: "request-3"})
	if err != nil {
		t.Fatal(err)
	}
	if !repository.reviewed || reviewed.Current.Status != proposal.StatusInReview {
		t.Fatalf("review did not remain a separate transition: %#v", reviewed)
	}
}
