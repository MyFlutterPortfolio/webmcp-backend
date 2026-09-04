package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"webmcp-backend/internal/application/agentworkflow"
	"webmcp-backend/internal/domain/core"
)

type agentChatRequest struct {
	BusinessID     string                           `json:"business_id"`
	GoalID         string                           `json:"goal_id"`
	ConversationID string                           `json:"conversation_id"`
	Message        string                           `json:"message"`
	CurrentStage   string                           `json:"current_stage"`
	AvailableTools []string                         `json:"available_tools"`
	RecentMessages []agentworkflow.ConversationTurn `json:"recent_messages"`
}

func (s *Server) agentChat(w http.ResponseWriter, r *http.Request) {
	if !s.agentChatEnabled {
		writeError(w, r, http.StatusNotFound, "agent_chat_disabled", "agent chat is not enabled on this deployment")
		return
	}
	principal, ok := principalFromRequest(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authenticated principal is required")
		return
	}
	if s.deps.AgentChat == nil {
		writeError(w, r, http.StatusServiceUnavailable, "agent_chat_unavailable", "agent chat service is unavailable")
		return
	}
	if !s.allowAgentChat(clientIP(r)) {
		writeError(w, r, http.StatusTooManyRequests, "agent_chat_rate_limited", "agent chat rate limit reached; try again shortly")
		return
	}
	var request agentChatRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_agent_chat_request", err.Error())
		return
	}
	businessID := core.ID(strings.TrimSpace(request.BusinessID))
	goalID := core.ID(strings.TrimSpace(request.GoalID))
	if !principal.GuestAllows(businessID, goalID) {
		writeError(w, r, http.StatusForbidden, "guest_scope_forbidden", "guest demo is limited to its configured business and goal")
		return
	}
	result, err := s.deps.AgentChat.Chat(r.Context(), agentworkflow.Input{
		OrganizationID: principal.OrganizationID,
		ActorID:        principal.UserID,
		BusinessID:     businessID,
		GoalID:         goalID,
		ConversationID: request.ConversationID,
		Message:        request.Message,
		CurrentStage:   request.CurrentStage,
		AvailableTools: request.AvailableTools,
		RecentMessages: request.RecentMessages,
		Guest:          principal.Guest,
	})
	if err != nil {
		switch {
		case errors.Is(err, agentworkflow.ErrInvalidInput):
			writeError(w, r, http.StatusBadRequest, "invalid_agent_chat_request", "message or chat context is invalid")
		case errors.Is(err, agentworkflow.ErrContextNotFound):
			writeError(w, r, http.StatusNotFound, "agent_context_not_found", "the requested business goal context was not found")
		default:
			writeError(w, r, http.StatusServiceUnavailable, "agent_chat_unavailable", "agent chat could not produce a safe response")
		}
		return
	}
	result.RequestID = requestIDFromContext(r.Context())
	writeJSON(w, http.StatusOK, result)
}
