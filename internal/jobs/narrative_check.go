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

// normaliseNumber makes "1,305", "1305" and "05" comparable to "1305" and "5".
func normaliseNumber(n string) string {
	n = strings.ReplaceAll(n, ",", "")
	if trimmed := strings.TrimLeft(n, "0"); trimmed != "" && !strings.HasPrefix(trimmed, ".") {
		return trimmed
	}
	return n
}

// groundedNumbers rejects any text that quotes a number absent from source. Every
// figure the model may use is in the prompt, so any other number is invented.
func groundedNumbers(source string, texts ...string) error {
	allowed := map[string]bool{}
	for _, n := range numberPattern.FindAllString(source, -1) {
		allowed[normaliseNumber(n)] = true
	}
	var invented []string
	for _, text := range texts {
		for _, n := range numberPattern.FindAllString(text, -1) {
			if !allowed[normaliseNumber(n)] {
				invented = append(invented, n)
			}
		}
	}
	if len(invented) > 0 {
		return fmt.Errorf("quotes numbers not in the prompt: %s", strings.Join(invented, ", "))
	}
	return nil
}

// groundedQuotedNames rejects any quoted name, such as a map description, that
// does not appear verbatim in source.
func groundedQuotedNames(source string, texts ...string) error {
	var invented []string
	for _, text := range texts {
		for _, m := range quotedNamePattern.FindAllStringSubmatch(text, -1) {
			if !strings.Contains(source, m[1]) {
				invented = append(invented, fmt.Sprintf("%q", m[1]))
			}
		}
	}
	if len(invented) > 0 {
		return fmt.Errorf("quotes names not in the prompt: %s", strings.Join(invented, ", "))
	}
	return nil
}

// checkNarrative applies both checks to the territory report narrative.
func checkNarrative(resp LLMResponse, userMsg string) error {
	if err := groundedNumbers(userMsg, resp.Coverage, resp.NeedsAttention); err != nil {
		return err
	}
	return groundedQuotedNames(userMsg, resp.Coverage, resp.NeedsAttention)
}
