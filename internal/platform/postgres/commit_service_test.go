package postgres

import (
	"testing"

	"webmcp-backend/internal/domain/core"
)

func TestValidateCommitInputRequiresOperationIdentity(t *testing.T) {
	input := CommitInput{OrganizationID: "org-1", BusinessID: "business-1", ProposalID: "proposal-1", ProposalVersionID: "version-1", ActorID: "user-1", RequestID: "request-1"}
	if err := validateCommitInput(input); err == nil {
		t.Fatal("expected missing operation and idempotency validation")
	}
	input.OperationID = core.ID("operation-1")
	input.IdempotencyKey = "operation-1"
	if err := validateCommitInput(input); err != nil {
		t.Fatalf("expected valid input: %v", err)
	}
}

func TestArtifactIDsAreDeterministicAndBounded(t *testing.T) {
	first := deterministicArtifactID("decision", "operation-1")
	second := deterministicArtifactID("decision", "operation-1")
	if first != second {
		t.Fatalf("expected stable artifact id, got %q and %q", first, second)
	}
	if len(first) > 200 {
		t.Fatalf("artifact id exceeds domain limit: %d", len(first))
	}
}
