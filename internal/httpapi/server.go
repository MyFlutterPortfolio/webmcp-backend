package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"webmcp-backend/internal/config"
)

type Server struct {
	httpServer       *http.Server
	ready            atomic.Bool
	deps             Dependencies
	guestIssueMu     sync.Mutex
	guestIssues      map[string][]time.Time
	agentChatEnabled bool
	agentChatMu      sync.Mutex
	agentChatIssues  map[string][]time.Time
}

type ReadinessCheck func(context.Context) error

type GuestSessionIssuer interface {
	IssueGuestToken() (string, time.Time, error)
	GuestBusinessID() string
	GuestGoalID() string
}

const readinessTimeout = 5 * time.Second

func NewServer(cfg config.Config, logger *slog.Logger) *Server {
	return NewServerWithDependencies(cfg, logger, Dependencies{})
}

func NewServerWithDependencies(cfg config.Config, logger *slog.Logger, deps Dependencies) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	mux := http.NewServeMux()
	server := &Server{deps: deps, guestIssues: make(map[string][]time.Time), agentChatEnabled: cfg.AgentChatEnabled, agentChatIssues: make(map[string][]time.Time)}

	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.readyCheck)
	mux.HandleFunc("POST /api/v1/guest/session", server.guestSession)
	requireAuthorizer := cfg.Environment == "production" || cfg.Environment == "staging"
	protected := func(next http.Handler) http.Handler {
		return authenticated(deps.Authenticator, deps.Authorizer, requireAuthorizer, next)
	}
	mux.Handle("POST /api/v1/agent/chat", protected(http.HandlerFunc(server.agentChat)))
	mux.Handle("GET /api/v1/businesses/{businessId}/snapshot", protected(http.HandlerFunc(server.businessSnapshot)))
	mux.Handle("POST /api/v1/analysis", protected(http.HandlerFunc(server.analyzeBusiness)))
	mux.Handle("POST /api/v1/goals/{goalId}/constraints", protected(http.HandlerFunc(server.updateConstraint)))
	mux.Handle("POST /api/v1/scenarios", protected(http.HandlerFunc(server.createScenario)))
	mux.Handle("POST /api/v1/scenarios/{scenarioId}/simulation", protected(http.HandlerFunc(server.simulateScenario)))
	mux.Handle("POST /api/v1/scenarios/compare", protected(http.HandlerFunc(server.compareScenarios)))
	mux.Handle("POST /api/v1/proposals", protected(http.HandlerFunc(server.createProposal)))
	mux.Handle("POST /api/v1/proposals/{proposalId}/revisions", protected(http.HandlerFunc(server.reviseProposal)))
	mux.Handle("POST /api/v1/proposals/{proposalId}/review", protected(http.HandlerFunc(server.requestHumanReview)))
	mux.Handle("POST /api/v1/proposals/{proposalId}/approval", protected(http.HandlerFunc(server.decideApproval)))
	mux.Handle("POST /api/v1/proposals/{proposalId}/commit", protected(http.HandlerFunc(server.commitProposal)))
	mux.Handle("/api/v1/", protected(http.HandlerFunc(server.notFound)))
	mux.HandleFunc("/", server.notFound)

	var handler http.Handler = mux
	handler = bodyLimitMiddleware(cfg.MaxBodyBytes, handler)
	handler = corsMiddleware(cfg.AllowedOrigins, handler)
	handler = recoverMiddleware(logger, handler)
	handler = securityHeadersMiddleware(handler)
	handler = requestIDMiddleware(handler)
	handler = accessLogMiddleware(logger, handler)

	server.httpServer = &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.RequestTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
	return server
}

func (s *Server) guestSession(w http.ResponseWriter, r *http.Request) {
	issuer, ok := s.deps.GuestIssuer.(GuestSessionIssuer)
	if !ok || issuer == nil {
		writeError(w, r, http.StatusNotFound, "guest_demo_disabled", "guest demo is not enabled")
		return
	}
	if !s.allowGuestIssue(clientIP(r)) {
		writeError(w, r, http.StatusTooManyRequests, "guest_rate_limited", "guest demo session limit reached; try again later")
		return
	}
	if s.deps.Readiness != nil {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()
		if err := s.deps.Readiness(ctx); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "guest_unavailable", "guest demo is temporarily unavailable")
			return
		}
	}
	token, expiresAt, err := issuer.IssueGuestToken()
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "guest_unavailable", "guest demo is temporarily unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": token, "token_type": "Bearer", "expires_at": expiresAt.UTC().Format(time.RFC3339), "business_id": issuer.GuestBusinessID(), "goal_id": issuer.GuestGoalID(), "mode": "guest"})
}

func clientIP(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func (s *Server) allowGuestIssue(ip string) bool {
	now := time.Now()
	cutoff := now.Add(-time.Hour)
	s.guestIssueMu.Lock()
	defer s.guestIssueMu.Unlock()
	issues := s.guestIssues[ip][:0]
	for _, at := range s.guestIssues[ip] {
		if at.After(cutoff) {
			issues = append(issues, at)
		}
	}
	if len(issues) >= 12 {
		s.guestIssues[ip] = issues
		return false
	}
	issues = append(issues, now)
	s.guestIssues[ip] = issues
	return true
}

func (s *Server) allowAgentChat(ip string) bool {
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	s.agentChatMu.Lock()
	defer s.agentChatMu.Unlock()
	issues := s.agentChatIssues[ip][:0]
	for _, at := range s.agentChatIssues[ip] {
		if at.After(cutoff) {
			issues = append(issues, at)
		}
	}
	if len(issues) >= 30 {
		s.agentChatIssues[ip] = issues
		return false
	}
	s.agentChatIssues[ip] = append(issues, now)
	return true
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.ready.Store(false)
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) SetReady(value bool) {
	s.ready.Store(value)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"service":    "webmcp-backend",
		"request_id": requestIDFromContext(r.Context()),
	})
}

func (s *Server) readyCheck(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		writeError(w, r, http.StatusServiceUnavailable, "not_ready", "service is not ready")
		return
	}
	if s.deps.Readiness != nil {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()
		if err := s.deps.Readiness(ctx); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "dependency_not_ready", "required dependency is not ready")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ready",
		"service":    "webmcp-backend",
		"request_id": requestIDFromContext(r.Context()),
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, "not_found", "route not found")
}

func IsExpectedShutdown(err error) bool {
	return errors.Is(err, http.ErrServerClosed)
}
