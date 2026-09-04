package commit

import (
	"fmt"
	"strings"
	"time"

	"webmcp-backend/internal/domain/approval"
	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/proposal"
)

type Status string

const (
	StatusRequested Status = "requested"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Operation struct {
	ID                      core.ID
	IdempotencyKey          string
	ProposalID              core.ID
	ProposalVersionID       core.ID
	BusinessID              core.ID
	ExpectedBusinessVersion uint64
	Status                  Status
}

func (o Operation) Validate() error {
	for label, id := range map[string]core.ID{"operation_id": o.ID, "proposal_id": o.ProposalID, "proposal_version_id": o.ProposalVersionID, "business_id": o.BusinessID} {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	if strings.TrimSpace(o.IdempotencyKey) == "" || o.ExpectedBusinessVersion == 0 {
		return fmt.Errorf("idempotency key and expected business version are required")
	}
	if o.Status != StatusRequested && o.Status != StatusSucceeded && o.Status != StatusFailed {
		return fmt.Errorf("invalid commit status %q", o.Status)
	}
	return nil
}

type ValidationInput struct {
	Now                 time.Time
	Business            business.Snapshot
	Proposal            proposal.Proposal
	Approval            approval.Decision
	Operation           Operation
	Authorized          bool
	ExistingOperationID core.ID
}

func Validate(input ValidationInput) error {
	if err := input.Business.Validate(); err != nil {
		return fmt.Errorf("business validation: %w", err)
	}
	if err := input.Proposal.Validate(); err != nil {
		return fmt.Errorf("proposal validation: %w", err)
	}
	if err := input.Approval.Validate(input.Now); err != nil {
		return fmt.Errorf("approval validation: %w", err)
	}
	if err := input.Operation.Validate(); err != nil {
		return fmt.Errorf("operation validation: %w", err)
	}
	if !input.Authorized {
		return fmt.Errorf("commit authorization denied")
	}
	if input.Proposal.BusinessID != input.Business.BusinessID || input.Proposal.Current.BaseBusinessVersion != input.Business.Version {
		return fmt.Errorf("proposal is stale or belongs to another business")
	}
	if input.Proposal.Current.Status != proposal.StatusApproved {
		return fmt.Errorf("proposal version is not approved")
	}
	if !input.Approval.IsValidForCommit(input.Proposal.ID, input.Proposal.Current.ID, input.Business.BusinessID, input.Now) {
		return fmt.Errorf("approval does not match the approved proposal version")
	}
	if input.Operation.ProposalID != input.Proposal.ID || input.Operation.ProposalVersionID != input.Proposal.Current.ID || input.Operation.BusinessID != input.Business.BusinessID {
		return fmt.Errorf("commit operation does not match proposal")
	}
	if input.Operation.ExpectedBusinessVersion != input.Business.Version {
		return fmt.Errorf("business version conflict")
	}
	if input.ExistingOperationID != "" && input.ExistingOperationID != input.Operation.ID {
		return fmt.Errorf("idempotency key already belongs to another operation")
	}
	return nil
}
