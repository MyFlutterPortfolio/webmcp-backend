package commit

import (
	"testing"
	"time"

	"webmcp-backend/internal/domain/approval"
	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/proposal"
)

func validCommitInput() ValidationInput {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	return ValidationInput{
		Now:        now,
		Business:   business.Snapshot{BusinessID: "b-1", Version: 4},
		Proposal:   proposal.Proposal{ID: "p-1", BusinessID: "b-1", GoalID: "g-1", Version: 1, Current: proposal.Version{ID: "pv-1", ProposalID: "p-1", Number: 1, ScenarioID: "s-1", BaseBusinessVersion: 4, Summary: "approved plan", Status: proposal.StatusApproved}},
		Approval:   approval.Decision{ID: "a-1", ProposalID: "p-1", ProposalVersionID: "pv-1", BusinessID: "b-1", DecidedBy: "human-1", Status: approval.StatusApproved, Reason: "reviewed"},
		Operation:  Operation{ID: "op-1", IdempotencyKey: "idem-1", ProposalID: "p-1", ProposalVersionID: "pv-1", BusinessID: "b-1", ExpectedBusinessVersion: 4, Status: StatusRequested},
		Authorized: true,
	}
}

func TestCommitValidationAcceptsVersionBoundApproval(t *testing.T) {
	if err := Validate(validCommitInput()); err != nil {
		t.Fatalf("expected valid commit: %v", err)
	}
}

func TestCommitRejectsStaleProposalAndForgedApproval(t *testing.T) {
	input := validCommitInput()
	input.Business.Version = 5
	if err := Validate(input); err == nil {
		t.Fatal("expected stale proposal rejection")
	}

	input = validCommitInput()
	input.Approval.ProposalVersionID = core.ID("pv-old")
	if err := Validate(input); err == nil {
		t.Fatal("expected version-bound approval rejection")
	}
}

func TestCommitRejectsUnauthorizedOperation(t *testing.T) {
	input := validCommitInput()
	input.Authorized = false
	if err := Validate(input); err == nil {
		t.Fatal("expected authorization rejection")
	}
}
