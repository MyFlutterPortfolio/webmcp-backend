package proposal

import (
	"fmt"
	"strings"

	"webmcp-backend/internal/domain/core"
)

type Status string

const (
	StatusDraft      Status = "draft"
	StatusInReview   Status = "in_review"
	StatusApproved   Status = "approved"
	StatusRejected   Status = "rejected"
	StatusCommitted  Status = "committed"
	StatusSuperseded Status = "superseded"
)

type Change struct {
	EntityType string  `json:"entity_type"`
	EntityID   core.ID `json:"entity_id"`
	Field      string  `json:"field"`
	From       string  `json:"from"`
	To         string  `json:"to"`
}

type Version struct {
	ID                  core.ID
	ProposalID          core.ID
	Number              int
	ScenarioID          core.ID
	BaseBusinessVersion uint64
	Summary             string
	Changes             []Change
	Status              Status
}

func (v Version) Validate() error {
	for label, id := range map[string]core.ID{"proposal_version_id": v.ID, "proposal_id": v.ProposalID, "scenario_id": v.ScenarioID} {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	if v.Number <= 0 || v.BaseBusinessVersion == 0 || strings.TrimSpace(v.Summary) == "" {
		return fmt.Errorf("proposal version number, base version and summary are required")
	}
	if !validStatus(v.Status) {
		return fmt.Errorf("invalid proposal version status %q", v.Status)
	}
	return nil
}

func (v Version) CanTransition(to Status) bool {
	switch v.Status {
	case StatusDraft:
		return to == StatusInReview || to == StatusSuperseded
	case StatusInReview:
		return to == StatusApproved || to == StatusRejected || to == StatusSuperseded
	case StatusApproved:
		return to == StatusCommitted || to == StatusSuperseded
	case StatusRejected, StatusCommitted, StatusSuperseded:
		return false
	default:
		return false
	}
}

func (v Version) Transition(to Status) (Version, error) {
	if !v.CanTransition(to) {
		return Version{}, fmt.Errorf("invalid proposal version transition %s -> %s", v.Status, to)
	}
	v.Status = to
	return v, nil
}

type Proposal struct {
	ID         core.ID
	BusinessID core.ID
	GoalID     core.ID
	Current    Version
	Version    uint64
}

func (p Proposal) Validate() error {
	for label, id := range map[string]core.ID{"proposal_id": p.ID, "business_id": p.BusinessID, "goal_id": p.GoalID} {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	if p.Version == 0 {
		return fmt.Errorf("proposal version must be positive")
	}
	return p.Current.Validate()
}

func validStatus(status Status) bool {
	return status == StatusDraft || status == StatusInReview || status == StatusApproved || status == StatusRejected || status == StatusCommitted || status == StatusSuperseded
}
