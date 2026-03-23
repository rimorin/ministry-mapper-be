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

// callLLM makes the API call to gpt-5.4-mini and returns the raw JSON content string.
// gpt-5.4-mini is a unified model: it automatically routes to reasoning for statistical
// analysis and uses efficient generation for narrative prose — giving better accuracy
// and more natural writing than pure reasoning models like o4-mini.
// Temperature 0.3 keeps output factual and deterministic.
// JSON mode guarantees a valid JSON payload. 90-second timeout covers reasoning bursts.
func (c *llmClient) callLLM(systemMsg, userMsg string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModelGPT5_4Mini,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.DeveloperMessage(systemMsg),
			openai.UserMessage(userMsg),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openai.ResponseFormatJSONObjectParam{Type: "json_object"},
		},
		Temperature: openai.Float(0.3),
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
	raw, err := c.callLLM(systemMsg, userMsg)
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
func (c *llmClient) generateOverview(systemMsg, userMsg string) (OverviewLLMResponse, error) {
	raw, err := c.callLLM(systemMsg, userMsg)
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
