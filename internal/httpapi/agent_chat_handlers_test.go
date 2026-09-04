package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webmcp-backend/internal/application/agentworkflow"
	"webmcp-backend/internal/auth"
)

type testAgentChatAPI struct {
	called bool
}

func (a *testAgentChatAPI) Chat(_ context.Context, input agentworkflow.Input) (agentworkflow.Response, error) {
	a.called = true
	return agentworkflow.Response{ConversationID: "conversation-1", Message: "Grounded response", Provider: "deterministic_fallback", Grounded: agentworkflow.VerifiedContext{BusinessID: input.BusinessID, BusinessVersion: 7, GoalID: input.GoalID}, Warnings: []string{}}, nil
}

func TestAgentChatRouteUsesAuthenticatedBoundaryAndStableResponse(t *testing.T) {
	chat := &testAgentChatAPI{}
	c := testServerConfig()
	c.AgentChatEnabled = true
	server := NewServerWithDependencies(c, testLogger(), Dependencies{Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleViewer)}, AgentChat: chat})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/chat", strings.NewReader(`{"business_id":"business-1","goal_id":"goal-1","message":"inspect the pulse","current_stage":"orient","available_tools":["get_business_snapshot"],"recent_messages":[]}`))
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"conversation_id":"conversation-1"`) || !strings.Contains(response.Body.String(), `"request_id"`) {
		t.Fatalf("agent chat contract failed: %d %s", response.Code, response.Body.String())
	}
	if !chat.called {
		t.Fatal("agent chat service was not called")
	}
}

func TestAgentChatRouteEnforcesGuestScope(t *testing.T) {
	c := testServerConfig()
	c.AgentChatEnabled = true
	server := NewServerWithDependencies(c, testLogger(), Dependencies{Authenticator: testAuthenticator{principal: auth.Principal{UserID: "guest-1", OrganizationID: "org-1", Role: auth.RoleViewer, Guest: true, GuestBusinessID: "business-demo", GuestGoalID: "goal-demo"}}, AgentChat: &testAgentChatAPI{}})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/chat", strings.NewReader(`{"business_id":"other-business","goal_id":"goal-demo","message":"inspect","current_stage":"orient"}`))
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "guest_scope_forbidden") {
		t.Fatalf("guest scope was not enforced: %d %s", response.Code, response.Body.String())
	}
}

func TestAgentChatRateLimitProtectsProviderBoundary(t *testing.T) {
	c := testServerConfig()
	c.AgentChatEnabled = true
	server := NewServerWithDependencies(c, testLogger(), Dependencies{Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleViewer)}, AgentChat: &testAgentChatAPI{}})
	body := `{"business_id":"business-1","goal_id":"goal-1","message":"inspect","current_stage":"orient"}`
	for index := 0; index < 30; index++ {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/chat", strings.NewReader(body))
		request.RemoteAddr = "192.0.2.40:1234"
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("chat request %d unexpectedly failed: %d %s", index+1, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/chat", strings.NewReader(body))
	request.RemoteAddr = "192.0.2.40:1234"
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "agent_chat_rate_limited") {
		t.Fatalf("chat rate limit was not enforced: %d %s", response.Code, response.Body.String())
	}
}
