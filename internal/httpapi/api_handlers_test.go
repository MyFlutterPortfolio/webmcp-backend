package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webmcp-backend/internal/application/approvalworkflow"
	"webmcp-backend/internal/application/ports"
	"webmcp-backend/internal/application/proposalworkflow"
	"webmcp-backend/internal/application/scenarioworkflow"
	"webmcp-backend/internal/auth"
	"webmcp-backend/internal/config"
	"webmcp-backend/internal/domain/approval"
	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/commit"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/proposal"
	"webmcp-backend/internal/domain/scenario"
)

func testServerConfig() config.Config {
	return config.Config{
		Environment: "test", HTTPAddr: ":8080", ShutdownTimeout: time.Second,
		RequestTimeout: time.Second, MaxBodyBytes: 1024,
		AllowedOrigins:    []string{"https://app.example.com"},
		ReadHeaderTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second,
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type testAuthenticator struct {
	principal auth.Principal
	err       error
}

func (a testAuthenticator) Authenticate(context.Context, *http.Request) (auth.Principal, error) {
	return a.principal, a.err
}

type testSnapshotReader struct {
	snapshot       business.Snapshot
	organizationID core.ID
	businessID     core.ID
}

func (r *testSnapshotReader) GetSnapshot(_ context.Context, organizationID, businessID core.ID) (business.Snapshot, error) {
	r.organizationID = organizationID
	r.businessID = businessID
	return r.snapshot, nil
}

type testCommitter struct {
	called  bool
	input   ports.CommitInput
	outcome ports.CommitOutcome
	err     error
}

func (c *testCommitter) Commit(_ context.Context, input ports.CommitInput) (ports.CommitOutcome, error) {
	c.called = true
	c.input = input
	return c.outcome, c.err
}

func apiPrincipal(role auth.Role) auth.Principal {
	return auth.Principal{UserID: "user-1", OrganizationID: "org-1", Role: role}
}

type testScenarioWorkflow struct {
	created   bool
	simulated bool
	compared  bool
}

func (w *testScenarioWorkflow) CreateScenario(_ context.Context, input scenarioworkflow.CreateInput) (scenario.Scenario, error) {
	w.created = true
	return scenario.Scenario{ID: "scenario-1", GoalID: input.GoalID, BusinessID: input.BusinessID, BaseBusinessVersion: 4, Name: input.Name, Objective: input.Objective, ProposedActions: input.Actions, Status: scenario.StatusDraft, Version: 1}, nil
}

func (w *testScenarioWorkflow) SimulateScenario(_ context.Context, input scenarioworkflow.SimulateInput) (scenario.Scenario, error) {
	w.simulated = true
	return scenario.Scenario{ID: input.ScenarioID, BusinessID: input.BusinessID, BaseBusinessVersion: 4, Name: "Simulated", Objective: "Test", ProposedActions: []scenario.Action{{Type: scenario.ActionMarketingBudget, Value: 100, Unit: "cents"}}, Status: scenario.StatusSimulated, Version: 2, Result: scenario.Result{Calculated: true, BaselineMetrics: map[string]float64{}, ProjectedMetrics: map[string]float64{}, MetricDeltas: map[string]float64{}, Warnings: []string{}}}, nil
}

func (w *testScenarioWorkflow) CompareScenarios(_ context.Context, _ scenarioworkflow.CompareInput) (scenario.Comparison, error) {
	w.compared = true
	return scenario.Comparison{Items: []scenario.ComparisonItem{}, RecommendationAvailable: false, Warnings: []string{}}, nil
}

type testProposalWorkflow struct {
	created  bool
	revised  bool
	reviewed bool
}

type testApprovalWorkflow struct {
	called bool
	input  approvalworkflow.DecisionInput
	err    error
}

func (w *testApprovalWorkflow) Decide(_ context.Context, input approvalworkflow.DecisionInput) (approval.Decision, error) {
	w.called = true
	w.input = input
	if w.err != nil {
		return approval.Decision{}, w.err
	}
	return approval.Decision{
		ID: "approval-1", ProposalID: input.ProposalID, ProposalVersionID: input.ProposalVersionID,
		BusinessID: input.BusinessID, DecidedBy: input.ActorID, Status: input.Status, Reason: input.Reason,
	}, nil
}

func testProposal() proposal.Proposal {
	return proposal.Proposal{ID: "proposal-1", BusinessID: "biz-1", GoalID: "goal-1", Version: 1, Current: proposal.Version{
		ID: "proposal-version-1", ProposalID: "proposal-1", Number: 1, ScenarioID: "scenario-1", BaseBusinessVersion: 4,
		Summary: "Increase margin", Status: proposal.StatusDraft, Changes: []proposal.Change{},
	}}
}

func (w *testProposalWorkflow) CreateProposal(_ context.Context, _ proposalworkflow.CreateInput) (proposal.Proposal, error) {
	w.created = true
	return testProposal(), nil
}

func (w *testProposalWorkflow) ReviseProposal(_ context.Context, _ proposalworkflow.ReviseInput) (proposal.Proposal, error) {
	w.revised = true
	result := testProposal()
	result.Version = 2
	result.Current.Number = 2
	result.Current.ID = "proposal-version-2"
	result.Current.Status = proposal.StatusDraft
	return result, nil
}

func (w *testProposalWorkflow) RequestHumanReview(_ context.Context, _ proposalworkflow.ReviewInput) (proposal.Proposal, error) {
	w.reviewed = true
	result := testProposal()
	result.Version = 2
	result.Current.Status = proposal.StatusInReview
	return result, nil
}

func TestAPIRequiresConfiguredAuthentication(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/biz-1/snapshot", nil)
	response := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected auth configuration failure, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"auth_not_configured"`) {
		t.Fatalf("expected safe auth error, got %s", response.Body.String())
	}
	if response.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("authentication challenge header is missing")
	}
}

func TestBusinessSnapshotUsesAuthenticatedTenantAndStableJSON(t *testing.T) {
	reader := &testSnapshotReader{snapshot: business.Snapshot{
		BusinessID: "biz-1", Version: 7,
		Products: []business.Product{{ID: "product-1", SKU: "sku-1", Name: "Starter", PriceCents: 1200, CostCents: 400, InventoryUnits: 8, Active: true, Version: 2}},
	}}
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator:  testAuthenticator{principal: apiPrincipal(auth.RoleViewer)},
		SnapshotReader: reader,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/biz-1/snapshot", nil)
	request.Header.Set("X-Request-ID", "snapshot-test")
	response := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected snapshot success, got %d: %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode snapshot response: %v", err)
	}
	if payload["business_id"] != "biz-1" || payload["version"] != float64(7) {
		t.Fatalf("unexpected snapshot envelope: %#v", payload)
	}
	if !strings.Contains(response.Body.String(), `"sku":"sku-1"`) {
		t.Fatalf("snapshot item fields are not stable JSON: %s", response.Body.String())
	}
	if reader.organizationID != "org-1" || reader.businessID != "biz-1" {
		t.Fatalf("snapshot was not scoped to authenticated tenant: %#v", reader)
	}
}

func TestViewerCannotCommitAndCommitterIsNotCalled(t *testing.T) {
	committer := &testCommitter{}
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleViewer)},
		Committer:     committer,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/proposals/proposal-1/commit", strings.NewReader(`{"business_id":"biz-1","proposal_version_id":"version-1"}`))
	request.Header.Set("Idempotency-Key", "commit-test-1")
	response := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected viewer rejection, got %d", response.Code)
	}
	if committer.called {
		t.Fatal("viewer request reached the commit port")
	}
}

func TestCommitEndpointPassesBodyBusinessAndIdempotencyContract(t *testing.T) {
	committer := &testCommitter{outcome: ports.CommitOutcome{
		OperationID: "operation-1", DecisionID: "decision-1", Status: commit.StatusSucceeded,
		CommittedBusinessVersion: 8,
	}}
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleOwner)},
		Committer:     committer,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/proposals/proposal-1/commit", strings.NewReader(`{"business_id":"biz-1","proposal_version_id":"version-1"}`))
	request.Header.Set("Idempotency-Key", "commit-test-2")
	request.Header.Set("X-Request-ID", "request-commit-2")
	response := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected commit success, got %d: %s", response.Code, response.Body.String())
	}
	if !committer.called || committer.input.BusinessID != "biz-1" || committer.input.ProposalVersionID != "version-1" {
		t.Fatalf("commit input did not preserve explicit resource identity: %#v", committer.input)
	}
	if committer.input.IdempotencyKey != "commit-test-2" || !committer.input.Authorized {
		t.Fatalf("commit safety fields were not propagated: %#v", committer.input)
	}
	if !strings.Contains(response.Body.String(), `"request_id":"request-commit-2"`) {
		t.Fatalf("request correlation was not returned: %s", response.Body.String())
	}
}

func TestCommitEndpointRejectsUnknownFields(t *testing.T) {
	committer := &testCommitter{}
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleOwner)},
		Committer:     committer,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/proposals/proposal-1/commit", strings.NewReader(`{"business_id":"biz-1","proposal_version_id":"version-1","unexpected":true}`))
	request.Header.Set("Idempotency-Key", "commit-test-3")
	response := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || committer.called {
		t.Fatalf("invalid commit body was accepted: status=%d called=%t", response.Code, committer.called)
	}
}

func TestAuthenticatorErrorsRemainSafe(t *testing.T) {
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{err: errors.New("token rejected")},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/biz-1/snapshot", nil)
	response := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "token rejected") {
		t.Fatalf("authenticator error leaked or mapped incorrectly: %d %s", response.Code, response.Body.String())
	}
}

func TestScenarioWorkflowEndpointsKeepSemanticContracts(t *testing.T) {
	workflow := &testScenarioWorkflow{}
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleOwner)}, Scenarios: workflow,
	})
	create := httptest.NewRequest(http.MethodPost, "/api/v1/scenarios", strings.NewReader(`{"business_id":"biz-1","goal_id":"goal-1","name":"Balanced","objective":"Grow margin","proposed_actions":[{"type":"marketing_budget","value":1000,"unit":"cents"}]}`))
	createResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated || !workflow.created || !strings.Contains(createResponse.Body.String(), `"status":"draft"`) {
		t.Fatalf("scenario creation contract failed: %d %s", createResponse.Code, createResponse.Body.String())
	}

	simulate := httptest.NewRequest(http.MethodPost, "/api/v1/scenarios/scenario-1/simulation", strings.NewReader(`{"business_id":"biz-1"}`))
	simulateResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(simulateResponse, simulate)
	if simulateResponse.Code != http.StatusOK || !workflow.simulated || !strings.Contains(simulateResponse.Body.String(), `"calculated":true`) {
		t.Fatalf("scenario simulation contract failed: %d %s", simulateResponse.Code, simulateResponse.Body.String())
	}

	compare := httptest.NewRequest(http.MethodPost, "/api/v1/scenarios/compare", strings.NewReader(`{"business_id":"biz-1","scenario_ids":["scenario-1","scenario-2"]}`))
	compareResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(compareResponse, compare)
	if compareResponse.Code != http.StatusOK || !workflow.compared || !strings.Contains(compareResponse.Body.String(), `"recommendation_available":false`) {
		t.Fatalf("scenario comparison contract failed: %d %s", compareResponse.Code, compareResponse.Body.String())
	}
}

func TestProposalWorkflowEndpointsKeepReviewBoundary(t *testing.T) {
	workflow := &testProposalWorkflow{}
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleOwner)}, Proposals: workflow,
	})
	create := httptest.NewRequest(http.MethodPost, "/api/v1/proposals", strings.NewReader(`{"business_id":"biz-1","goal_id":"goal-1","scenario_id":"scenario-1","summary":"Increase margin safely"}`))
	createResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated || !workflow.created || !strings.Contains(createResponse.Body.String(), `"status":"draft"`) {
		t.Fatalf("proposal creation contract failed: %d %s", createResponse.Code, createResponse.Body.String())
	}

	revise := httptest.NewRequest(http.MethodPost, "/api/v1/proposals/proposal-1/revisions", strings.NewReader(`{"feedback":"Keep spend below budget"}`))
	reviseResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(reviseResponse, revise)
	if reviseResponse.Code != http.StatusOK || !workflow.revised || !strings.Contains(reviseResponse.Body.String(), `"number":2`) {
		t.Fatalf("proposal revision contract failed: %d %s", reviseResponse.Code, reviseResponse.Body.String())
	}

	review := httptest.NewRequest(http.MethodPost, "/api/v1/proposals/proposal-1/review", strings.NewReader(`{"business_id":"biz-1"}`))
	reviewResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(reviewResponse, review)
	if reviewResponse.Code != http.StatusOK || !workflow.reviewed || !strings.Contains(reviewResponse.Body.String(), `"status":"in_review"`) {
		t.Fatalf("proposal review contract failed: %d %s", reviewResponse.Code, reviewResponse.Body.String())
	}
}

func TestApprovalEndpointPreservesHumanDecisionContext(t *testing.T) {
	workflow := &testApprovalWorkflow{}
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleOwner)}, Approvals: workflow,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/proposals/proposal-1/approval", strings.NewReader(`{"business_id":"biz-1","proposal_version_id":"proposal-version-1","decision":"approved","reason":"Reviewed the modeled trade-offs and operating limits."}`))
	request.Header.Set("X-Request-ID", "approval-request-1")
	response := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !workflow.called {
		t.Fatalf("approval endpoint failed: status=%d called=%t body=%s", response.Code, workflow.called, response.Body.String())
	}
	if workflow.input.BusinessID != "biz-1" || workflow.input.ProposalID != "proposal-1" || workflow.input.ProposalVersionID != "proposal-version-1" || workflow.input.Status != approval.StatusApproved || workflow.input.ActorID != "user-1" {
		t.Fatalf("approval context was not preserved: %#v", workflow.input)
	}
	if workflow.input.RequestID != "approval-request-1" || !strings.Contains(response.Body.String(), `"status":"approved"`) {
		t.Fatalf("approval response/correlation contract failed: %s", response.Body.String())
	}
}

func TestViewerCannotDecideApprovalAndApprovalWorkflowIsNotCalled(t *testing.T) {
	workflow := &testApprovalWorkflow{}
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleViewer)}, Approvals: workflow,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/proposals/proposal-1/approval", strings.NewReader(`{"business_id":"biz-1","proposal_version_id":"proposal-version-1","decision":"approved","reason":"Not authorized."}`))
	response := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || workflow.called {
		t.Fatalf("viewer reached approval boundary: status=%d called=%t", response.Code, workflow.called)
	}
}
