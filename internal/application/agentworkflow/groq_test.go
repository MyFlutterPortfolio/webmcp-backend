package agentworkflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGroqProviderUsesServerSideKeyAndParsesStructuredResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("server-side API key header missing")
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected Groq endpoint: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"message\":\"Grounded answer\",\"tool_suggestion\":{\"name\":\"get_business_snapshot\",\"arguments\":{},\"reason\":\"Start with evidence\",\"risk\":\"read_only\"}}"}}]}`))
	}))
	defer server.Close()

	provider := NewGroqProviderWithClient("test-secret", "test-model", server.URL, server.Client(), 256)
	result, err := provider.Generate(context.Background(), Prompt{Message: "inspect", CurrentStage: "orient"})
	if err != nil {
		t.Fatalf("Groq provider failed: %v", err)
	}
	if result.Message != "Grounded answer" || result.ToolSuggestion == nil || result.ToolSuggestion.Name != "get_business_snapshot" {
		t.Fatalf("unexpected structured response: %+v", result)
	}
}

func TestGroqProviderRejectsMalformedStructuredResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not-json"}}]}`))
	}))
	defer server.Close()

	provider := NewGroqProviderWithClient("test-secret", "test-model", server.URL, server.Client(), 256)
	if _, err := provider.Generate(context.Background(), Prompt{Message: "inspect"}); err == nil {
		t.Fatal("malformed structured output was accepted")
	} else if !strings.Contains(err.Error(), "structured output") {
		t.Fatalf("unexpected malformed output error: %v", err)
	}
}
