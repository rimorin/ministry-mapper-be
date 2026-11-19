//go:build testdata

package jobs

import (
	"slices"
	"testing"
)

// Publishers read instructions on the map they are given, so neither digest
// reaches them: not the administrators' and not the one for everyone else.
func TestCongregationRecipients_NeverIncludePublishers(t *testing.T) {
	app := setupMessagesTestApp(t)

	for _, c := range []struct {
		adminOnly bool
		want      []string
	}{
		{true, []string{"admin@alpha.test"}},
		{false, []string{"conductor@alpha.test", "readonly@alpha.test"}},
	} {
		recipients, err := fetchCongregationRecipients(app, "testcongalpha01", c.adminOnly)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, r := range recipients {
			got = append(got, r.Email)
		}
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("adminOnly=%v: recipients = %v, want %v", c.adminOnly, got, c.want)
		}
	}
}
