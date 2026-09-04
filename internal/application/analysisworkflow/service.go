package analysisworkflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"webmcp-backend/internal/application/ports"
	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
)

var (
	ErrAnalysisNotFound = errors.New("analysis context was not found")
)

type API interface {
	Analyze(context.Context, Input) (Result, error)
}

type Service struct {
	snapshots ports.BusinessSnapshotReader
	goals     ports.GoalReader
}

type Input struct {
	OrganizationID core.ID
	ActorID        core.ID
	BusinessID     core.ID
	GoalID         core.ID
	Focus          string
}

type Signal struct {
	Key         string  `json:"key"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
}

type Evidence struct {
	Source string  `json:"source"`
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
}

type Tradeoff struct {
	Key         string `json:"key"`
	Dimension   string `json:"dimension"`
	Description string `json:"description"`
}

type Constraint struct {
	ID        core.ID        `json:"id"`
	GoalID    core.ID        `json:"goal_id"`
	Key       string         `json:"key"`
	Value     string         `json:"value"`
	Hard      bool           `json:"hard"`
	CreatedBy core.ActorType `json:"created_by"`
}

type Result struct {
	BusinessID      core.ID      `json:"business_id"`
	GoalID          core.ID      `json:"goal_id"`
	BusinessVersion uint64       `json:"business_version"`
	Focus           string       `json:"focus"`
	Signals         []Signal     `json:"signals"`
	Evidence        []Evidence   `json:"evidence"`
	Tradeoffs       []Tradeoff   `json:"tradeoffs"`
	Guardrails      []Constraint `json:"guardrails"`
}

func NewService(snapshots ports.BusinessSnapshotReader, goals ports.GoalReader) Service {
	return Service{snapshots: snapshots, goals: goals}
}

func (s Service) Analyze(ctx context.Context, input Input) (Result, error) {
	for label, id := range map[string]core.ID{"organization_id": input.OrganizationID, "actor_id": input.ActorID, "business_id": input.BusinessID, "goal_id": input.GoalID} {
		if err := id.Validate(label); err != nil {
			return Result{}, err
		}
	}
	focus := strings.TrimSpace(input.Focus)
	if focus == "" || len(focus) > 1000 {
		return Result{}, fmt.Errorf("focus must be between 1 and 1000 characters")
	}
	if s.snapshots == nil || s.goals == nil {
		return Result{}, fmt.Errorf("analysis workflow dependencies are unavailable")
	}
	snapshot, err := s.snapshots.GetSnapshot(ctx, input.OrganizationID, input.BusinessID)
	if err != nil {
		return Result{}, err
	}
	goal, err := s.goals.GetGoal(ctx, input.OrganizationID, input.BusinessID, input.GoalID)
	if err != nil {
		return Result{}, err
	}
	if goal.BusinessID != input.BusinessID {
		return Result{}, ErrAnalysisNotFound
	}
	return buildResult(snapshot, goal, focus), nil
}

func buildResult(snapshot business.Snapshot, goal planning.Goal, focus string) Result {
	activeProducts := 0
	inventoryUnits := int64(0)
	for _, product := range snapshot.Products {
		if product.Active {
			activeProducts++
		}
		inventoryUnits += product.InventoryUnits
	}
	revenue, cost := int64(0), int64(0)
	for _, order := range snapshot.Orders {
		revenue += order.RevenueCents
		cost += order.CostCents
	}
	margin := revenue - cost
	marginRate := 0.0
	if revenue > 0 {
		marginRate = float64(margin) / float64(revenue) * 100
	}
	repeatCustomers := 0
	for _, customer := range snapshot.Customers {
		if customer.Active && strings.Contains(strings.ToLower(customer.Segment), "repeat") {
			repeatCustomers++
		}
	}

	signals := []Signal{
		{Key: "repeat_revenue", Title: "Repeat-customer leverage", Description: "Active repeat customers provide a measurable audience for a targeted retention move.", Confidence: confidence(repeatCustomers > 0, 0.92, 0.58)},
		{Key: "margin_capacity", Title: "Margin capacity is visible", Description: "The current snapshot supports comparing growth upside against contribution margin.", Confidence: confidence(revenue > 0, 0.88, 0.46)},
	}
	if inventoryUnits == 0 {
		signals = append(signals, Signal{Key: "inventory_unknown", Title: "Inventory pressure needs attention", Description: "No active inventory units are available in the verified snapshot.", Confidence: 0.91})
	}
	return Result{
		BusinessID: snapshot.BusinessID, GoalID: goal.ID, BusinessVersion: snapshot.Version, Focus: focus,
		Signals: signals,
		Evidence: []Evidence{
			{Source: "verified business snapshot", Metric: "revenue_cents", Value: float64(revenue), Unit: "cents"},
			{Source: "verified business snapshot", Metric: "margin_cents", Value: float64(margin), Unit: "cents"},
			{Source: "verified business snapshot", Metric: "active_products", Value: float64(activeProducts), Unit: "count"},
			{Source: "verified business snapshot", Metric: "repeat_customers", Value: float64(repeatCustomers), Unit: "count"},
			{Source: "derived deterministic measure", Metric: "margin_percent", Value: marginRate, Unit: "percent"},
		},
		Tradeoffs: []Tradeoff{
			{Key: "growth_vs_margin", Dimension: "economics", Description: "Retention spend can expand revenue while reducing near-term margin if the offer is too broad."},
			{Key: "growth_vs_inventory", Dimension: "operations", Description: "Demand growth must remain within the human inventory guardrails."},
		},
		Guardrails: constraintViews(goal.Constraints),
	}
}

func constraintViews(constraints []planning.Constraint) []Constraint {
	result := make([]Constraint, 0, len(constraints))
	for _, constraint := range constraints {
		result = append(result, Constraint{ID: constraint.ID, GoalID: constraint.GoalID, Key: constraint.Key, Value: constraint.Value, Hard: constraint.Hard, CreatedBy: constraint.CreatedBy})
	}
	return result
}

func confidence(condition bool, yes, no float64) float64 {
	if condition {
		return yes
	}
	return no
}
