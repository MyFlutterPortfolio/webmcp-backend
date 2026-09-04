package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"webmcp-backend/internal/application/agentworkflow"
	"webmcp-backend/internal/application/analysisworkflow"
	"webmcp-backend/internal/application/approvalworkflow"
	"webmcp-backend/internal/application/planningworkflow"
	"webmcp-backend/internal/application/proposalworkflow"
	"webmcp-backend/internal/application/scenarioworkflow"
	"webmcp-backend/internal/auth"
	"webmcp-backend/internal/config"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/httpapi"
	"webmcp-backend/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	var databasePoolClose func()
	dependencies := httpapi.Dependencies{}
	var authenticators []auth.Authenticator
	if cfg.AuthConfigured() {
		authenticator, authErr := auth.NewJWKSAuthenticator(auth.JWKSConfig{
			Issuer:    cfg.JWTIssuer,
			Audience:  cfg.JWTAudience,
			JWKSURL:   cfg.JWKSURL,
			ClockSkew: cfg.JWTClockSkew,
		})
		if authErr != nil {
			logger.Error("authentication startup failed", "error", authErr)
			os.Exit(1)
		}
		authenticators = append(authenticators, authenticator)
	}
	if cfg.GuestDemoConfigured() {
		guestAuthenticator, guestErr := auth.NewGuestAuthenticator(cfg.GuestDemoSecret, core.ID(cfg.GuestUserID), core.ID(cfg.GuestOrganizationID), core.ID(cfg.GuestBusinessID), core.ID(cfg.GuestGoalID), cfg.GuestTokenTTL)
		if guestErr != nil {
			logger.Error("guest demo startup failed", "error", guestErr)
			os.Exit(1)
		}
		authenticators = append(authenticators, guestAuthenticator)
		dependencies.GuestIssuer = guestAuthenticator
	}
	if len(authenticators) > 0 {
		dependencies.Authenticator = auth.ChainAuthenticators(authenticators...)
	}
	if cfg.DatabaseURL != "" {
		databaseContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pool, openErr := postgres.Open(databaseContext, cfg, logger)
		cancel()
		if openErr != nil {
			logger.Error("database startup failed", "error", openErr)
			os.Exit(1)
		}
		databasePoolClose = pool.Close
		dependencies.Readiness = pool.Ping
		dependencies.Authorizer = postgres.PrincipalAuthorizer{Pool: pool}
		if cfg.GuestDemoConfigured() {
			dependencies.Readiness = func(ctx context.Context) error {
				if err := pool.Ping(ctx); err != nil {
					return err
				}
				return postgres.CheckGuestDemo(ctx, pool, core.ID(cfg.GuestOrganizationID), core.ID(cfg.GuestUserID), core.ID(cfg.GuestBusinessID), core.ID(cfg.GuestGoalID))
			}
		}
		dependencies.SnapshotReader = postgres.BusinessRepository{Pool: pool}
		dependencies.Committer = postgres.CommitService{Pool: pool}
		scenarioRepository := postgres.ScenarioRepository{Pool: pool}
		dependencies.Analysis = analysisworkflow.NewService(dependencies.SnapshotReader, scenarioRepository)
		dependencies.Constraints = planningworkflow.NewService(scenarioRepository)
		dependencies.Scenarios = scenarioworkflow.NewService(dependencies.SnapshotReader, scenarioRepository, scenarioRepository)
		proposalRepository := postgres.ProposalRepository{Pool: pool}
		dependencies.Proposals = proposalworkflow.NewService(scenarioRepository, proposalRepository)
		approvalRepository := postgres.ApprovalRepository{Pool: pool}
		dependencies.Approvals = approvalworkflow.NewService(approvalRepository, nil)
		var primaryAgent agentworkflow.Provider
		if cfg.GeminiAPIKey != "" {
			primaryAgent = agentworkflow.NewGeminiProvider(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL, 9*time.Second, cfg.GeminiMaxOutputTokens)
		}
		dependencies.AgentChat = agentworkflow.NewService(dependencies.SnapshotReader, scenarioRepository, primaryAgent, agentworkflow.DeterministicProvider{})
	}
	if databasePoolClose != nil {
		defer databasePoolClose()
	}

	server := httpapi.NewServerWithDependencies(cfg, logger, dependencies)
	server.SetReady(dependencies.Authenticator != nil && dependencies.Readiness != nil &&
		dependencies.SnapshotReader != nil && dependencies.Committer != nil &&
		dependencies.Scenarios != nil && dependencies.Proposals != nil && dependencies.Approvals != nil &&
		dependencies.Analysis != nil && dependencies.Constraints != nil &&
		(!cfg.AgentChatEnabled || dependencies.AgentChat != nil))

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "addr", cfg.HTTPAddr, "environment", cfg.Environment)
		serverErr <- server.ListenAndServe()
	}()

	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		if !httpapi.IsExpectedShutdown(err) {
			logger.Error("http server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case <-shutdownContext.Done():
		logger.Info("shutdown signal received")
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		logger.Info("http server stopped")
	}
}
