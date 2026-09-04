package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webmcp-backend/internal/config"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	c := config.Config{
		Environment:       "test",
		HTTPAddr:          ":8080",
		ShutdownTimeout:   time.Second,
		RequestTimeout:    time.Second,
		MaxBodyBytes:      1024,
		AllowedOrigins:    []string{"https://app.example.com"},
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
	}
	return NewServer(c, slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil)))
}

type testGuestIssuer struct{}

func (testGuestIssuer) IssueGuestToken() (string, time.Time, error) {
	return "guest-token", time.Now().UTC().Add(15 * time.Minute), nil
}

func (testGuestIssuer) GuestBusinessID() string { return "business-demo" }
func (testGuestIssuer) GuestGoalID() string     { return "goal-demo" }

func TestGuestSessionIsScopedAndRateLimited(t *testing.T) {
	server := NewServerWithDependencies(testServerConfig(), slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil)), Dependencies{
		GuestIssuer: testGuestIssuer{},
	})

	for index := 0; index < 12; index++ {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/guest/session", nil)
		request.RemoteAddr = "192.0.2.10:1234"
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"mode":"guest"`) {
			t.Fatalf("guest session %d was not issued: %d %s", index+1, response.Code, response.Body.String())
		}
	}

	limitedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/guest/session", nil)
	limitedRequest.RemoteAddr = "192.0.2.10:1234"
	limitedResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(limitedResponse, limitedRequest)
	if limitedResponse.Code != http.StatusTooManyRequests {
		t.Fatalf("guest session limit was not enforced: %d %s", limitedResponse.Code, limitedResponse.Body.String())
	}
}

func TestGuestSessionDoesNotIssueTokenWhenDependencyIsNotReady(t *testing.T) {
	server := NewServerWithDependencies(testServerConfig(), slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil)), Dependencies{
		GuestIssuer: testGuestIssuer{},
		Readiness:   func(context.Context) error { return errors.New("demo data unavailable") },
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/guest/session", nil)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "guest_unavailable") {
		t.Fatalf("guest token was issued before readiness: %d %s", response.Code, response.Body.String())
	}
}

func TestHealthAndReadiness(t *testing.T) {
	server := testServer(t)
	server.SetReady(false)

	readyRequest := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readyResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(readyResponse, readyRequest)
	if readyResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected not ready status, got %d", readyResponse.Code)
	}

	server.SetReady(true)
	healthRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("expected health status, got %d", healthResponse.Code)
	}
	if healthResponse.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}
	if healthResponse.Header().Get("X-Request-ID") == "" {
		t.Fatal("request id missing")
	}
	if healthResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("sensitive API cache protection is missing")
	}
}

func TestReadinessChecksConfiguredDependency(t *testing.T) {
	server := NewServerWithDependencies(testServerConfig(), slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil)), Dependencies{
		Readiness: func(context.Context) error { return errors.New("database unavailable") },
	})
	server.SetReady(true)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "dependency_not_ready") {
		t.Fatalf("unhealthy dependency was reported ready: %d %s", response.Code, response.Body.String())
	}
}

func TestRequestIDRejectsUnsafeInputAndPreservesSafeInput(t *testing.T) {
	server := testServer(t)
	server.SetReady(true)

	unsafe := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	unsafe.Header.Set("X-Request-ID", "bad value")
	unsafeResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(unsafeResponse, unsafe)
	if unsafeResponse.Header().Get("X-Request-ID") == "bad value" {
		t.Fatal("unsafe request id was accepted")
	}

	safe := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	safe.Header.Set("X-Request-ID", "demo-123")
	safeResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(safeResponse, safe)
	if got := safeResponse.Header().Get("X-Request-ID"); got != "demo-123" {
		t.Fatalf("safe request id was not preserved: %q", got)
	}
}

func TestCORSOnlyAllowsConfiguredOrigin(t *testing.T) {
	server := testServer(t)

	allowed := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	allowed.Header.Set("Origin", "https://app.example.com")
	allowed.Header.Set("Access-Control-Request-Method", http.MethodGet)
	allowedResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(allowedResponse, allowed)
	if allowedResponse.Code != http.StatusNoContent {
		t.Fatalf("expected allowed preflight, got %d", allowedResponse.Code)
	}
	if allowedResponse.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatal("allowed origin was not returned")
	}

	badMethod := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	badMethod.Header.Set("Origin", "https://app.example.com")
	badMethod.Header.Set("Access-Control-Request-Method", http.MethodPut)
	badMethodResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(badMethodResponse, badMethod)
	if badMethodResponse.Code != http.StatusForbidden {
		t.Fatalf("expected denied CORS method, got %d", badMethodResponse.Code)
	}

	badHeader := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	badHeader.Header.Set("Origin", "https://app.example.com")
	badHeader.Header.Set("Access-Control-Request-Method", http.MethodPost)
	badHeader.Header.Set("Access-Control-Request-Headers", "X-Not-Allowed")
	badHeaderResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(badHeaderResponse, badHeader)
	if badHeaderResponse.Code != http.StatusForbidden {
		t.Fatalf("expected denied CORS header, got %d", badHeaderResponse.Code)
	}

	denied := httptest.NewRequest(http.MethodOptions, "/healthz", strings.NewReader(""))
	denied.Header.Set("Origin", "https://evil.example.com")
	deniedResponse := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(deniedResponse, denied)
	if deniedResponse.Code != http.StatusForbidden {
		t.Fatalf("expected denied preflight, got %d", deniedResponse.Code)
	}
}

func TestUnknownRouteUsesSafeJSONError(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected not found, got %d", response.Code)
	}
	if response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", response.Header().Get("Content-Type"))
	}
}
