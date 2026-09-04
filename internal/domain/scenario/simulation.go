package scenario

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
)

const marketingReturnMultiple = 2.0

type ComparisonItem struct {
	ID               core.ID
	Name             string
	Status           Status
	ProjectedMetrics map[string]float64
	MetricDeltas     map[string]float64
	Risks            []Risk
	Warnings         []string
}

type Comparison struct {
	Items                   []ComparisonItem
	RecommendedScenarioID   core.ID
	RecommendationAvailable bool
	RecommendationBasis     string
	Warnings                []string
}

func Compare(candidates []Scenario) (Comparison, error) {
	if len(candidates) < 2 {
		return Comparison{}, fmt.Errorf("at least two scenarios are required for comparison")
	}
	comparison := Comparison{Items: make([]ComparisonItem, 0, len(candidates)), Warnings: make([]string, 0)}
	var best *Scenario
	for _, candidate := range candidates {
		if candidate.Status != StatusSimulated && candidate.Status != StatusCompared {
			return Comparison{}, fmt.Errorf("scenario %s must be simulated before comparison", candidate.ID)
		}
		if !candidate.Result.Calculated {
			return Comparison{}, fmt.Errorf("scenario %s has no calculated result", candidate.ID)
		}
		comparison.Items = append(comparison.Items, ComparisonItem{
			ID: candidate.ID, Name: candidate.Name, Status: candidate.Status,
			ProjectedMetrics: candidate.Result.ProjectedMetrics, MetricDeltas: candidate.Result.MetricDeltas,
			Risks: candidate.Risks, Warnings: candidate.Result.Warnings,
		})
		if hasHighRisk(candidate.Risks) {
			continue
		}
		if best == nil || betterMargin(candidate, *best) {
			copy := candidate
			best = &copy
		}
	}
	if best == nil {
		comparison.Warnings = append(comparison.Warnings, "every candidate contains a high-severity risk; human review must resolve the risks before selection")
		return comparison, nil
	}
	comparison.RecommendedScenarioID = best.ID
	comparison.RecommendationAvailable = true
	comparison.RecommendationBasis = "highest projected margin among candidates without high-severity risks"
	return comparison, nil
}

func hasHighRisk(risks []Risk) bool {
	for _, risk := range risks {
		if strings.EqualFold(risk.Severity, "high") {
			return true
		}
	}
	return false
}

func betterMargin(candidate, current Scenario) bool {
	candidateMargin := candidate.Result.ProjectedMetrics["margin_cents"]
	currentMargin := current.Result.ProjectedMetrics["margin_cents"]
	if candidateMargin == currentMargin {
		return string(candidate.ID) < string(current.ID)
	}
	return candidateMargin > currentMargin
}

// Simulate applies only deterministic, bounded rules to a verified snapshot.
// It never mutates the canonical business state and never calls an LLM.
func Simulate(snapshot business.Snapshot, goal planning.Goal, candidate Scenario) (Scenario, error) {
	if err := snapshot.Validate(); err != nil {
		return Scenario{}, fmt.Errorf("invalid simulation snapshot: %w", err)
	}
	if err := goal.Validate(); err != nil {
		return Scenario{}, fmt.Errorf("invalid simulation goal: %w", err)
	}
	if err := candidate.Validate(); err != nil {
		return Scenario{}, fmt.Errorf("invalid simulation scenario: %w", err)
	}
	if candidate.Status != StatusDraft {
		return Scenario{}, fmt.Errorf("only draft scenarios can be simulated")
	}
	if candidate.BusinessID != snapshot.BusinessID || candidate.GoalID != goal.ID || candidate.BaseBusinessVersion != snapshot.Version {
		return Scenario{}, fmt.Errorf("scenario context does not match the verified snapshot and goal")
	}

	products := make(map[string]business.Product, len(snapshot.Products))
	for _, product := range snapshot.Products {
		products[string(product.ID)] = product
	}
	ordersByProduct := make(map[string]float64)
	baselineRevenue, baselineCost := 0.0, 0.0
	for _, order := range snapshot.Orders {
		ordersByProduct[string(order.ProductID)] += float64(order.RevenueCents)
		baselineRevenue += float64(order.RevenueCents)
		baselineCost += float64(order.CostCents)
	}
	baselineInventory := 0.0
	for _, product := range snapshot.Products {
		baselineInventory += float64(product.InventoryUnits)
	}

	projectedRevenue, projectedCost, projectedInventory, marketingSpend := baselineRevenue, baselineCost, baselineInventory, 0.0
	warnings := make([]string, 0)
	risks := make([]Risk, 0)
	for _, action := range candidate.ProposedActions {
		switch action.Type {
		case ActionPriceAdjustment:
			product, exists := products[string(action.TargetID)]
			if !exists || !product.Active {
				return Scenario{}, fmt.Errorf("price adjustment target %q is not an active product", action.TargetID)
			}
			productRevenue := ordersByProduct[string(action.TargetID)]
			projectedRevenue += productRevenue * action.Value / 100
			if productRevenue == 0 {
				warnings = append(warnings, fmt.Sprintf("product %s has no observed order revenue in the snapshot", action.TargetID))
			}
		case ActionInventoryReplenish:
			product, exists := products[string(action.TargetID)]
			if !exists || !product.Active {
				return Scenario{}, fmt.Errorf("inventory target %q is not an active product", action.TargetID)
			}
			projectedInventory += action.Value
		case ActionMarketingBudget:
			marketingSpend += action.Value
			projectedRevenue += action.Value * marketingReturnMultiple
			projectedCost += action.Value
		}
	}

	applyHardBudgetRisk(goal, marketingSpend, &warnings, &risks)
	projectedMargin := projectedRevenue - projectedCost
	baselineMargin := baselineRevenue - baselineCost
	result := Result{
		BaselineMetrics: map[string]float64{
			"revenue_cents": baselineRevenue, "cost_cents": baselineCost,
			"margin_cents": baselineMargin, "inventory_units": baselineInventory,
		},
		ProjectedMetrics: map[string]float64{
			"revenue_cents": projectedRevenue, "cost_cents": projectedCost,
			"margin_cents": projectedMargin, "inventory_units": projectedInventory,
			"marketing_spend_cents": marketingSpend,
		},
		MetricDeltas: map[string]float64{
			"revenue_cents":   projectedRevenue - baselineRevenue,
			"cost_cents":      projectedCost - baselineCost,
			"margin_cents":    projectedMargin - baselineMargin,
			"inventory_units": projectedInventory - baselineInventory,
		},
		Warnings: warnings, Calculated: true,
	}
	candidate.Result = result
	candidate.Risks = risks
	candidate.Status = StatusSimulated
	candidate.Version++
	return candidate, nil
}

func applyHardBudgetRisk(goal planning.Goal, marketingSpend float64, warnings *[]string, risks *[]Risk) {
	for _, constraint := range goal.Constraints {
		if !constraint.Hard || !strings.EqualFold(strings.TrimSpace(constraint.Key), "budget") {
			continue
		}
		limit, err := strconv.ParseFloat(strings.TrimSpace(constraint.Value), 64)
		if err != nil || math.IsNaN(limit) || math.IsInf(limit, 0) || limit < 0 {
			*warnings = append(*warnings, "hard budget constraint could not be evaluated because its value is not numeric")
			*risks = append(*risks, Risk{Key: "budget_constraint_unknown", Description: "The hard budget constraint needs a numeric cents value before approval.", Severity: "high"})
			continue
		}
		if marketingSpend > limit {
			*warnings = append(*warnings, "projected marketing spend exceeds the hard budget constraint")
			*risks = append(*risks, Risk{Key: "budget_exceeded", Description: "Projected marketing spend exceeds the human hard budget constraint.", Severity: "high"})
		}
	}
}
