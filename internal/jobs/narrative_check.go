package jobs

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	numberPattern     = regexp.MustCompile(`\d[\d,]*(?:\.\d+)?`)
	quotedNamePattern = regexp.MustCompile(`["“]([^"”]+)["”]`)
)

// checkNarrative rejects a narrative that quotes a number or a map name absent
// from the prompt. Every figure the model may use is in userMsg, so anything
// else is invented; the caller drops the narrative rather than emailing it.
func checkNarrative(resp LLMResponse, userMsg string) error {
	allowed := map[string]bool{}
	for _, n := range numberPattern.FindAllString(userMsg, -1) {
		allowed[strings.ReplaceAll(n, ",", "")] = true
	}

	var problems []string
	for _, text := range []string{resp.Coverage, resp.NeedsAttention} {
		for _, n := range numberPattern.FindAllString(text, -1) {
			if !allowed[strings.ReplaceAll(n, ",", "")] {
				problems = append(problems, "number "+n)
			}
		}
		for _, m := range quotedNamePattern.FindAllStringSubmatch(text, -1) {
			if !strings.Contains(userMsg, m[1]) {
				problems = append(problems, fmt.Sprintf("name %q", m[1]))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("narrative quotes facts not in the prompt: %s", strings.Join(problems, ", "))
	}
	return nil
}
