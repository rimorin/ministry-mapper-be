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

// generateSummary sends the prompt to gpt-5.4-mini and returns a parsed LLMResponse.
// gpt-5.4-mini is a unified model: it automatically routes to reasoning for statistical
// analysis and uses efficient generation for narrative prose — giving better accuracy
// and more natural writing than pure reasoning models like o4-mini.
// Temperature 0.3 keeps output factual and deterministic.
// JSON mode guarantees a valid JSON payload. 90-second timeout covers reasoning bursts.
func (c *llmClient) generateSummary(systemMsg, userMsg string) (LLMResponse, error) {
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
		return LLMResponse{}, fmt.Errorf("openai request: %w", err)
	}

	if len(completion.Choices) == 0 {
		return LLMResponse{}, fmt.Errorf("openai returned no choices")
	}

	var result LLMResponse
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &result); err != nil {
		log.Printf("AI summary: failed to parse JSON response: %v", err)
		return LLMResponse{}, fmt.Errorf("parse LLM response: %w", err)
	}

	return result, nil
}
