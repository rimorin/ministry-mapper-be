package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ministry-mapper/internal/llm"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// OverviewLLMResponse holds the parsed JSON output from the AI model for the notes,
// messages and instructions emails. Todo is filled only by the messages shape.
type OverviewLLMResponse struct {
	Overview string   `json:"overview"`
	Todo     []string `json:"todo"`
}

// OverviewSummary is the template-ready AI overview for those emails.
// Available is set true only after a successful, grounded LLM call.
type OverviewSummary struct {
	Available bool
	Overview  string
	Todo      []string
}

// llmClient adds the report jobs' typed calls to the shared client.
type llmClient struct {
	*llm.Client
}

// newLLMClient returns nil if OPENAI_API_KEY is not set, which callers treat as
// "feature disabled".
func newLLMClient() *llmClient {
	c := llm.New(reportModel)
	if c == nil {
		return nil
	}
	return &llmClient{c}
}

// reportModel is the model used for every congregation narrative. gpt-5.6-terra is
// picked for instruction adherence, not reasoning: BuildPrompt hands it every figure
// pre-counted, so following the writing rules is all that is left.
//
// reasoning_effort is left at its default; observed usage is 0 reasoning tokens on
// this prompt, so there is nothing to tune down.
const reportModel = openai.ChatModelGPT5_6Terra

// One schema per response shape, so a caller can never receive a field its prompt did
// not ask for — the notes and instructions prompts ask for "exactly one field". The
// messages shape adds a to-do list; strict mode has no maxItems, so the prompt caps it.
var (
	territoryReportSchema  = llm.JSONSchema("territory_report", "coverage", "needs_attention")
	overviewOnlySchema     = llm.JSONSchema("overview_only", "overview")
	messagesOverviewSchema = shared.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:   "messages_overview",
		Strict: openai.Bool(true),
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"overview": map[string]any{"type": "string"},
				"todo":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required":             []string{"overview", "todo"},
			"additionalProperties": false,
		},
	}
)

// callLLM makes the API call with the report timeout and returns the raw JSON content.
func (c *llmClient) callLLM(systemMsg, userMsg string, schema shared.ResponseFormatJSONSchemaJSONSchemaParam) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return c.Complete(ctx, systemMsg, userMsg, schema)
}

// generateSummary sends the prompt to the LLM and returns a parsed LLMResponse
// for the monthly territory report.
func (c *llmClient) generateSummary(systemMsg, userMsg string) (LLMResponse, error) {
	raw, err := c.callLLM(systemMsg, userMsg, territoryReportSchema)
	if err != nil {
		return LLMResponse{}, err
	}

	var result LLMResponse
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		log.Printf("AI summary: failed to parse JSON response: %v", err)
		return LLMResponse{}, fmt.Errorf("parse LLM response: %w", err)
	}

	return result, nil
}

// generateOverview sends the prompt to the LLM and returns a parsed OverviewLLMResponse
// for notes and messages emails.
func (c *llmClient) generateOverview(systemMsg, userMsg string, schema shared.ResponseFormatJSONSchemaJSONSchemaParam) (OverviewLLMResponse, error) {
	raw, err := c.callLLM(systemMsg, userMsg, schema)
	if err != nil {
		return OverviewLLMResponse{}, err
	}

	var result OverviewLLMResponse
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		log.Printf("AI overview: failed to parse JSON response: %v", err)
		return OverviewLLMResponse{}, fmt.Errorf("parse LLM response: %w", err)
	}

	return result, nil
}
