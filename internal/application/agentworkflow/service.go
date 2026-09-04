package agentworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"webmcp-backend/internal/application/ports"
	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
)

var (
	ErrInvalidInput       = errors.New("agent chat input is invalid")
	ErrContextUnavailable = errors.New("agent chat context is unavailable")
	ErrContextNotFound    = errors.New("agent chat context was not found")
)

// API is the application boundary for the human-facing agent conversation.
// It is deliberately separate from WebMCP: chat can recommend a semantic
// capability, but only the browser registry can execute that capability.
type API interface {
	Chat(context.Context, Input) (Response, error)
}

type Provider interface {
	Generate(context.Context, Prompt) (ProviderResponse, error)
}

type labeledProvider interface {
	Provider
	Label() string
}

type Input struct {
	OrganizationID core.ID
	ActorID        core.ID
	BusinessID     core.ID
	GoalID         core.ID
	ConversationID string
	Message        string
	CurrentStage   string
	AvailableTools []string
	RecentMessages []ConversationTurn
	Guest          bool
}

type ConversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolSuggestion struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Reason    string         `json:"reason"`
	Risk      string         `json:"risk"`
}

type MetricFact struct {
	Key    string  `json:"key"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
	Period string  `json:"period"`
}

type GuardrailFact struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Hard  bool   `json:"hard"`
}

// VerifiedContext is intentionally a compact, non-PII projection of the
// authorized business state. It gives the model evidence without forwarding
// customer names, raw orders or database-shaped records.
type VerifiedContext struct {
	BusinessID          core.ID         `json:"business_id"`
	BusinessVersion     uint64          `json:"business_version"`
	ProductCount        int             `json:"product_count"`
	ActiveProductCount  int             `json:"active_product_count"`
	CustomerCount       int             `json:"customer_count"`
	RepeatCustomerCount int             `json:"repeat_customer_count"`
	OrderCount          int             `json:"order_count"`
	RevenueCents        int64           `json:"revenue_cents"`
	MarginCents         int64           `json:"margin_cents"`
	Metrics             []MetricFact    `json:"metrics"`
	GoalID              core.ID         `json:"goal_id"`
	GoalVersion         uint64          `json:"goal_version"`
	Objective           string          `json:"objective"`
	TimeHorizonDays     int             `json:"time_horizon_days"`
	Preference          string          `json:"preference"`
	Guardrails          []GuardrailFact `json:"guardrails"`
}

type Response struct {
	ConversationID string          `json:"conversation_id"`
	Message        string          `json:"message"`
	Provider       string          `json:"provider"`
	Grounded       VerifiedContext `json:"grounded_context"`
	ToolSuggestion *ToolSuggestion `json:"tool_suggestion,omitempty"`
	Warnings       []string        `json:"warnings"`
	RequestID      string          `json:"request_id,omitempty"`
}

type ToolDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Risk        string `json:"risk"`
}

type Prompt struct {
	Message      string
	CurrentStage string
	History      []ConversationTurn
	Context      VerifiedContext
	AllowedTools []ToolDescriptor
	Guest        bool
}

type ProviderResponse struct {
	Message        string
	ToolSuggestion *ToolSuggestion
}

type Service struct {
	snapshots ports.BusinessSnapshotReader
	goals     ports.GoalReader
	primary   Provider
	fallbacks []Provider
}

// NewService creates a provider chain. The primary is attempted first, then
// each fallback in order. A deterministic provider should be the final
// fallback so the judge path remains useful without an external AI secret.
func NewService(snapshots ports.BusinessSnapshotReader, goals ports.GoalReader, primary Provider, fallbacks ...Provider) Service {
	return Service{snapshots: snapshots, goals: goals, primary: primary, fallbacks: fallbacks}
}

func (s Service) Chat(ctx context.Context, input Input) (Response, error) {
	if err := validateInput(input); err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if s.snapshots == nil || s.goals == nil {
		return Response{}, ErrContextUnavailable
	}

	snapshot, err := s.snapshots.GetSnapshot(ctx, input.OrganizationID, input.BusinessID)
	if err != nil {
		return Response{}, fmt.Errorf("%w: business context read failed", ErrContextUnavailable)
	}
	goal, err := s.goals.GetGoal(ctx, input.OrganizationID, input.BusinessID, input.GoalID)
	if err != nil {
		return Response{}, fmt.Errorf("%w: goal context read failed", ErrContextUnavailable)
	}
	if snapshot.BusinessID != input.BusinessID || goal.BusinessID != input.BusinessID || goal.ID != input.GoalID {
		return Response{}, ErrContextNotFound
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID == "" {
		conversationID, err = newConversationID()
		if err != nil {
			return Response{}, fmt.Errorf("create conversation id: %w", err)
		}
	}
	contextProjection := projectContext(snapshot, goal)
	allowed := allowedDescriptors(input.CurrentStage, input.AvailableTools)
	prompt := Prompt{Message: strings.TrimSpace(input.Message), CurrentStage: canonicalStage(input.CurrentStage), History: boundedHistory(input.RecentMessages), Context: contextProjection, AllowedTools: allowed, Guest: input.Guest}

	providerName := "deterministic_fallback"
	var generated ProviderResponse
	providers := make([]Provider, 0, 1+len(s.fallbacks))
	if s.primary != nil {
		providers = append(providers, s.primary)
	}
	providers = append(providers, s.fallbacks...)
	usedProviderIndex := -1
	for index, provider := range providers {
		if provider == nil {
			continue
		}
		candidate, providerErr := provider.Generate(ctx, prompt)
		if errors.Is(providerErr, context.Canceled) || errors.Is(providerErr, context.DeadlineExceeded) {
			return Response{}, providerErr
		}
		if providerErr != nil || strings.TrimSpace(candidate.Message) == "" {
			continue
		}
		generated = candidate
		providerName = providerLabel(provider)
		usedProviderIndex = index
		break
	}
	if usedProviderIndex < 0 {
		return Response{}, fmt.Errorf("%w: agent response was empty", ErrContextUnavailable)
	}

	result := Response{
		ConversationID: conversationID,
		Message:        limitText(generated.Message, 6000),
		Provider:       providerName,
		Grounded:       contextProjection,
		Warnings:       []string{"The agent is advisory. No business state changed in this chat."},
	}
	if providerName == "deterministic_fallback" && s.primary != nil {
		result.Warnings = append(result.Warnings, "Groq was unavailable for this turn; the safe deterministic guide answered instead.")
	} else if usedProviderIndex > 0 && providerName == "groq" {
		result.Warnings = append(result.Warnings, "The primary Groq model was unavailable; a configured fallback model answered instead.")
	}
	if generated.ToolSuggestion != nil {
		suggestion, suggestionErr := validateSuggestion(*generated.ToolSuggestion, input, allowed)
		if suggestionErr == nil {
			result.ToolSuggestion = &suggestion
		} else {
			result.Warnings = append(result.Warnings, "The suggested capability was withheld because its typed contract or workflow gate was not satisfied.")
		}
	}
	return result, nil
}

func providerLabel(provider Provider) string {
	if labeled, ok := provider.(labeledProvider); ok && strings.TrimSpace(labeled.Label()) != "" {
		return strings.TrimSpace(labeled.Label())
	}
	return "agent_provider"
}

func validateInput(input Input) error {
	for label, id := range map[string]core.ID{"organization_id": input.OrganizationID, "actor_id": input.ActorID, "business_id": input.BusinessID, "goal_id": input.GoalID} {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	if strings.TrimSpace(input.Message) == "" || len([]rune(input.Message)) > 4000 {
		return fmt.Errorf("message must contain between 1 and 4000 characters")
	}
	if len(input.ConversationID) > 200 || len(input.CurrentStage) > 40 || len(input.AvailableTools) > 24 || len(input.RecentMessages) > 8 {
		return fmt.Errorf("chat context exceeds its bounds")
	}
	for _, name := range input.AvailableTools {
		if strings.TrimSpace(name) == "" || len([]rune(name)) > 100 {
			return fmt.Errorf("available tool name is invalid")
		}
	}
	for _, turn := range input.RecentMessages {
		if turn.Role != "user" && turn.Role != "assistant" {
			return fmt.Errorf("recent message role is invalid")
		}
		if strings.TrimSpace(turn.Content) == "" || len([]rune(turn.Content)) > 2000 {
			return fmt.Errorf("recent message content is invalid")
		}
	}
	return nil
}

func projectContext(snapshot business.Snapshot, goal planning.Goal) VerifiedContext {
	result := VerifiedContext{BusinessID: snapshot.BusinessID, BusinessVersion: snapshot.Version, ProductCount: len(snapshot.Products), CustomerCount: len(snapshot.Customers), OrderCount: len(snapshot.Orders), GoalID: goal.ID, GoalVersion: goal.Version, Objective: goal.Objective, TimeHorizonDays: goal.TimeHorizonDays, Preference: string(goal.Preference), Metrics: make([]MetricFact, 0, min(len(snapshot.Metrics), 12)), Guardrails: make([]GuardrailFact, 0, min(len(goal.Constraints), 12))}
	for _, product := range snapshot.Products {
		if product.Active {
			result.ActiveProductCount++
		}
	}
	for _, customer := range snapshot.Customers {
		if customer.Active && strings.Contains(strings.ToLower(customer.Segment), "repeat") {
			result.RepeatCustomerCount++
		}
	}
	for _, order := range snapshot.Orders {
		result.RevenueCents += order.RevenueCents
		result.MarginCents += order.RevenueCents - order.CostCents
	}
	for _, metric := range snapshot.Metrics {
		if len(result.Metrics) == 12 {
			break
		}
		result.Metrics = append(result.Metrics, MetricFact{Key: metric.Key, Value: metric.Value, Unit: metric.Unit, Period: metric.Period})
	}
	for _, constraint := range goal.Constraints {
		if len(result.Guardrails) == 12 {
			break
		}
		result.Guardrails = append(result.Guardrails, GuardrailFact{Key: constraint.Key, Value: constraint.Value, Hard: constraint.Hard})
	}
	return result
}

func allowedDescriptors(stage string, available []string) []ToolDescriptor {
	allowedNames := stageToolNames(canonicalStage(stage))
	availableSet := make(map[string]struct{}, len(available))
	for _, name := range available {
		availableSet[strings.TrimSpace(name)] = struct{}{}
	}
	filterByRegistry := len(availableSet) > 0
	result := make([]ToolDescriptor, 0, len(allowedNames))
	for _, name := range allowedNames {
		if filterByRegistry {
			if _, ok := availableSet[name]; !ok {
				continue
			}
		}
		result = append(result, ToolDescriptor{Name: name, Description: toolDescription(name), Risk: toolRisk(name)})
	}
	return result
}

func validateSuggestion(suggestion ToolSuggestion, input Input, allowed []ToolDescriptor) (ToolSuggestion, error) {
	name := strings.TrimSpace(suggestion.Name)
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, descriptor := range allowed {
		allowedSet[descriptor.Name] = struct{}{}
	}
	if name == "" || name == "commit_approved_proposal" {
		return ToolSuggestion{}, errors.New("consequential or empty suggestion is not chat-executable")
	}
	if _, ok := allowedSet[name]; !ok {
		return ToolSuggestion{}, errors.New("tool is not exposed for the current workflow stage")
	}
	args := suggestion.Arguments
	if args == nil {
		args = map[string]any{}
	}
	switch name {
	case "get_business_snapshot", "request_human_review", "verify_committed_result":
		if len(args) != 0 {
			return ToolSuggestion{}, errors.New("tool does not accept arguments")
		}
	case "analyze_business":
		var value struct {
			Focus string `json:"focus"`
		}
		if err := strictArgs(args, &value); err != nil || strings.TrimSpace(value.Focus) == "" || len([]rune(value.Focus)) > 1000 {
			return ToolSuggestion{}, errors.New("analysis focus is invalid")
		}
		args = map[string]any{"focus": strings.TrimSpace(value.Focus)}
	case "run_scenario":
		var value struct {
			ScenarioID string `json:"scenario_id"`
		}
		if err := strictArgs(args, &value); err != nil || !validText(value.ScenarioID, 200) {
			return ToolSuggestion{}, errors.New("scenario id is invalid")
		}
		args = map[string]any{"scenario_id": strings.TrimSpace(value.ScenarioID)}
	case "compare_scenarios":
		var value struct {
			ScenarioIDs []string `json:"scenario_ids"`
		}
		if err := strictArgs(args, &value); err != nil || len(value.ScenarioIDs) < 2 || len(value.ScenarioIDs) > 8 {
			return ToolSuggestion{}, errors.New("scenario ids are invalid")
		}
		for _, id := range value.ScenarioIDs {
			if !validText(id, 200) {
				return ToolSuggestion{}, errors.New("scenario id is invalid")
			}
		}
		args = map[string]any{"scenario_ids": value.ScenarioIDs}
	case "create_scenario":
		var value struct {
			Name            string            `json:"name"`
			Objective       string            `json:"objective"`
			ProposedActions []json.RawMessage `json:"proposed_actions"`
		}
		if err := strictArgs(args, &value); err != nil || !validText(value.Name, 120) || !validText(value.Objective, 1000) || len(value.ProposedActions) == 0 || len(value.ProposedActions) > 8 {
			return ToolSuggestion{}, errors.New("scenario arguments are invalid")
		}
		for _, raw := range value.ProposedActions {
			var action struct {
				Type     string  `json:"type"`
				TargetID string  `json:"target_id"`
				Value    float64 `json:"value"`
				Unit     string  `json:"unit"`
			}
			if json.Unmarshal(raw, &action) != nil || (action.Type != "price_adjustment" && action.Type != "inventory_replenishment" && action.Type != "marketing_budget") || (action.Unit != "percent" && action.Unit != "units" && action.Unit != "cents") || (action.TargetID != "" && !validText(action.TargetID, 200)) || action.Value < 0 || action.Value > 1000000000 {
				return ToolSuggestion{}, errors.New("scenario action is invalid")
			}
		}
	case "draft_proposal":
		var value struct {
			ScenarioID string `json:"scenario_id"`
			Summary    string `json:"summary"`
		}
		if err := strictArgs(args, &value); err != nil || !validText(value.ScenarioID, 200) || !validText(value.Summary, 2000) {
			return ToolSuggestion{}, errors.New("proposal arguments are invalid")
		}
	case "revise_proposal":
		var value struct {
			Feedback string `json:"feedback"`
		}
		if err := strictArgs(args, &value); err != nil || !validText(value.Feedback, 2000) {
			return ToolSuggestion{}, errors.New("proposal feedback is invalid")
		}
	}
	risk := toolRisk(name)
	if risk == "human_gate" || input.Guest {
		if name == "commit_approved_proposal" {
			return ToolSuggestion{}, errors.New("guest or chat cannot execute commit")
		}
	}
	reason := limitText(suggestion.Reason, 500)
	if reason == "" {
		reason = "This typed capability matches the current workflow stage."
	}
	return ToolSuggestion{Name: name, Arguments: args, Reason: reason, Risk: risk}, nil
}

func strictArgs(args map[string]any, destination any) error {
	payload, err := json.Marshal(args)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func validText(value string, max int) bool {
	value = strings.TrimSpace(value)
	return value != "" && len([]rune(value)) <= max
}

func boundedHistory(history []ConversationTurn) []ConversationTurn {
	if len(history) > 8 {
		history = history[len(history)-8:]
	}
	result := make([]ConversationTurn, 0, len(history))
	for _, turn := range history {
		result = append(result, ConversationTurn{Role: turn.Role, Content: limitText(turn.Content, 2000)})
	}
	return result
}

func canonicalStage(stage string) string {
	switch strings.TrimSpace(stage) {
	case "investigate", "explore", "collaborate", "review", "approved":
		return strings.TrimSpace(stage)
	default:
		return "orient"
	}
}

func stageToolNames(stage string) []string {
	switch canonicalStage(stage) {
	case "investigate":
		return []string{"get_business_snapshot", "analyze_business"}
	case "explore":
		return []string{"get_business_snapshot", "analyze_business", "create_scenario", "run_scenario", "compare_scenarios", "draft_proposal"}
	case "collaborate":
		return []string{"get_business_snapshot", "revise_proposal"}
	case "review":
		return []string{"get_business_snapshot", "request_human_review"}
	case "approved":
		return []string{"get_business_snapshot", "commit_approved_proposal", "verify_committed_result"}
	default:
		return []string{"get_business_snapshot"}
	}
}

func toolDescription(name string) string {
	switch name {
	case "get_business_snapshot":
		return "Read the authoritative business pulse for the open workspace."
	case "analyze_business":
		return "Analyze verified business evidence for the current human goal."
	case "create_scenario":
		return "Create a bounded non-canonical strategic alternative."
	case "run_scenario":
		return "Run deterministic simulation without changing canonical state."
	case "compare_scenarios":
		return "Compare simulated alternatives and their trade-offs."
	case "draft_proposal":
		return "Create a reviewable proposal from a simulated scenario."
	case "revise_proposal":
		return "Apply human feedback to a working proposal version."
	case "request_human_review":
		return "Submit the current proposal for explicit human review."
	case "verify_committed_result":
		return "Read back the authoritative state after a commit."
	default:
		return "Typed business workspace capability."
	}
}

func toolRisk(name string) string {
	switch name {
	case "create_scenario", "run_scenario", "draft_proposal", "revise_proposal":
		return "non_canonical"
	case "request_human_review":
		return "human_gate"
	default:
		return "read_only"
	}
}

func newConversationID() (string, error) {
	id, err := core.NewID("conversation")
	return string(id), err
}

func limitText(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
