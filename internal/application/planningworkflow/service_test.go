package planningworkflow

import (
	"context"
	"testing"

	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
)

type constraintWriter struct{ saved planning.Constraint }

func (w *constraintWriter) AddConstraint(_ context.Context, _ core.ID, _ core.ID, constraint planning.Constraint, _ string) error {
	w.saved = constraint
	return nil
}

func TestAddConstraintCreatesHumanOwnedBoundedConstraint(t *testing.T) {
	writer := &constraintWriter{}
	service := NewService(writer)
	result, err := service.AddConstraint(context.Background(), ConstraintInput{OrganizationID: "org-1", ActorID: "user-1", GoalID: "goal-1", Key: "budget", Value: "8000 cents", Hard: true, RequestID: "request-1"})
	if err != nil {
		t.Fatalf("add constraint failed: %v", err)
	}
	if result.Key != "budget" || result.Value != "8000 cents" || result.CreatedBy != core.ActorHuman || writer.saved.ID == "" {
		t.Fatalf("constraint was not created safely: %#v", result)
	}
}
