package planning

import (
	"fmt"
	"strings"

	"webmcp-backend/internal/domain/core"
)

type GoalStatus string

const (
	GoalDraft     GoalStatus = "draft"
	GoalActive    GoalStatus = "active"
	GoalCompleted GoalStatus = "completed"
	GoalCancelled GoalStatus = "cancelled"
)

type Preference string

const (
	PreferenceConservative Preference = "conservative"
	PreferenceBalanced     Preference = "balanced"
	PreferenceAggressive   Preference = "aggressive"
)

type Constraint struct {
	ID        core.ID
	GoalID    core.ID
	Key       string
	Value     string
	Hard      bool
	CreatedBy core.ActorType
}

func (c Constraint) Validate() error {
	if err := c.ID.Validate("constraint_id"); err != nil {
		return err
	}
	if err := c.GoalID.Validate("goal_id"); err != nil {
		return err
	}
	if strings.TrimSpace(c.Key) == "" || strings.TrimSpace(c.Value) == "" {
		return fmt.Errorf("constraint key and value are required")
	}
	if err := c.CreatedBy.Validate(); err != nil {
		return err
	}
	return nil
}

type Goal struct {
	ID              core.ID
	BusinessID      core.ID
	Objective       string
	TimeHorizonDays int
	Preference      Preference
	Constraints     []Constraint
	Status          GoalStatus
	Version         uint64
}

func (g Goal) Validate() error {
	if err := g.ID.Validate("goal_id"); err != nil {
		return err
	}
	if err := g.BusinessID.Validate("business_id"); err != nil {
		return err
	}
	if strings.TrimSpace(g.Objective) == "" || g.TimeHorizonDays <= 0 || g.Version == 0 {
		return fmt.Errorf("goal objective, positive horizon and version are required")
	}
	switch g.Preference {
	case PreferenceConservative, PreferenceBalanced, PreferenceAggressive:
	default:
		return fmt.Errorf("invalid goal preference %q", g.Preference)
	}
	if !validGoalStatus(g.Status) {
		return fmt.Errorf("invalid goal status %q", g.Status)
	}
	for _, constraint := range g.Constraints {
		if err := constraint.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (g Goal) CanTransition(to GoalStatus) bool {
	switch g.Status {
	case GoalDraft:
		return to == GoalActive || to == GoalCancelled
	case GoalActive:
		return to == GoalCompleted || to == GoalCancelled
	case GoalCompleted, GoalCancelled:
		return false
	default:
		return false
	}
}

func (g Goal) Transition(to GoalStatus) (Goal, error) {
	if !g.CanTransition(to) {
		return Goal{}, fmt.Errorf("invalid goal transition %s -> %s", g.Status, to)
	}
	g.Status = to
	g.Version++
	return g, nil
}

func validGoalStatus(status GoalStatus) bool {
	return status == GoalDraft || status == GoalActive || status == GoalCompleted || status == GoalCancelled
}
