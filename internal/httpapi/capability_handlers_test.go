package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webmcp-backend/internal/application/analysisworkflow"
	"webmcp-backend/internal/application/planningworkflow"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
)

type testAnalysisAPI struct{}

func (testAnalysisAPI) Analyze(context.Context, analysisworkflow.Input) (analysisworkflow.Result, error) {
	return analysisworkflow.Result{BusinessID: "business-1", GoalID: "goal-1", BusinessVersion: 4, Focus: "retention", Guardrails: []analysisworkflow.Constraint{{ID: "constraint-1", GoalID: "goal-1", Key: "budget", Value: "8000", Hard: true, CreatedBy: core.ActorHuman}}}, nil
}

type testConstraintAPI struct{}

func (testConstraintAPI) AddConstraint(_ context.Context, input planningworkflow.ConstraintInput) (planning.Constraint, error) {
	return planning.Constraint{ID: "constraint-1", GoalID: input.GoalID, Key: input.Key, Value: input.Value, Hard: input.Hard, CreatedBy: core.ActorHuman}, nil
}

func TestAnalysisAndConstraintRoutesExposeStableContracts(t *testing.T) {
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal("owner")}, Analysis: testAnalysisAPI{}, Constraints: testConstraintAPI{},
	})

	analysisRequest := httptest.NewRequest(http.MethodPost, "/api/v1/analysis", strings.NewReader(`{"business_id":"business-1","goal_id":"goal-1","focus":"retention"}`))
	analysisResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(analysisResponse, analysisRequest)
	if analysisResponse.Code != http.StatusOK || !strings.Contains(analysisResponse.Body.String(), `"business_version":4`) || !strings.Contains(analysisResponse.Body.String(), `"goal_id":"goal-1"`) {
		t.Fatalf("analysis contract failed: %d %s", analysisResponse.Code, analysisResponse.Body.String())
	}

	constraintRequest := httptest.NewRequest(http.MethodPost, "/api/v1/goals/goal-1/constraints", strings.NewReader(`{"key":"budget","value":"8000","hard":true}`))
	constraintResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(constraintResponse, constraintRequest)
	if constraintResponse.Code != http.StatusOK || !strings.Contains(constraintResponse.Body.String(), `"created_by":"human"`) {
		t.Fatalf("constraint contract failed: %d %s", constraintResponse.Code, constraintResponse.Body.String())
	}
}
