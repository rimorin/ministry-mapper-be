package llm

import (
	"strings"
	"testing"
)

func TestGroundedNumbers_NormalisesSeparatorsAndLeadingZeros(t *testing.T) {
	source := "Address: Blk 412 #05-12\nDate: 02 Aug 2026\nTotal 1,305"
	if err := GroundedNumbers(source, "Blk 412 unit 5-12 on 2 Aug: 1305 homes."); err != nil {
		t.Errorf("normalised numbers rejected: %v", err)
	}
	if err := GroundedNumbers(source, "Blk 413 has a dog."); err == nil || !strings.Contains(err.Error(), "413") {
		t.Errorf("invented block number accepted: %v", err)
	}
}

func TestGroundedNumbers_ReadsCommaListsInSource(t *testing.T) {
	source := `{"rows":[["Alpha",3,24,90]]}`
	if err := GroundedNumbers(source, "Homes count after 3 tries; links last 24 hours; threshold 90."); err != nil {
		t.Errorf("values from a JSON list rejected: %v", err)
	}
	if err := GroundedNumbers(source, "Threshold 95."); err == nil {
		t.Error("an invented number was accepted")
	}
}
