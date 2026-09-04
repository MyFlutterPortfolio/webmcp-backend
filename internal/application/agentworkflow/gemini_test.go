package agentworkflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiProviderUsesServerSideKeyAndParsesStructuredResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-secret" {
			t.Errorf("server-side API key header missing")
		}
		if !strings.HasSuffix(r.URL.Path, "/models/test-model:generateContent") {
			t.Errorf("unexpected Gemini endpoint: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"message\":\"Grounded answer\",\"tool_suggestion\":{\"name\":\"get_business_snapshot\",\"arguments\":{},\"reason\":\"Start with evidence\",\"risk\":\"read_only\"}}"}]}}]}`))
	}))
	defer server.Close()

	provider := NewGeminiProviderWithClient("test-secret", "test-model", server.URL, server.Client(), 256)
	result, err := provider.Generate(context.Background(), Prompt{Message: "inspect", CurrentStage: "orient"})
	if err != nil {
		t.Fatalf("Gemini provider failed: %v", err)
	}
	if result.Message != "Grounded answer" || result.ToolSuggestion == nil || result.ToolSuggestion.Name != "get_business_snapshot" {
		t.Fatalf("unexpected structured response: %+v", result)
	}
}

func TestGeminiProviderRejectsMalformedStructuredResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"not-json"}]}}]}`))
	}))
	defer server.Close()

	provider := NewGeminiProviderWithClient("test-secret", "test-model", server.URL, server.Client(), 256)
	if _, err := provider.Generate(context.Background(), Prompt{Message: "inspect"}); err == nil {
		t.Fatal("malformed structured output was accepted")
	}
}
