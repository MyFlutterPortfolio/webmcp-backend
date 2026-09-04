package approvalworkflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"webmcp-backend/internal/domain/approval"
	"webmcp-backend/internal/domain/core"
)

type fakeApprovalRepository struct {
	organizationID core.ID
	decision       approval.Decision
	requestID      string
	err            error
}

func (r *fakeApprovalRepository) RecordDecision(_ context.Context, organizationID core.ID, decision approval.Decision, requestID string) error {
	r.organizationID = organizationID
	r.decision = decision
	r.requestID = requestID
	return r.err
}

func validDecisionInput() DecisionInput {
	return DecisionInput{
		OrganizationID: "org-1", ActorID: "user-1", ProposalID: "proposal-1",
		ProposalVersionID: "proposal-version-1", BusinessID: "business-1",
		Status: approval.StatusApproved, Reason: "The scenario is within the approved operating limits.", RequestID: "request-1",
	}
}

func TestServiceCreatesVersionBoundHumanDecision(t *testing.T) {
	repository := &fakeApprovalRepository{}
	service := NewService(repository, func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })

	decision, err := service.Decide(context.Background(), validDecisionInput())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Status != approval.StatusApproved || decision.ProposalID != "proposal-1" || decision.ProposalVersionID != "proposal-version-1" || decision.BusinessID != "business-1" {
		t.Fatalf("decision lost version-bound identity: %#v", decision)
	}
	if repository.organizationID != "org-1" || repository.requestID != "request-1" {
		t.Fatalf("repository contract was not preserved: %#v", repository)
	}
	if repository.decision.ID == "" || repository.decision.DecidedBy != "user-1" {
		t.Fatalf("decision actor/id was not persisted to the port: %#v", repository.decision)
	}
}

func TestServiceRequiresExplicitReasonForEveryDecision(t *testing.T) {
	repository := &fakeApprovalRepository{}
	service := NewService(repository, nil)
	input := validDecisionInput()
	input.Status = approval.StatusRejected
	input.Reason = "  "

	if _, err := service.Decide(context.Background(), input); err == nil {
		t.Fatal("expected rejected decision without reason to fail")
	}
	if repository.decision.ID != "" {
		t.Fatal("invalid decision reached the repository")
	}
}

func TestServiceRejectsUnsupportedDecisionAndPropagatesRepositoryError(t *testing.T) {
	repository := &fakeApprovalRepository{err: errors.New("transaction failed")}
	service := NewService(repository, nil)
	input := validDecisionInput()
	input.Status = approval.StatusPending
	if _, err := service.Decide(context.Background(), input); err == nil {
		t.Fatal("expected pending status to be rejected")
	}

	input = validDecisionInput()
	decisionErr := errors.New("approval transaction failed")
	repository.err = decisionErr
	if _, err := service.Decide(context.Background(), input); !errors.Is(err, decisionErr) {
		t.Fatalf("repository error = %v, want %v", err, decisionErr)
	}
}
