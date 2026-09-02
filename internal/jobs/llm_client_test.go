package jobs

import (
	"encoding/json"
	"slices"
	"testing"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// gpt-5.6 rejects any temperature but 1 with a 400, so the request must not carry it.
func TestChatParams_OmitTemperature(t *testing.T) {
	body := marshalParams(t, territoryReportSchema)

	if _, ok := body["temperature"]; ok {
		t.Error("temperature must not be sent — gpt-5.6 rejects any value but 1")
	}
	if got := body["model"]; got != "gpt-5.6-terra" {
		t.Errorf("model = %v; want gpt-5.6-terra", got)
	}
}

// Strict mode guarantees the response shape only if every property is required and
// extras are forbidden.
func TestResponseSchemas_Strict(t *testing.T) {
	cases := map[string][]string{
		"territory_report":  {"coverage", "needs_attention"},
		"overview_only":     {"overview"},
		"messages_overview": {"overview", "key_themes"},
	}

	for _, sc := range []shared.ResponseFormatJSONSchemaJSONSchemaParam{
		territoryReportSchema, overviewOnlySchema, messagesOverviewSchema,
	} {
		body := marshalParams(t, sc)
		schema := body["response_format"].(map[string]any)["json_schema"].(map[string]any)

		name := schema["name"].(string)
		wantFields, ok := cases[name]
		if !ok {
			t.Fatalf("unexpected schema name %q", name)
		}
		if schema["strict"] != true {
			t.Errorf("%s: strict must be true", name)
		}

		def := schema["schema"].(map[string]any)
		if def["additionalProperties"] != false {
			t.Errorf("%s: additionalProperties must be false", name)
		}

		required := asStrings(def["required"].([]any))
		properties := def["properties"].(map[string]any)
		if len(required) != len(wantFields) || len(properties) != len(wantFields) {
			t.Fatalf("%s: got %d required / %d properties; want %d fields",
				name, len(required), len(properties), len(wantFields))
		}
		for _, f := range wantFields {
			if _, ok := properties[f]; !ok {
				t.Errorf("%s: missing property %q", name, f)
			}
			if !slices.Contains(required, f) {
				t.Errorf("%s: %q must be required", name, f)
			}
		}
	}
}

// A schema field that does not match its struct tag leaves the field silently empty.
func TestResponseSchemas_MatchStructTags(t *testing.T) {
	for _, tc := range []struct {
		schema shared.ResponseFormatJSONSchemaJSONSchemaParam
		target any
	}{
		{territoryReportSchema, &LLMResponse{}},
		{messagesOverviewSchema, &OverviewLLMResponse{}},
	} {
		body := marshalParams(t, tc.schema)
		schema := body["response_format"].(map[string]any)["json_schema"].(map[string]any)
		def := schema["schema"].(map[string]any)

		payload := map[string]any{}
		for f := range def["properties"].(map[string]any) {
			payload[f] = "x"
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, tc.target); err != nil {
			t.Fatalf("%s: %v", schema["name"], err)
		}

		round, err := json.Marshal(tc.target)
		if err != nil {
			t.Fatal(err)
		}
		var back map[string]any
		if err := json.Unmarshal(round, &back); err != nil {
			t.Fatal(err)
		}
		for f := range payload {
			if back[f] != "x" {
				t.Errorf("%s: schema field %q does not map to a struct field", schema["name"], f)
			}
		}
	}
}

func marshalParams(t *testing.T, schema shared.ResponseFormatJSONSchemaJSONSchemaParam) map[string]any {
	t.Helper()
	raw, err := json.Marshal(openai.ChatCompletionNewParams{
		Model: reportModel,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.DeveloperMessage("sys"),
			openai.UserMessage("user"),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{JSONSchema: schema},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func asStrings(vals []any) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i], _ = v.(string)
	}
	return out
}
