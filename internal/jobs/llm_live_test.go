package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// These tests call the real API and cost money, so they need an explicit opt-in:
//
//	export $(grep -v '^#' .env | grep OPENAI_API_KEY | xargs)
//	OPENAI_LIVE_TEST=1 go test ./internal/jobs -run TestLive -v -count=1
func liveClient(t *testing.T) *llmClient {
	t.Helper()
	if os.Getenv("OPENAI_LIVE_TEST") == "" {
		t.Skip("set OPENAI_LIVE_TEST=1 to run live API tests")
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	c := newLLMClient()
	if c == nil {
		t.Fatal("newLLMClient returned nil despite key being set")
	}
	return c
}

// Synthetic figures only — no real congregation data is sent.
func liveSampleData() SummaryData {
	return SummaryData{
		CongregationName: "Sample Congregation",
		Period:           "August 2026",
		Territories: []TerritoryProgress{
			{Code: "M09", Total: 900, Invalid: 8, Progress: 62, NotDone: 300},
			{Code: "W06", Total: 700, Invalid: 3, Progress: 48, NotDone: 320},
			{Code: "M01", Total: 800, Invalid: 15, Progress: 41, NotDone: 400},
			{Code: "W03", Total: 650, Invalid: 5, Progress: 55, NotDone: 240},
		},
		MonthlyByTerritory: []TerritoryMonthlyActivity{
			{TerritoryCode: "M09", Done: 270, NotHome: 90, DNC: 4},
			{TerritoryCode: "W06", Done: 211, NotHome: 123, DNC: 2},
			{TerritoryCode: "M01", Done: 143, NotHome: 285, DNC: 6},
			{TerritoryCode: "W03", Done: 138, NotHome: 244, DNC: 3},
		},
		Activity: []ActivityItem{
			{Status: "done", Count: 762},
			{Status: "not_home", Count: 742},
			{Status: "not_done", Count: 541},
			{Status: "do_not_call", Count: 15},
			{Status: "invalid", Count: 12},
		},
		ActiveTerritories: 4,
		HouseholdsReached: 762,
		Visits:            1519,
		NotHomeFatigue: []NotHomeFatigue{
			{TerritoryCode: "W03", MaxedOut: 88, Retrying: 120, Stale: 40, MaxedOutPct: 42.3},
			{TerritoryCode: "M09", MaxedOut: 10, Retrying: 150, Stale: 3, MaxedOutPct: 6.3},
		},
		StalledMaps:   []MapHealthItem{{TerritoryCode: "W11", MapDescription: "Blk 118 Rivervale", NotDone: 96}},
		CompletedMaps: []MapHealthItem{{TerritoryCode: "M09", MapDescription: "Blk 412 Serangoon"}},
		HighDNCMaps:   []MapHealthItem{{TerritoryCode: "M01", MapDescription: "Blk 77 Yishun", DNC: 18}},
	}
}

func TestLiveReport(t *testing.T) {
	c := liveClient(t)
	sys, user := BuildPrompt(liveSampleData())

	start := time.Now()
	resp, err := c.generateSummary(sys, user)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("generateSummary: %v", err)
	}

	fmt.Printf("\n=== TERRITORY REPORT (%.1fs) ===\n\nCOVERAGE:\n%s\n\nNEEDS ATTENTION:\n%s\n\n",
		elapsed.Seconds(), resp.Coverage, resp.NeedsAttention)

	if resp.Coverage == "" || resp.NeedsAttention == "" {
		t.Error("both paragraphs must be populated")
	}
}

func TestLiveOverviewOnly(t *testing.T) {
	c := liveClient(t)
	sys, user := BuildNotesPrompt([]notesData{
		{Address: "Blk 412 #05-12", Publisher: "Bro A", Date: "2026-08-04", Message: "Large dog in the yard, call from the gate."},
		{Address: "Blk 412 #07-03", Publisher: "Sis B", Date: "2026-08-11", Message: "Intercom broken, side gate is open on weekends."},
		{Address: "Blk 77 #02-18", Publisher: "Bro C", Date: "2026-08-19", Message: "Unit under renovation, vacant."},
	}, "Sample")

	resp, err := c.generateOverview(sys, user, overviewOnlySchema)
	if err != nil {
		t.Fatalf("generateOverview (overview_only): %v", err)
	}
	fmt.Printf("=== NOTES OVERVIEW ===\n%s\n\nkey_themes (must be empty): %q\n\n", resp.Overview, resp.KeyThemes)

	if resp.Overview == "" {
		t.Error("overview must be populated")
	}
	if resp.KeyThemes != "" {
		t.Error("strict schema must prevent key_themes on the overview_only shape")
	}
}

func TestLiveMessagesOverview(t *testing.T) {
	c := liveClient(t)
	sys, user := BuildMessagesPrompt([]messagesData{
		{MapName: "Blk 118 Rivervale", Publisher: "Bro A", Date: "2026-08-06", Message: "Unit 44 does not exist, please remove it."},
		{MapName: "Blk 412 Serangoon", Publisher: "Sis B", Date: "2026-08-14", Message: "Lift lobby needs a resident card after 8pm."},
	}, "Sample")

	resp, err := c.generateOverview(sys, user, messagesOverviewSchema)
	if err != nil {
		t.Fatalf("generateOverview (messages_overview): %v", err)
	}
	fmt.Printf("=== MESSAGES OVERVIEW ===\n%s\n\nKEY THEMES:\n%s\n\n", resp.Overview, resp.KeyThemes)

	if resp.Overview == "" || resp.KeyThemes == "" {
		t.Error("both fields must be populated")
	}
}

// Reports real token usage so cost can be checked against the estimate.
func TestLiveUsage(t *testing.T) {
	c := liveClient(t)
	sys, user := BuildPrompt(liveSampleData())

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: reportModel,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.DeveloperMessage(sys),
			openai.UserMessage(user),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{JSONSchema: territoryReportSchema},
		},
	})
	if err != nil {
		t.Fatalf("usage probe: %v", err)
	}

	u := completion.Usage
	const inRate, outRate = 2.0, 12.0 // USD per 1M tokens, gpt-5.6-terra
	cost := float64(u.PromptTokens)/1e6*inRate + float64(u.CompletionTokens)/1e6*outRate

	fmt.Printf("=== USAGE ===\nmodel returned:   %s\nprompt tokens:    %d\ncompletion:       %d (reasoning %d)\ntotal:            %d\nest. cost/report: $%.5f\n\n",
		completion.Model, u.PromptTokens, u.CompletionTokens,
		u.CompletionTokensDetails.ReasoningTokens, u.TotalTokens, cost)

	raw, _ := json.Marshal(completion.Usage)
	t.Logf("raw usage: %s", raw)
}
