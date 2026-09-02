package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// OverviewLLMResponse holds the parsed JSON output from the AI model for notes and messages emails.
type OverviewLLMResponse struct {
	Overview  string `json:"overview"`
	KeyThemes string `json:"key_themes"`
}

// OverviewSummary is the template-ready AI overview for notes and messages emails.
// Available is set true only after a successful LLM call populates the narrative fields.
type OverviewSummary struct {
	Available bool
	Overview  string
	KeyThemes string
}

// llmClient wraps the official OpenAI Go SDK for generating congregation summaries.
type llmClient struct {
	client openai.Client
}

// newLLMClient initialises an OpenAI client using OPENAI_API_KEY from the environment.
// Returns nil if the key is not set, which callers treat as "feature disabled".
func newLLMClient() *llmClient {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil
	}
	return &llmClient{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
	}
}

// reportModel is the model used for every congregation narrative. gpt-5.6-terra is
// picked for instruction adherence, not reasoning: BuildPrompt hands it every figure
// pre-counted, so following the writing rules is all that is left.
//
// IMPORTANT: the 5.6 family rejects any temperature but 1, so temperature is not sent
// at all. reasoning_effort is left at its default; observed usage is 0 reasoning
// tokens on this prompt, so there is nothing to tune down.
const reportModel = openai.ChatModelGPT5_6Terra

// jsonSchema builds a strict Structured Outputs schema from a field list. Strict mode
// requires every property in "required" and additionalProperties false.
func jsonSchema(name string, fields ...string) shared.ResponseFormatJSONSchemaJSONSchemaParam {
	properties := make(map[string]any, len(fields))
	for _, f := range fields {
		properties[f] = map[string]any{"type": "string"}
	}
	return shared.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:   name,
		Strict: openai.Bool(true),
		Schema: map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             fields,
			"additionalProperties": false,
		},
	}
}

// One schema per response shape, so a caller can never receive a field its prompt did
// not ask for — the notes and instructions prompts ask for "exactly one field".
var (
	territoryReportSchema  = jsonSchema("territory_report", "coverage", "needs_attention")
	overviewOnlySchema     = jsonSchema("overview_only", "overview")
	messagesOverviewSchema = jsonSchema("messages_overview", "overview", "key_themes")
)

// callLLM makes the API call and returns the raw JSON content string. The schema is
// enforced by the API, so callers can unmarshal without a shape check.
func (c *llmClient) callLLM(systemMsg, userMsg string, schema shared.ResponseFormatJSONSchemaJSONSchemaParam) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: reportModel,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.DeveloperMessage(systemMsg),
			openai.UserMessage(userMsg),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{JSONSchema: schema},
		},
	})
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}

	return completion.Choices[0].Message.Content, nil
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
