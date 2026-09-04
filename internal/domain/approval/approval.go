package approval

import (
	"fmt"
	"strings"
	"time"

	"webmcp-backend/internal/domain/core"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusExpired  Status = "expired"
	StatusRevoked  Status = "revoked"
)

type Decision struct {
	ID                core.ID
	ProposalID        core.ID
	ProposalVersionID core.ID
	BusinessID        core.ID
	DecidedBy         core.ID
	Status            Status
	Reason            string
	ExpiresAt         time.Time
}

func (d Decision) Validate(now time.Time) error {
	for label, id := range map[string]core.ID{"approval_id": d.ID, "proposal_id": d.ProposalID, "proposal_version_id": d.ProposalVersionID, "business_id": d.BusinessID, "decided_by": d.DecidedBy} {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	if d.Status != StatusPending && d.Status != StatusApproved && d.Status != StatusRejected && d.Status != StatusExpired && d.Status != StatusRevoked {
		return fmt.Errorf("invalid approval status %q", d.Status)
	}
	if d.Status == StatusApproved && strings.TrimSpace(d.Reason) == "" {
		return fmt.Errorf("approved decision requires a reason")
	}
	if !d.ExpiresAt.IsZero() && !d.ExpiresAt.After(now) && d.Status == StatusPending {
		return fmt.Errorf("pending approval is expired")
	}
	return nil
}

func (d Decision) IsValidForCommit(proposalID, proposalVersionID, businessID core.ID, now time.Time) bool {
	return d.Status == StatusApproved && d.ProposalID == proposalID && d.ProposalVersionID == proposalVersionID && d.BusinessID == businessID && (d.ExpiresAt.IsZero() || d.ExpiresAt.After(now))
}
