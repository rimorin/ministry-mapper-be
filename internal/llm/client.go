// Package llm holds the model access shared by the report jobs and the admin
// assistant: the OpenAI client, strict response schemas, the checks that reject
// invented figures, and the writing rules every reader-facing prompt ends with.
package llm

import (
	"context"
	"errors"
	"fmt"
	"os"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// Client wraps the official OpenAI Go SDK for one model.
type Client struct {
	API   openai.Client
	Model openai.ChatModel
}

// New initialises a client using OPENAI_API_KEY from the environment. It returns
// nil if the key is not set, which callers treat as "feature disabled".
func New(model openai.ChatModel) *Client {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil
	}
	return &Client{
		API:   openai.NewClient(option.WithAPIKey(apiKey)),
		Model: model,
	}
}

// JSONSchema builds a strict Structured Outputs schema from a list of string fields.
// Strict mode requires every property in "required" and additionalProperties false.
func JSONSchema(name string, fields ...string) shared.ResponseFormatJSONSchemaJSONSchemaParam {
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

// Complete makes one call and returns the raw JSON content string. The schema is
// enforced by the API, so callers can unmarshal without a shape check.
//
// IMPORTANT: the gpt-5.6 family rejects any temperature but 1, so temperature is
// never sent.
func (c *Client) Complete(ctx context.Context, systemMsg, userMsg string, schema shared.ResponseFormatJSONSchemaJSONSchemaParam) (string, error) {
	completion, err := c.API.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.Model,
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

// Respond makes one Responses call with the client's model, streamed: onText,
// when set, receives the reply's text as it is written. The caller builds the
// rest of the request, such as the admin assistant's functions.
func (c *Client) Respond(ctx context.Context, params responses.ResponseNewParams, onText func(string)) (*responses.Response, error) {
	params.Model = c.Model
	stream := c.API.Responses.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.output_text.delta":
			if onText != nil {
				onText(event.Delta)
			}
		case "response.completed":
			r := event.AsResponseCompleted().Response
			return &r, nil
		case "response.incomplete":
			// Returned as the unstreamed call returns it, for the caller to judge.
			r := event.AsResponseIncomplete().Response
			return &r, nil
		case "response.failed":
			r := event.AsResponseFailed().Response
			return nil, fmt.Errorf("openai response failed: %s", r.Error.Message)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("openai stream ended before the response")
}
