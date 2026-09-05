package jobs

import (
	"strings"
	"testing"
)

const checkPrompt = `VERIFIED FACTS
  Households reached (done):  1,305
  Total visits:               2220
PREVIOUS PERIOD (July 2026)
  Households reached (done):  98   change: +44
TERRITORY ACTIVITY
M04       |  142 | ...  91%
  flag (≥35% maxed)
  territory M05, map "Block 412" — 45 addresses unworked
`

func TestCheckNarrative_AcceptsGroundedFigures(t *testing.T) {
	resp := LLMResponse{
		Coverage:       "Publishers reached 1305 households across 2,220 visits, up 44 from July 2026. M04 reached 142 and stands at 91%.",
		NeedsAttention: `Map “Block 412” in M05 has 45 addresses unworked, above the 35% review level.`,
	}
	if err := checkNarrative(resp, checkPrompt); err != nil {
		t.Fatalf("grounded narrative rejected: %v", err)
	}
}

func TestCheckNarrative_RejectsInventedNumber(t *testing.T) {
	resp := LLMResponse{Coverage: "Publishers reached 1,400 households.", NeedsAttention: "Nothing to act on."}
	err := checkNarrative(resp, checkPrompt)
	if err == nil || !strings.Contains(err.Error(), "number 1,400") {
		t.Fatalf("expected the invented number to be reported, got %v", err)
	}
}

func TestCheckNarrative_RejectsInventedMapName(t *testing.T) {
	resp := LLMResponse{Coverage: "M04 reached 142.", NeedsAttention: `Map "Block 999" in M05 is stalled.`}
	err := checkNarrative(resp, checkPrompt)
	if err == nil || !strings.Contains(err.Error(), `name "Block 999"`) {
		t.Fatalf("expected the invented map name to be reported, got %v", err)
	}
}
