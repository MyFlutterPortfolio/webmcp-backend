package approvalworkflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"webmcp-backend/internal/application/ports"
	"webmcp-backend/internal/domain/approval"
	"webmcp-backend/internal/domain/core"
)

var (
	ErrApprovalContext      = errors.New("approval context mismatch")
	ErrApprovalState        = errors.New("proposal is not awaiting approval")
	ErrApprovalStale        = errors.New("proposal is stale")
	ErrApprovalUnauthorized = errors.New("approval actor is not authorized")
)

type API interface {
	Decide(context.Context, DecisionInput) (approval.Decision, error)
}

type Service struct {
	repository ports.ApprovalRepository
	now        func() time.Time
}

func NewService(repository ports.ApprovalRepository, now func() time.Time) Service {
	return Service{repository: repository, now: now}
}

type DecisionInput struct {
	OrganizationID    core.ID
	ActorID           core.ID
	ProposalID        core.ID
	ProposalVersionID core.ID
	BusinessID        core.ID
	Status            approval.Status
	Reason            string
	RequestID         string
}

func (s Service) Decide(ctx context.Context, input DecisionInput) (approval.Decision, error) {
	if err := validateIDs(map[string]core.ID{
		"organization_id": input.OrganizationID, "actor_id": input.ActorID,
		"proposal_id": input.ProposalID, "proposal_version_id": input.ProposalVersionID, "business_id": input.BusinessID,
	}); err != nil {
		return approval.Decision{}, err
	}
	if input.Status != approval.StatusApproved && input.Status != approval.StatusRejected {
		return approval.Decision{}, fmt.Errorf("approval decision must be approved or rejected")
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" || len(reason) > 2000 || strings.TrimSpace(input.RequestID) == "" {
		return approval.Decision{}, fmt.Errorf("reason and request_id are required within their limits")
	}
	if s.repository == nil {
		return approval.Decision{}, fmt.Errorf("approval service dependencies are unavailable")
	}
	id, err := core.NewID("approval")
	if err != nil {
		return approval.Decision{}, err
	}
	decision := approval.Decision{
		ID: id, ProposalID: input.ProposalID, ProposalVersionID: input.ProposalVersionID,
		BusinessID: input.BusinessID, DecidedBy: input.ActorID, Status: input.Status, Reason: reason,
	}
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now().UTC()
	}
	if err := decision.Validate(now); err != nil {
		return approval.Decision{}, err
	}
	if err := s.repository.RecordDecision(ctx, input.OrganizationID, decision, input.RequestID); err != nil {
		return approval.Decision{}, err
	}
	return decision, nil
}

func validateIDs(ids map[string]core.ID) error {
	for label, id := range ids {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	return nil
}
