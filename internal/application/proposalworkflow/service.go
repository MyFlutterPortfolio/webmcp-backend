package proposalworkflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"webmcp-backend/internal/application/ports"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/proposal"
	"webmcp-backend/internal/domain/scenario"
)

var (
	ErrProposalNotFound     = errors.New("proposal not found")
	ErrProposalState        = errors.New("proposal state does not allow this operation")
	ErrProposalContext      = errors.New("proposal context mismatch")
	ErrProposalConflict     = errors.New("proposal changed before update")
	ErrProposalUnauthorized = errors.New("proposal actor is not authorized")
)

type API interface {
	CreateProposal(context.Context, CreateInput) (proposal.Proposal, error)
	ReviseProposal(context.Context, ReviseInput) (proposal.Proposal, error)
	RequestHumanReview(context.Context, ReviewInput) (proposal.Proposal, error)
}

type Service struct {
	scenarios ports.ScenarioRepository
	proposals ports.ProposalRepository
}

func NewService(scenarios ports.ScenarioRepository, proposals ports.ProposalRepository) Service {
	return Service{scenarios: scenarios, proposals: proposals}
}

type CreateInput struct {
	OrganizationID core.ID
	ActorID        core.ID
	BusinessID     core.ID
	GoalID         core.ID
	ScenarioID     core.ID
	Summary        string
	RequestID      string
}

type ReviseInput struct {
	OrganizationID core.ID
	ActorID        core.ID
	BusinessID     core.ID
	ProposalID     core.ID
	Feedback       string
	RequestID      string
}

type ReviewInput struct {
	OrganizationID core.ID
	ActorID        core.ID
	BusinessID     core.ID
	ProposalID     core.ID
	RequestID      string
}

func (s Service) CreateProposal(ctx context.Context, input CreateInput) (proposal.Proposal, error) {
	if err := validateIDs(map[string]core.ID{"organization_id": input.OrganizationID, "actor_id": input.ActorID, "business_id": input.BusinessID, "goal_id": input.GoalID, "scenario_id": input.ScenarioID}); err != nil {
		return proposal.Proposal{}, err
	}
	if strings.TrimSpace(input.Summary) == "" || len(strings.TrimSpace(input.Summary)) > 2000 {
		return proposal.Proposal{}, fmt.Errorf("summary must be between 1 and 2000 characters")
	}
	if strings.TrimSpace(input.RequestID) == "" {
		return proposal.Proposal{}, fmt.Errorf("request_id is required")
	}
	if s.scenarios == nil || s.proposals == nil {
		return proposal.Proposal{}, fmt.Errorf("proposal workflow dependencies are unavailable")
	}
	candidate, err := s.scenarios.GetScenario(ctx, input.OrganizationID, input.BusinessID, input.ScenarioID)
	if err != nil {
		return proposal.Proposal{}, err
	}
	if candidate.BusinessID != input.BusinessID || candidate.GoalID != input.GoalID || (candidate.Status != scenario.StatusSimulated && candidate.Status != scenario.StatusCompared) || !candidate.Result.Calculated {
		return proposal.Proposal{}, ErrProposalContext
	}
	proposalID, err := core.NewID("proposal")
	if err != nil {
		return proposal.Proposal{}, err
	}
	versionID, err := core.NewID("proposal-version")
	if err != nil {
		return proposal.Proposal{}, err
	}
	candidateChanges := changesForScenario(candidate)
	result := proposal.Proposal{
		ID: proposalID, BusinessID: input.BusinessID, GoalID: input.GoalID, Version: 1,
		Current: proposal.Version{ID: versionID, ProposalID: proposalID, Number: 1, ScenarioID: candidate.ID, BaseBusinessVersion: candidate.BaseBusinessVersion, Summary: strings.TrimSpace(input.Summary), Changes: candidateChanges, Status: proposal.StatusDraft},
	}
	if err := result.Validate(); err != nil {
		return proposal.Proposal{}, err
	}
	if err := s.proposals.CreateProposal(ctx, input.OrganizationID, input.ActorID, result, input.RequestID); err != nil {
		return proposal.Proposal{}, err
	}
	return result, nil
}

func (s Service) ReviseProposal(ctx context.Context, input ReviseInput) (proposal.Proposal, error) {
	if err := validateIDs(map[string]core.ID{"organization_id": input.OrganizationID, "actor_id": input.ActorID, "proposal_id": input.ProposalID}); err != nil {
		return proposal.Proposal{}, err
	}
	feedback := strings.TrimSpace(input.Feedback)
	if feedback == "" || len(feedback) > 2000 || strings.TrimSpace(input.RequestID) == "" {
		return proposal.Proposal{}, fmt.Errorf("feedback and request_id are required within their limits")
	}
	if s.proposals == nil {
		return proposal.Proposal{}, fmt.Errorf("proposal workflow dependencies are unavailable")
	}
	current, err := s.proposals.GetProposal(ctx, input.OrganizationID, input.BusinessID, input.ProposalID)
	if err != nil {
		return proposal.Proposal{}, err
	}
	if current.Current.Status != proposal.StatusDraft {
		return proposal.Proposal{}, ErrProposalState
	}
	versionID, err := core.NewID("proposal-version")
	if err != nil {
		return proposal.Proposal{}, err
	}
	current.Current.Status = proposal.StatusSuperseded
	next := current
	next.Version++
	next.Current = proposal.Version{
		ID: versionID, ProposalID: current.ID, Number: current.Current.Number + 1,
		ScenarioID: current.Current.ScenarioID, BaseBusinessVersion: current.Current.BaseBusinessVersion,
		Summary: strings.TrimSpace(current.Current.Summary + "\nRevision: " + feedback),
		Changes: append(append([]proposal.Change{}, current.Current.Changes...), proposal.Change{EntityType: "proposal", EntityID: current.ID, Field: "human_feedback", To: feedback}),
		Status:  proposal.StatusDraft,
	}
	if len(next.Current.Summary) > 5000 {
		return proposal.Proposal{}, fmt.Errorf("revised summary exceeds 5000 characters")
	}
	if err := next.Current.Validate(); err != nil {
		return proposal.Proposal{}, err
	}
	if err := s.proposals.SaveRevision(ctx, input.OrganizationID, input.ActorID, next, uint64(current.Current.Number), input.RequestID); err != nil {
		return proposal.Proposal{}, err
	}
	return next, nil
}

func (s Service) RequestHumanReview(ctx context.Context, input ReviewInput) (proposal.Proposal, error) {
	if err := validateIDs(map[string]core.ID{"organization_id": input.OrganizationID, "actor_id": input.ActorID, "business_id": input.BusinessID, "proposal_id": input.ProposalID}); err != nil {
		return proposal.Proposal{}, err
	}
	if strings.TrimSpace(input.RequestID) == "" {
		return proposal.Proposal{}, fmt.Errorf("request_id is required")
	}
	if s.proposals == nil {
		return proposal.Proposal{}, fmt.Errorf("proposal workflow dependencies are unavailable")
	}
	current, err := s.proposals.GetProposal(ctx, input.OrganizationID, input.BusinessID, input.ProposalID)
	if err != nil {
		return proposal.Proposal{}, err
	}
	if current.BusinessID != input.BusinessID || current.Current.Status != proposal.StatusDraft {
		return proposal.Proposal{}, ErrProposalState
	}
	if err := s.proposals.SubmitReview(ctx, input.OrganizationID, input.ActorID, input.ProposalID, uint64(current.Current.Number), input.RequestID); err != nil {
		return proposal.Proposal{}, err
	}
	current.Current.Status = proposal.StatusInReview
	current.Version++
	return current, nil
}

func changesForScenario(candidate scenario.Scenario) []proposal.Change {
	changes := make([]proposal.Change, 0, len(candidate.ProposedActions))
	for _, action := range candidate.ProposedActions {
		changes = append(changes, proposal.Change{EntityType: "strategic_action", EntityID: action.TargetID, Field: string(action.Type), To: fmt.Sprintf("%.4f %s", action.Value, action.Unit)})
	}
	return changes
}

func validateIDs(ids map[string]core.ID) error {
	for label, id := range ids {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	return nil
}
