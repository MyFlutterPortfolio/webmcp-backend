package scenario

import (
	"fmt"
	"math"
	"strings"

	"webmcp-backend/internal/domain/core"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusSimulated Status = "simulated"
	StatusCompared  Status = "compared"
	StatusArchived  Status = "archived"
)

type Assumption struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ActionType string

const (
	ActionPriceAdjustment    ActionType = "price_adjustment"
	ActionInventoryReplenish ActionType = "inventory_replenishment"
	ActionMarketingBudget    ActionType = "marketing_budget"
)

// Action is intentionally typed. Free-form agent prose is not executable
// business state and must be converted into one of these bounded actions
// before simulation or approval.
type Action struct {
	Type     ActionType `json:"type"`
	TargetID core.ID    `json:"target_id,omitempty"`
	Value    float64    `json:"value"`
	Unit     string     `json:"unit"`
}

func (a Action) Validate() error {
	switch a.Type {
	case ActionPriceAdjustment:
		if err := a.TargetID.Validate("action_target_id"); err != nil {
			return err
		}
		if a.Unit != "percent" || a.Value < -90 || a.Value > 200 {
			return fmt.Errorf("price adjustment must be between -90 and 200 percent")
		}
	case ActionInventoryReplenish:
		if err := a.TargetID.Validate("action_target_id"); err != nil {
			return err
		}
		if a.Unit != "units" || math.Trunc(a.Value) != a.Value || a.Value <= 0 || a.Value > 1000000000 {
			return fmt.Errorf("inventory replenishment must be a positive bounded unit value")
		}
	case ActionMarketingBudget:
		if a.Unit != "cents" || math.Trunc(a.Value) != a.Value || a.Value <= 0 || a.Value > 1000000000000 {
			return fmt.Errorf("marketing budget must be a positive bounded cents value")
		}
	default:
		return fmt.Errorf("invalid scenario action type %q", a.Type)
	}
	return nil
}

type Result struct {
	BaselineMetrics  map[string]float64 `json:"baseline_metrics"`
	ProjectedMetrics map[string]float64 `json:"projected_metrics"`
	MetricDeltas     map[string]float64 `json:"metric_deltas"`
	Warnings         []string           `json:"warnings"`
	Calculated       bool               `json:"calculated"`
}

type Risk struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

type Scenario struct {
	ID                  core.ID
	GoalID              core.ID
	BusinessID          core.ID
	BaseBusinessVersion uint64
	Name                string
	Objective           string
	ProposedActions     []Action
	Assumptions         []Assumption
	Result              Result
	Risks               []Risk
	Status              Status
	Version             uint64
}

func (s Scenario) Validate() error {
	for label, id := range map[string]core.ID{"scenario_id": s.ID, "goal_id": s.GoalID, "business_id": s.BusinessID} {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	if s.BaseBusinessVersion == 0 || s.Version == 0 || strings.TrimSpace(s.Name) == "" || strings.TrimSpace(s.Objective) == "" {
		return fmt.Errorf("scenario name, objective and positive versions are required")
	}
	if len(s.ProposedActions) == 0 || !validStatus(s.Status) {
		return fmt.Errorf("scenario requires actions and a valid status")
	}
	for _, action := range s.ProposedActions {
		if err := action.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (s Scenario) CanTransition(to Status) bool {
	switch s.Status {
	case StatusDraft:
		return to == StatusSimulated || to == StatusArchived
	case StatusSimulated:
		return to == StatusCompared || to == StatusArchived
	case StatusCompared:
		return to == StatusArchived
	case StatusArchived:
		return false
	default:
		return false
	}
}

func (s Scenario) Transition(to Status) (Scenario, error) {
	if !s.CanTransition(to) {
		return Scenario{}, fmt.Errorf("invalid scenario transition %s -> %s", s.Status, to)
	}
	s.Status = to
	s.Version++
	return s, nil
}

func validStatus(status Status) bool {
	return status == StatusDraft || status == StatusSimulated || status == StatusCompared || status == StatusArchived
}
