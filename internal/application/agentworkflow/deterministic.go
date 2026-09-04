package agentworkflow

import (
	"context"
	"fmt"
	"strings"
)

// DeterministicProvider keeps the deployed judge path useful when a provider
// key is absent or temporarily unavailable. It is intentionally conservative:
// it can guide and suggest typed read/working capabilities, never approval or
// commit, and never invents business numbers.
type DeterministicProvider struct{}

func (DeterministicProvider) Label() string { return "deterministic_fallback" }

func (DeterministicProvider) Generate(_ context.Context, prompt Prompt) (ProviderResponse, error) {
	message := "I can work with the verified business context, analyze evidence, model bounded alternatives, and prepare a reviewable proposal. Ask me to inspect the business pulse or investigate a specific growth question."
	var suggestion *ToolSuggestion
	text := strings.ToLower(strings.TrimSpace(prompt.Message))
	allowed := make(map[string]struct{}, len(prompt.AllowedTools))
	for _, tool := range prompt.AllowedTools {
		allowed[tool.Name] = struct{}{}
	}
	has := func(name string) bool { _, ok := allowed[name]; return ok }

	switch {
	case strings.Contains(text, "commit") || strings.Contains(text, "approve"):
		message = "I will not approve or commit a decision from chat. Review the exact proposal, record a human reason, confirm the commit gate, and let the backend verify the resulting business version."
	case (strings.Contains(text, "review") || strings.Contains(text, "submit")) && has("request_human_review"):
		message = "The current workflow can submit the exact proposal version for human review. The review record is separate from this conversation and does not commit business state."
		suggestion = &ToolSuggestion{Name: "request_human_review", Arguments: map[string]any{}, Reason: "You asked to move the current working proposal to the human review boundary."}
	case (strings.Contains(text, "scenario") || strings.Contains(text, "alternative") || strings.Contains(text, "retention") || strings.Contains(text, "price")) && has("create_scenario"):
		if strings.Contains(text, "price") {
			message = "I can create a bounded price-lift alternative for deterministic simulation. It will remain non-canonical until you review and approve a proposal."
			suggestion = &ToolSuggestion{Name: "create_scenario", Arguments: map[string]any{"name": "Selective price lift", "objective": "Increase contribution margin on the strongest sellers while keeping demand risk visible.", "proposed_actions": []any{map[string]any{"type": "price_adjustment", "value": 5, "unit": "percent"}}}, Reason: "A typed scenario is the next safe capability for this exploration step."}
		} else {
			message = "I can create a bounded retention alternative for deterministic simulation. It will remain non-canonical until you review and approve a proposal."
			suggestion = &ToolSuggestion{Name: "create_scenario", Arguments: map[string]any{"name": "Focused retention", "objective": "Grow repeat revenue without increasing stock pressure.", "proposed_actions": []any{map[string]any{"type": "marketing_budget", "value": 8000, "unit": "cents"}}}, Reason: "A typed scenario is the next safe capability for this exploration step."}
		}
	case strings.Contains(text, "analy") || strings.Contains(text, "evidence") || strings.Contains(text, "opportun") || strings.Contains(text, "growth"):
		if has("analyze_business") {
			message = "I will use the verified snapshot and current goal to produce evidence, signals, trade-offs, and guardrails. The calculation remains deterministic in the backend."
			suggestion = &ToolSuggestion{Name: "analyze_business", Arguments: map[string]any{"focus": prompt.Message}, Reason: "Your question is an evidence-analysis request for the current business goal."}
		} else if has("get_business_snapshot") {
			suggestion = &ToolSuggestion{Name: "get_business_snapshot", Arguments: map[string]any{}, Reason: "The current stage exposes business context before deeper analysis."}
		}
	default:
		if has("get_business_snapshot") {
			message = "The workspace is grounded at business version " + fmt.Sprint(prompt.Context.BusinessVersion) + ". I can read the business pulse, investigate evidence, compare bounded scenarios, and prepare a proposal while you retain approval and commit authority."
			suggestion = &ToolSuggestion{Name: "get_business_snapshot", Arguments: map[string]any{}, Reason: "Start with the authoritative business snapshot before taking a strategic step."}
		}
	}
	return ProviderResponse{Message: message, ToolSuggestion: suggestion}, nil
}
