package planningworkflow

import (
	"context"
	"fmt"
	"strings"

	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
)

type API interface {
	AddConstraint(context.Context, ConstraintInput) (planning.Constraint, error)
}

type ConstraintWriter interface {
	AddConstraint(context.Context, core.ID, core.ID, planning.Constraint, string) error
}

type Service struct{ writer ConstraintWriter }

type ConstraintInput struct {
	OrganizationID core.ID
	ActorID        core.ID
	GoalID         core.ID
	Key            string
	Value          string
	Hard           bool
	RequestID      string
}

func NewService(writer ConstraintWriter) Service { return Service{writer: writer} }

func (s Service) AddConstraint(ctx context.Context, input ConstraintInput) (planning.Constraint, error) {
	for label, id := range map[string]core.ID{"organization_id": input.OrganizationID, "actor_id": input.ActorID, "goal_id": input.GoalID} {
		if err := id.Validate(label); err != nil {
			return planning.Constraint{}, err
		}
	}
	key, value := strings.TrimSpace(input.Key), strings.TrimSpace(input.Value)
	if key == "" || len(key) > 120 || value == "" || len(value) > 500 || strings.TrimSpace(input.RequestID) == "" {
		return planning.Constraint{}, fmt.Errorf("constraint key, value and request_id are required within their limits")
	}
	if s.writer == nil {
		return planning.Constraint{}, fmt.Errorf("planning workflow dependencies are unavailable")
	}
	id, err := core.NewID("constraint")
	if err != nil {
		return planning.Constraint{}, err
	}
	constraint := planning.Constraint{ID: id, GoalID: input.GoalID, Key: key, Value: value, Hard: input.Hard, CreatedBy: core.ActorHuman}
	if err := constraint.Validate(); err != nil {
		return planning.Constraint{}, err
	}
	if err := s.writer.AddConstraint(ctx, input.OrganizationID, input.ActorID, constraint, input.RequestID); err != nil {
		return planning.Constraint{}, err
	}
	return constraint, nil
}
