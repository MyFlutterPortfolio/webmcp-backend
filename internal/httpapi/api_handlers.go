package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"webmcp-backend/internal/application/analysisworkflow"
	"webmcp-backend/internal/application/approvalworkflow"
	"webmcp-backend/internal/application/planningworkflow"
	"webmcp-backend/internal/application/ports"
	"webmcp-backend/internal/application/proposalworkflow"
	"webmcp-backend/internal/application/scenarioworkflow"
	"webmcp-backend/internal/auth"
	"webmcp-backend/internal/domain/approval"
	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/proposal"
	"webmcp-backend/internal/domain/scenario"
	"webmcp-backend/internal/platform/postgres"
)

// Dependencies are the application ports required by the HTTP boundary.
// Authentication is intentionally an explicit dependency: the API must fail
// closed until a real identity provider is configured.
type Dependencies struct {
	SnapshotReader ports.BusinessSnapshotReader
	Committer      ports.Committer
	Authenticator  auth.Authenticator
	Authorizer     PrincipalAuthorizer
	Readiness      ReadinessCheck
	Scenarios      scenarioworkflow.API
	Proposals      proposalworkflow.API
	Approvals      approvalworkflow.API
	Analysis       analysisworkflow.API
	Constraints    planningworkflow.API
	GuestIssuer    GuestSessionIssuer
}

// PrincipalAuthorizer maps a verified token subject to the server-side
// organization membership and role. Signature verification alone is not an
// authorization decision.
type PrincipalAuthorizer interface {
	AuthorizePrincipal(context.Context, auth.Principal) error
}

type scenarioResponse struct {
	Status              scenario.Status       `json:"status"`
	ID                  core.ID               `json:"id"`
	GoalID              core.ID               `json:"goal_id"`
	BusinessID          core.ID               `json:"business_id"`
	BaseBusinessVersion uint64                `json:"base_business_version"`
	Name                string                `json:"name"`
	Objective           string                `json:"objective"`
	ProposedActions     []scenario.Action     `json:"proposed_actions"`
	Assumptions         []scenario.Assumption `json:"assumptions"`
	Result              scenario.Result       `json:"result"`
	Risks               []scenario.Risk       `json:"risks"`
	Version             uint64                `json:"version"`
	RequestID           string                `json:"request_id"`
}

type comparisonResponse struct {
	Items                   []comparisonItemResponse `json:"items"`
	RecommendedScenarioID   core.ID                  `json:"recommended_scenario_id,omitempty"`
	RecommendationAvailable bool                     `json:"recommendation_available"`
	RecommendationBasis     string                   `json:"recommendation_basis"`
	Warnings                []string                 `json:"warnings"`
	RequestID               string                   `json:"request_id"`
}

type comparisonItemResponse struct {
	ID               core.ID            `json:"id"`
	Name             string             `json:"name"`
	Status           scenario.Status    `json:"status"`
	ProjectedMetrics map[string]float64 `json:"projected_metrics"`
	MetricDeltas     map[string]float64 `json:"metric_deltas"`
	Risks            []scenario.Risk    `json:"risks"`
	Warnings         []string           `json:"warnings"`
}

type createScenarioRequest struct {
	BusinessID      string            `json:"business_id"`
	GoalID          string            `json:"goal_id"`
	Name            string            `json:"name"`
	Objective       string            `json:"objective"`
	ProposedActions []scenario.Action `json:"proposed_actions"`
}

type analyzeBusinessRequest struct {
	BusinessID string `json:"business_id"`
	GoalID     string `json:"goal_id"`
	Focus      string `json:"focus"`
}

type constraintRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Hard  bool   `json:"hard"`
}

type compareScenariosRequest struct {
	BusinessID  string   `json:"business_id"`
	ScenarioIDs []string `json:"scenario_ids"`
}

type proposalResponse struct {
	ID         core.ID                 `json:"id"`
	BusinessID core.ID                 `json:"business_id"`
	GoalID     core.ID                 `json:"goal_id"`
	Version    uint64                  `json:"version"`
	Current    proposalVersionResponse `json:"current"`
	RequestID  string                  `json:"request_id"`
}

type proposalVersionResponse struct {
	ID                  core.ID           `json:"id"`
	ProposalID          core.ID           `json:"proposal_id"`
	Number              int               `json:"number"`
	ScenarioID          core.ID           `json:"scenario_id"`
	BaseBusinessVersion uint64            `json:"base_business_version"`
	Summary             string            `json:"summary"`
	Changes             []proposal.Change `json:"changes"`
	Status              proposal.Status   `json:"status"`
}

type createProposalRequest struct {
	BusinessID string `json:"business_id"`
	GoalID     string `json:"goal_id"`
	ScenarioID string `json:"scenario_id"`
	Summary    string `json:"summary"`
}

type analysisResponse struct {
	analysisworkflow.Result
	RequestID string `json:"request_id"`
}

type constraintResponse struct {
	ID        core.ID        `json:"id"`
	GoalID    core.ID        `json:"goal_id"`
	Key       string         `json:"key"`
	Value     string         `json:"value"`
	Hard      bool           `json:"hard"`
	CreatedBy core.ActorType `json:"created_by"`
	RequestID string         `json:"request_id"`
}

func (s *Server) analyzeBusiness(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if s.deps.Analysis == nil {
		writeError(w, r, http.StatusServiceUnavailable, "analysis_unavailable", "analysis service is unavailable")
		return
	}
	var request analyzeBusinessRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_analysis_request", err.Error())
		return
	}
	if !principal.GuestAllows(core.ID(strings.TrimSpace(request.BusinessID)), core.ID(strings.TrimSpace(request.GoalID))) {
		writeError(w, r, http.StatusForbidden, "guest_scope_forbidden", "guest demo is limited to its configured business and goal")
		return
	}
	result, err := s.deps.Analysis.Analyze(r.Context(), analysisworkflow.Input{
		OrganizationID: principal.OrganizationID, ActorID: principal.UserID,
		BusinessID: core.ID(strings.TrimSpace(request.BusinessID)), GoalID: core.ID(strings.TrimSpace(request.GoalID)), Focus: request.Focus,
	})
	if err != nil {
		if strings.Contains(err.Error(), "dependencies are unavailable") {
			writeError(w, r, http.StatusServiceUnavailable, "analysis_unavailable", "analysis service is unavailable")
			return
		}
		writeError(w, r, http.StatusUnprocessableEntity, "analysis_rejected", "business analysis could not be produced")
		return
	}
	writeJSON(w, http.StatusOK, analysisResponse{Result: result, RequestID: requestIDFromContext(r.Context())})
}

func (s *Server) updateConstraint(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if !principal.CanOperate() {
		writeError(w, r, http.StatusForbidden, "constraint_forbidden", "the current role cannot update constraints")
		return
	}
	if s.deps.Constraints == nil {
		writeError(w, r, http.StatusServiceUnavailable, "planning_unavailable", "planning service is unavailable")
		return
	}
	var request constraintRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_constraint_request", err.Error())
		return
	}
	constraint, err := s.deps.Constraints.AddConstraint(r.Context(), planningworkflow.ConstraintInput{
		OrganizationID: principal.OrganizationID, ActorID: principal.UserID, GoalID: core.ID(r.PathValue("goalId")),
		Key: request.Key, Value: request.Value, Hard: request.Hard, RequestID: requestIDFromContext(r.Context()),
	})
	if err != nil {
		if strings.Contains(err.Error(), "dependencies are unavailable") {
			writeError(w, r, http.StatusServiceUnavailable, "planning_unavailable", "planning service is unavailable")
			return
		}
		writeError(w, r, http.StatusUnprocessableEntity, "constraint_rejected", "constraint could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, constraintResponse{ID: constraint.ID, GoalID: constraint.GoalID, Key: constraint.Key, Value: constraint.Value, Hard: constraint.Hard, CreatedBy: constraint.CreatedBy, RequestID: requestIDFromContext(r.Context())})
}

type reviseProposalRequest struct {
	Feedback   string `json:"feedback"`
	BusinessID string `json:"business_id"`
}

type reviewProposalRequest struct {
	BusinessID string `json:"business_id"`
}

type approvalDecisionRequest struct {
	BusinessID        string `json:"business_id"`
	ProposalVersionID string `json:"proposal_version_id"`
	Decision          string `json:"decision"`
	Reason            string `json:"reason"`
}

type approvalDecisionResponse struct {
	ID                core.ID         `json:"id"`
	ProposalID        core.ID         `json:"proposal_id"`
	ProposalVersionID core.ID         `json:"proposal_version_id"`
	BusinessID        core.ID         `json:"business_id"`
	Status            approval.Status `json:"status"`
	Reason            string          `json:"reason"`
	DecidedBy         core.ID         `json:"decided_by"`
	RequestID         string          `json:"request_id"`
}

func (s *Server) createScenario(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if !principal.CanOperate() && !principal.CanDemoOperate() {
		writeError(w, r, http.StatusForbidden, "scenario_forbidden", "the current role cannot create a scenario")
		return
	}
	if s.deps.Scenarios == nil {
		writeError(w, r, http.StatusServiceUnavailable, "scenario_unavailable", "scenario service is unavailable")
		return
	}
	var request createScenarioRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_scenario_request", err.Error())
		return
	}
	if !principal.GuestAllows(core.ID(strings.TrimSpace(request.BusinessID)), core.ID(strings.TrimSpace(request.GoalID))) {
		writeError(w, r, http.StatusForbidden, "guest_scope_forbidden", "guest demo is limited to its configured business and goal")
		return
	}
	candidate, err := s.deps.Scenarios.CreateScenario(r.Context(), scenarioworkflow.CreateInput{
		OrganizationID: principal.OrganizationID, ActorID: principal.UserID,
		BusinessID: core.ID(strings.TrimSpace(request.BusinessID)), GoalID: core.ID(strings.TrimSpace(request.GoalID)),
		Name: request.Name, Objective: request.Objective, Actions: request.ProposedActions,
		RequestID: requestIDFromContext(r.Context()),
	})
	if err != nil {
		writeScenarioError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, scenarioDTO(candidate, requestIDFromContext(r.Context())))
}

func (s *Server) simulateScenario(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if !principal.CanOperate() && !principal.CanDemoOperate() {
		writeError(w, r, http.StatusForbidden, "scenario_forbidden", "the current role cannot simulate a scenario")
		return
	}
	if s.deps.Scenarios == nil {
		writeError(w, r, http.StatusServiceUnavailable, "scenario_unavailable", "scenario service is unavailable")
		return
	}
	var request struct {
		BusinessID string `json:"business_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_simulation_request", err.Error())
		return
	}
	if !principal.GuestAllows(core.ID(strings.TrimSpace(request.BusinessID)), "") {
		writeError(w, r, http.StatusForbidden, "guest_scope_forbidden", "guest demo is limited to its configured business")
		return
	}
	candidate, err := s.deps.Scenarios.SimulateScenario(r.Context(), scenarioworkflow.SimulateInput{
		OrganizationID: principal.OrganizationID, ActorID: principal.UserID,
		BusinessID: core.ID(strings.TrimSpace(request.BusinessID)), ScenarioID: core.ID(r.PathValue("scenarioId")),
		RequestID: requestIDFromContext(r.Context()),
	})
	if err != nil {
		writeScenarioError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, scenarioDTO(candidate, requestIDFromContext(r.Context())))
}

func (s *Server) compareScenarios(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if s.deps.Scenarios == nil {
		writeError(w, r, http.StatusServiceUnavailable, "scenario_unavailable", "scenario service is unavailable")
		return
	}
	var request compareScenariosRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_comparison_request", err.Error())
		return
	}
	if !principal.GuestAllows(core.ID(strings.TrimSpace(request.BusinessID)), "") {
		writeError(w, r, http.StatusForbidden, "guest_scope_forbidden", "guest demo is limited to its configured business")
		return
	}
	ids := make([]core.ID, len(request.ScenarioIDs))
	for index, id := range request.ScenarioIDs {
		ids[index] = core.ID(strings.TrimSpace(id))
	}
	comparison, err := s.deps.Scenarios.CompareScenarios(r.Context(), scenarioworkflow.CompareInput{
		OrganizationID: principal.OrganizationID, BusinessID: core.ID(strings.TrimSpace(request.BusinessID)), ScenarioIDs: ids,
	})
	if err != nil {
		writeScenarioError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, comparisonDTO(comparison, requestIDFromContext(r.Context())))
}

func (s *Server) createProposal(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if !principal.CanOperate() && !principal.CanDemoOperate() {
		writeError(w, r, http.StatusForbidden, "proposal_forbidden", "the current role cannot create a proposal")
		return
	}
	if s.deps.Proposals == nil {
		writeError(w, r, http.StatusServiceUnavailable, "proposal_unavailable", "proposal service is unavailable")
		return
	}
	var request createProposalRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_proposal_request", err.Error())
		return
	}
	if !principal.GuestAllows(core.ID(strings.TrimSpace(request.BusinessID)), core.ID(strings.TrimSpace(request.GoalID))) {
		writeError(w, r, http.StatusForbidden, "guest_scope_forbidden", "guest demo is limited to its configured business and goal")
		return
	}
	created, err := s.deps.Proposals.CreateProposal(r.Context(), proposalworkflow.CreateInput{
		OrganizationID: principal.OrganizationID, ActorID: principal.UserID,
		BusinessID: core.ID(strings.TrimSpace(request.BusinessID)), GoalID: core.ID(strings.TrimSpace(request.GoalID)),
		ScenarioID: core.ID(strings.TrimSpace(request.ScenarioID)), Summary: request.Summary,
		RequestID: requestIDFromContext(r.Context()),
	})
	if err != nil {
		writeProposalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, proposalDTO(created, requestIDFromContext(r.Context())))
}

func (s *Server) reviseProposal(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if !principal.CanOperate() && !principal.CanDemoOperate() {
		writeError(w, r, http.StatusForbidden, "proposal_forbidden", "the current role cannot revise a proposal")
		return
	}
	if s.deps.Proposals == nil {
		writeError(w, r, http.StatusServiceUnavailable, "proposal_unavailable", "proposal service is unavailable")
		return
	}
	var request reviseProposalRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_revision_request", err.Error())
		return
	}
	if !principal.GuestAllows(core.ID(strings.TrimSpace(request.BusinessID)), "") {
		writeError(w, r, http.StatusForbidden, "guest_scope_forbidden", "guest demo is limited to its configured business")
		return
	}
	updated, err := s.deps.Proposals.ReviseProposal(r.Context(), proposalworkflow.ReviseInput{
		OrganizationID: principal.OrganizationID, ActorID: principal.UserID,
		BusinessID: core.ID(strings.TrimSpace(request.BusinessID)), ProposalID: core.ID(r.PathValue("proposalId")), Feedback: request.Feedback,
		RequestID: requestIDFromContext(r.Context()),
	})
	if err != nil {
		writeProposalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, proposalDTO(updated, requestIDFromContext(r.Context())))
}

func (s *Server) requestHumanReview(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if !principal.CanOperate() && !principal.CanDemoOperate() {
		writeError(w, r, http.StatusForbidden, "proposal_forbidden", "the current role cannot request human review")
		return
	}
	if s.deps.Proposals == nil {
		writeError(w, r, http.StatusServiceUnavailable, "proposal_unavailable", "proposal service is unavailable")
		return
	}
	var request reviewProposalRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_review_request", err.Error())
		return
	}
	if !principal.GuestAllows(core.ID(strings.TrimSpace(request.BusinessID)), "") {
		writeError(w, r, http.StatusForbidden, "guest_scope_forbidden", "guest demo is limited to its configured business")
		return
	}
	updated, err := s.deps.Proposals.RequestHumanReview(r.Context(), proposalworkflow.ReviewInput{
		OrganizationID: principal.OrganizationID, ActorID: principal.UserID, BusinessID: core.ID(strings.TrimSpace(request.BusinessID)),
		ProposalID: core.ID(r.PathValue("proposalId")), RequestID: requestIDFromContext(r.Context()),
	})
	if err != nil {
		writeProposalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, proposalDTO(updated, requestIDFromContext(r.Context())))
}

func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if !principal.CanApprove() {
		writeError(w, r, http.StatusForbidden, "approval_forbidden", "the current role cannot approve a proposal")
		return
	}
	if s.deps.Approvals == nil {
		writeError(w, r, http.StatusServiceUnavailable, "approval_unavailable", "approval service is unavailable")
		return
	}
	var request approvalDecisionRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_approval_request", err.Error())
		return
	}
	status := approval.Status(strings.TrimSpace(request.Decision))
	if status != approval.StatusApproved && status != approval.StatusRejected {
		writeError(w, r, http.StatusBadRequest, "invalid_approval_decision", "decision must be approved or rejected")
		return
	}
	decision, err := s.deps.Approvals.Decide(r.Context(), approvalworkflow.DecisionInput{
		OrganizationID: principal.OrganizationID, ActorID: principal.UserID,
		ProposalID: core.ID(r.PathValue("proposalId")), ProposalVersionID: core.ID(strings.TrimSpace(request.ProposalVersionID)),
		BusinessID: core.ID(strings.TrimSpace(request.BusinessID)), Status: status, Reason: request.Reason,
		RequestID: requestIDFromContext(r.Context()),
	})
	if err != nil {
		writeApprovalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, approvalDTO(decision, requestIDFromContext(r.Context())))
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("request body is required")
		}
		return fmt.Errorf("request body is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("request body must contain one JSON object")
	}
	return nil
}

func writeScenarioError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusUnprocessableEntity, "scenario_rejected", "scenario request was rejected"
	switch {
	case errors.Is(err, scenarioworkflow.ErrGoalNotFound):
		status, code, message = http.StatusNotFound, "goal_not_found", "goal was not found"
	case errors.Is(err, scenarioworkflow.ErrScenarioNotFound):
		status, code, message = http.StatusNotFound, "scenario_not_found", "scenario was not found"
	case errors.Is(err, postgres.ErrBusinessNotFound):
		status, code, message = http.StatusNotFound, "business_not_found", "business was not found"
	case errors.Is(err, scenarioworkflow.ErrScenarioContext):
		status, code, message = http.StatusConflict, "scenario_context_mismatch", "scenario context does not match the current business state"
	case errors.Is(err, scenarioworkflow.ErrScenarioConflict):
		status, code, message = http.StatusConflict, "scenario_conflict", "scenario changed before the operation completed"
	case errors.Is(err, scenarioworkflow.ErrScenarioStale):
		status, code, message = http.StatusConflict, "stale_scenario", "scenario was based on an outdated business version"
	case errors.Is(err, scenarioworkflow.ErrScenarioUnauthorized):
		status, code, message = http.StatusForbidden, "scenario_forbidden", "scenario authorization was denied"
	case strings.Contains(err.Error(), "dependencies are unavailable"):
		status, code, message = http.StatusServiceUnavailable, "scenario_unavailable", "scenario service is unavailable"
	}
	writeError(w, r, status, code, message)
}

func writeProposalError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusUnprocessableEntity, "proposal_rejected", "proposal request was rejected"
	switch {
	case errors.Is(err, proposalworkflow.ErrProposalNotFound):
		status, code, message = http.StatusNotFound, "proposal_not_found", "proposal was not found"
	case errors.Is(err, proposalworkflow.ErrProposalContext):
		status, code, message = http.StatusConflict, "proposal_context_mismatch", "proposal is not based on the requested scenario context"
	case errors.Is(err, proposalworkflow.ErrProposalState):
		status, code, message = http.StatusConflict, "proposal_state_conflict", "proposal is not in a state that allows this operation"
	case errors.Is(err, proposalworkflow.ErrProposalConflict):
		status, code, message = http.StatusConflict, "proposal_conflict", "proposal changed before the operation completed"
	case errors.Is(err, proposalworkflow.ErrProposalUnauthorized):
		status, code, message = http.StatusForbidden, "proposal_forbidden", "proposal authorization was denied"
	case errors.Is(err, scenarioworkflow.ErrScenarioNotFound):
		status, code, message = http.StatusNotFound, "scenario_not_found", "scenario was not found"
	case strings.Contains(err.Error(), "dependencies are unavailable"):
		status, code, message = http.StatusServiceUnavailable, "proposal_unavailable", "proposal service is unavailable"
	}
	writeError(w, r, status, code, message)
}

func writeApprovalError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusUnprocessableEntity, "approval_rejected", "approval request was rejected"
	switch {
	case errors.Is(err, approvalworkflow.ErrApprovalUnauthorized):
		status, code, message = http.StatusForbidden, "approval_forbidden", "approval authorization was denied"
	case errors.Is(err, approvalworkflow.ErrApprovalContext):
		status, code, message = http.StatusConflict, "approval_context_mismatch", "approval context does not match the current proposal"
	case errors.Is(err, approvalworkflow.ErrApprovalState):
		status, code, message = http.StatusConflict, "approval_state_conflict", "proposal is not awaiting approval"
	case errors.Is(err, approvalworkflow.ErrApprovalStale):
		status, code, message = http.StatusConflict, "stale_proposal", "proposal was created from an outdated business version"
	case strings.Contains(err.Error(), "dependencies are unavailable"):
		status, code, message = http.StatusServiceUnavailable, "approval_unavailable", "approval service is unavailable"
	}
	writeError(w, r, status, code, message)
}

func (s *Server) businessSnapshot(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if s.deps.SnapshotReader == nil {
		writeError(w, r, http.StatusServiceUnavailable, "snapshot_unavailable", "business snapshot service is unavailable")
		return
	}
	businessID := core.ID(r.PathValue("businessId"))
	if err := businessID.Validate("business_id"); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_business_id", "business_id is invalid")
		return
	}
	if !principal.GuestAllows(businessID, "") {
		writeError(w, r, http.StatusForbidden, "guest_scope_forbidden", "guest demo is limited to its configured business")
		return
	}
	snapshot, err := s.deps.SnapshotReader.GetSnapshot(r.Context(), principal.OrganizationID, businessID)
	if err != nil {
		if errors.Is(err, postgres.ErrBusinessNotFound) {
			writeError(w, r, http.StatusNotFound, "business_not_found", "business was not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "snapshot_failed", "business snapshot could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, snapshotDTO(snapshot, requestIDFromContext(r.Context())))
}

func (s *Server) commitProposal(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if !principal.CanCommit() {
		writeError(w, r, http.StatusForbidden, "commit_forbidden", "the current role cannot commit a proposal")
		return
	}
	if s.deps.Committer == nil {
		writeError(w, r, http.StatusServiceUnavailable, "commit_unavailable", "commit service is unavailable")
		return
	}
	proposalID := core.ID(r.PathValue("proposalId"))
	if err := proposalID.Validate("proposal_id"); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_proposal_id", "proposal_id is invalid")
		return
	}
	request, err := decodeCommitRequest(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_commit_request", err.Error())
		return
	}
	versionID := core.ID(strings.TrimSpace(request.ProposalVersionID))
	if err := versionID.Validate("proposal_version_id"); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_proposal_version_id", "proposal_version_id is invalid")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 200 {
		writeError(w, r, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must be between 8 and 200 characters")
		return
	}
	businessID := core.ID(strings.TrimSpace(request.BusinessID))
	if err := businessID.Validate("business_id"); err != nil {
		writeError(w, r, http.StatusBadRequest, "missing_business_id", "business_id is required for commit")
		return
	}
	input := ports.CommitInput{
		OrganizationID: principal.OrganizationID, OperationID: operationID(principal.OrganizationID, idempotencyKey), IdempotencyKey: idempotencyKey,
		BusinessID: businessID, ProposalID: proposalID, ProposalVersionID: versionID, ActorID: principal.UserID,
		RequestID: requestIDFromContext(r.Context()), Authorized: principal.CanCommit(),
	}
	outcome, err := s.deps.Committer.Commit(r.Context(), input)
	if err != nil {
		writeCommitError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"operation_id": outcome.OperationID, "decision_id": outcome.DecisionID, "status": outcome.Status,
		"committed_business_version": outcome.CommittedBusinessVersion, "replayed": outcome.Replayed,
		"request_id": requestIDFromContext(r.Context()),
	})
}

type commitRequest struct {
	ProposalVersionID string `json:"proposal_version_id"`
	BusinessID        string `json:"business_id"`
}

func decodeCommitRequest(r *http.Request) (commitRequest, error) {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request commitRequest
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return commitRequest{}, fmt.Errorf("request body is required")
		}
		return commitRequest{}, fmt.Errorf("request body is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return commitRequest{}, fmt.Errorf("request body must contain one JSON object")
	}
	return request, nil
}

func operationID(organizationID core.ID, idempotencyKey string) core.ID {
	digest := sha256.Sum256([]byte(string(organizationID) + ":commit:" + idempotencyKey))
	return core.ID("operation-" + hex.EncodeToString(digest[:16]))
}

func writeCommitError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusConflict, "commit_rejected", "commit was rejected"
	switch {
	case errors.Is(err, postgres.ErrCommitUnauthorized):
		status, code, message = http.StatusForbidden, "commit_forbidden", "commit authorization was denied"
	case errors.Is(err, postgres.ErrBusinessNotFound):
		status, code, message = http.StatusNotFound, "business_not_found", "business was not found"
	case errors.Is(err, postgres.ErrProposalNotFound):
		status, code, message = http.StatusNotFound, "proposal_not_found", "approved proposal version was not found"
	case errors.Is(err, postgres.ErrApprovalNotFound):
		status, code, message = http.StatusUnprocessableEntity, "approval_required", "a matching human approval is required"
	case errors.Is(err, postgres.ErrCommitInProgress):
		status, code, message = http.StatusConflict, "commit_in_progress", "commit is already in progress"
	case errors.Is(err, postgres.ErrIdempotencyConflict):
		status, code, message = http.StatusConflict, "idempotency_conflict", "idempotency key conflicts with another operation"
	}
	writeError(w, r, status, code, message)
}

type snapshotResponse struct {
	Status     string             `json:"status"`
	BusinessID core.ID            `json:"business_id"`
	Version    uint64             `json:"version"`
	Products   []productResponse  `json:"products"`
	Customers  []customerResponse `json:"customers"`
	Orders     []orderResponse    `json:"orders"`
	Metrics    []metricResponse   `json:"metrics"`
	RequestID  string             `json:"request_id"`
}

type productResponse struct {
	ID             core.ID `json:"id"`
	SKU            string  `json:"sku"`
	Name           string  `json:"name"`
	PriceCents     int64   `json:"price_cents"`
	CostCents      int64   `json:"cost_cents"`
	InventoryUnits int64   `json:"inventory_units"`
	Active         bool    `json:"active"`
	Version        uint64  `json:"version"`
}

type customerResponse struct {
	ID      core.ID `json:"id"`
	Name    string  `json:"name"`
	Segment string  `json:"segment"`
	Active  bool    `json:"active"`
	Version uint64  `json:"version"`
}

type orderResponse struct {
	ID             core.ID `json:"id"`
	CustomerID     core.ID `json:"customer_id"`
	ProductID      core.ID `json:"product_id"`
	Quantity       int64   `json:"quantity"`
	RevenueCents   int64   `json:"revenue_cents"`
	CostCents      int64   `json:"cost_cents"`
	OccurredAtUnix int64   `json:"occurred_at_unix"`
	Version        uint64  `json:"version"`
}

type metricResponse struct {
	Key     string  `json:"key"`
	Value   float64 `json:"value"`
	Unit    string  `json:"unit"`
	Period  string  `json:"period"`
	Source  string  `json:"source"`
	Version uint64  `json:"version"`
}

func snapshotDTO(snapshot business.Snapshot, requestID string) snapshotResponse {
	response := snapshotResponse{
		Status: "ok", BusinessID: snapshot.BusinessID, Version: snapshot.Version,
		Products:  make([]productResponse, 0, len(snapshot.Products)),
		Customers: make([]customerResponse, 0, len(snapshot.Customers)),
		Orders:    make([]orderResponse, 0, len(snapshot.Orders)),
		Metrics:   make([]metricResponse, 0, len(snapshot.Metrics)), RequestID: requestID,
	}
	for _, item := range snapshot.Products {
		response.Products = append(response.Products, productResponse{ID: item.ID, SKU: item.SKU, Name: item.Name, PriceCents: item.PriceCents, CostCents: item.CostCents, InventoryUnits: item.InventoryUnits, Active: item.Active, Version: item.Version})
	}
	for _, item := range snapshot.Customers {
		response.Customers = append(response.Customers, customerResponse{ID: item.ID, Name: item.Name, Segment: item.Segment, Active: item.Active, Version: item.Version})
	}
	for _, item := range snapshot.Orders {
		response.Orders = append(response.Orders, orderResponse{ID: item.ID, CustomerID: item.CustomerID, ProductID: item.ProductID, Quantity: item.Quantity, RevenueCents: item.RevenueCents, CostCents: item.CostCents, OccurredAtUnix: item.OccurredAtUnix, Version: item.Version})
	}
	for _, item := range snapshot.Metrics {
		response.Metrics = append(response.Metrics, metricResponse{Key: item.Key, Value: item.Value, Unit: item.Unit, Period: item.Period, Source: item.Source, Version: item.Version})
	}
	return response
}

func scenarioDTO(candidate scenario.Scenario, requestID string) scenarioResponse {
	actions := candidate.ProposedActions
	if actions == nil {
		actions = []scenario.Action{}
	}
	assumptions := candidate.Assumptions
	if assumptions == nil {
		assumptions = []scenario.Assumption{}
	}
	risks := candidate.Risks
	if risks == nil {
		risks = []scenario.Risk{}
	}
	result := candidate.Result
	if result.BaselineMetrics == nil {
		result.BaselineMetrics = map[string]float64{}
	}
	if result.ProjectedMetrics == nil {
		result.ProjectedMetrics = map[string]float64{}
	}
	if result.MetricDeltas == nil {
		result.MetricDeltas = map[string]float64{}
	}
	if result.Warnings == nil {
		result.Warnings = []string{}
	}
	return scenarioResponse{
		Status: candidate.Status, ID: candidate.ID, GoalID: candidate.GoalID, BusinessID: candidate.BusinessID,
		BaseBusinessVersion: candidate.BaseBusinessVersion, Name: candidate.Name, Objective: candidate.Objective,
		ProposedActions: actions, Assumptions: assumptions, Result: result, Risks: risks,
		Version: candidate.Version, RequestID: requestID,
	}
}

func comparisonDTO(comparison scenario.Comparison, requestID string) comparisonResponse {
	items := make([]comparisonItemResponse, 0, len(comparison.Items))
	for _, item := range comparison.Items {
		risks := item.Risks
		if risks == nil {
			risks = []scenario.Risk{}
		}
		warnings := item.Warnings
		if warnings == nil {
			warnings = []string{}
		}
		items = append(items, comparisonItemResponse{
			ID: item.ID, Name: item.Name, Status: item.Status, ProjectedMetrics: item.ProjectedMetrics,
			MetricDeltas: item.MetricDeltas, Risks: risks, Warnings: warnings,
		})
	}
	warnings := comparison.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return comparisonResponse{
		Items: items, RecommendedScenarioID: comparison.RecommendedScenarioID,
		RecommendationAvailable: comparison.RecommendationAvailable, RecommendationBasis: comparison.RecommendationBasis,
		Warnings: warnings, RequestID: requestID,
	}
}

func proposalDTO(candidate proposal.Proposal, requestID string) proposalResponse {
	changes := candidate.Current.Changes
	if changes == nil {
		changes = []proposal.Change{}
	}
	return proposalResponse{
		ID: candidate.ID, BusinessID: candidate.BusinessID, GoalID: candidate.GoalID, Version: candidate.Version,
		Current: proposalVersionResponse{
			ID: candidate.Current.ID, ProposalID: candidate.Current.ProposalID, Number: candidate.Current.Number,
			ScenarioID: candidate.Current.ScenarioID, BaseBusinessVersion: candidate.Current.BaseBusinessVersion,
			Summary: candidate.Current.Summary, Changes: changes, Status: candidate.Current.Status,
		},
		RequestID: requestID,
	}
}

func approvalDTO(decision approval.Decision, requestID string) approvalDecisionResponse {
	return approvalDecisionResponse{
		ID: decision.ID, ProposalID: decision.ProposalID, ProposalVersionID: decision.ProposalVersionID,
		BusinessID: decision.BusinessID, Status: decision.Status, Reason: decision.Reason,
		DecidedBy: decision.DecidedBy, RequestID: requestID,
	}
}
