package jobs

import "ministry-mapper/internal/llm"

// checkNarrative rejects a territory report narrative that quotes a number or a
// map name absent from the prompt.
func checkNarrative(resp LLMResponse, userMsg string) error {
	if err := llm.GroundedNumbers(userMsg, resp.Coverage, resp.NeedsAttention); err != nil {
		return err
	}
	return llm.GroundedQuotedNames(userMsg, resp.Coverage, resp.NeedsAttention)
}
