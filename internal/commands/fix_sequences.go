// Package commands holds console commands registered on app.RootCmd.
package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// Repairs violations of the documented invariant "address sequence is per-map
// and shared across floors for the same code".
//
// Two codes sharing a sequence breaks two things: the map table renders one
// row of column headers from the highest floor while every floor sorts its own
// units, so a tied pair can land under the wrong header on some floors; and
// the Excel report keys its grid on sequence (generate_report.go), so the
// second of a tied pair silently overwrites the first and vanishes.

type codeSequence struct {
	Code     string `db:"code"`
	Sequence int    `db:"sequence"`
	N        int    `db:"n"`
}

type mapPlan struct {
	Id         string
	Name       string
	Type       string
	Order      []string       // codes in repaired column order
	NewSeq     map[string]int // code -> repaired sequence
	OldSeq     map[string]int // code -> sequence today
	Collisions [][]string     // codes that currently share a sequence
	Drift      []string       // codes holding more than one sequence
	Descending bool           // map's columns run high-to-low
	Unclear    bool           // no dominant direction, tie order is a guess
}

func (p *mapPlan) blocked() bool {
	return len(p.Drift) > 0
}

// NewFixSequences builds the `fix-sequences` console command.
func NewFixSequences(app core.App) *cobra.Command {
	var apply bool
	var includeUnclear bool
	var only string

	command := &cobra.Command{
		Use:   "fix-sequences",
		Short: "Find and repair duplicate address sequences",
		Long: "Scans every map for codes sharing a sequence and renumbers the affected\n" +
			"maps 0..N-1. Runs as a dry run unless --apply is passed.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runFixSequences(app, apply, includeUnclear, only)
		},
	}

	command.Flags().BoolVar(&apply, "apply", false, "write the changes (default is a dry run)")
	command.Flags().BoolVar(&includeUnclear, "include-unclear", false, "also repair maps with no dominant column direction")
	command.Flags().StringVar(&only, "map", "", "limit the scan to a single map id")

	return command
}

func printUnclear(unclear []*mapPlan) {
	if len(unclear) == 0 {
		return
	}
	fmt.Printf("UNCLEAR - no dominant column direction (%d maps)\n", len(unclear))
	for _, plan := range unclear {
		fmt.Printf("  %-15s %-6s %s\n", plan.Id, plan.Type, truncate(plan.Name, 44))
		fmt.Printf("      %s\n", truncate(strings.Join(plan.Order, " "), 70))
	}
	fmt.Println("  Pass --include-unclear to repair these with natural code order.")
	fmt.Println()
}

func runFixSequences(app core.App, apply, includeUnclear bool, only string) error {
	ids, err := collidedMapIds(app, only)
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		fmt.Println("No sequence collisions found.")
		return nil
	}

	fmt.Printf("Found %d map(s) with colliding sequences.\n\n", len(ids))

	var fixable, review, unclear []*mapPlan
	for _, id := range ids {
		plan, err := buildPlan(app, id)
		if err != nil {
			return fmt.Errorf("map %s: %w", id, err)
		}
		if plan.blocked() {
			review = append(review, plan)
			continue
		}
		if plan.Unclear && !includeUnclear {
			unclear = append(unclear, plan)
			continue
		}
		fixable = append(fixable, plan)
	}

	printReview(review)
	printUnclear(unclear)

	fmt.Printf("REPAIRABLE (%d maps)\n", len(fixable))
	renumbered := 0
	for _, plan := range fixable {
		changed := plan.changedCodes()
		renumbered += len(changed)
		direction := "ascending"
		if plan.Descending {
			direction = "descending"
		}
		fmt.Printf("  %-15s %-6s %-44s [%s]\n", plan.Id, plan.Type, truncate(plan.Name, 44), direction)
		for _, group := range plan.Collisions {
			fmt.Printf("      seq %d shared by %s -> %s\n",
				plan.OldSeq[group[0]], strings.Join(group, ", "), describeNew(plan, group))
		}
	}

	fmt.Printf("\n%d map(s) repairable, %d address column(s) renumbered, %d skipped, %d unclear.\n",
		len(fixable), renumbered, len(review), len(unclear))

	if !apply {
		fmt.Println("\nDry run - nothing written. Re-run with --apply to write.")
		return nil
	}

	for _, plan := range fixable {
		if err := writePlan(app, plan); err != nil {
			return fmt.Errorf("map %s: %w", plan.Id, err)
		}
	}
	fmt.Printf("\nApplied to %d map(s).\n", len(fixable))

	return nil
}

func printReview(review []*mapPlan) {
	if len(review) == 0 {
		return
	}
	fmt.Printf("SKIPPED - duplicate unit records (%d maps)\n", len(review))
	for _, plan := range review {
		fmt.Printf("  %-15s %-6s %s\n", plan.Id, plan.Type, truncate(plan.Name, 44))
		fmt.Printf("      code(s) %s hold more than one sequence; renumbering cannot\n",
			truncate(strings.Join(plan.Drift, ", "), 50))
		fmt.Printf("      choose which duplicate record to keep\n")
	}
	fmt.Println()
}

func collidedMapIds(app core.App, only string) ([]string, error) {
	// Two ways to break "one sequence per code, per map": two codes on one
	// sequence, or one code on two sequences. The second does not always come
	// with the first, so both have to be swept for.
	sql := `
		SELECT DISTINCT map FROM (
			SELECT map FROM addresses
			%[1]s
			GROUP BY map, sequence
			HAVING COUNT(DISTINCT code) > 1
			UNION
			SELECT map FROM addresses
			%[1]s
			GROUP BY map, code
			HAVING COUNT(DISTINCT sequence) > 1
		)`

	params := dbx.Params{}
	filter := ""
	if only != "" {
		filter = "WHERE map = {:map}"
		params["map"] = only
	}

	var rows []struct {
		Map string `db:"map"`
	}
	if err := app.DB().NewQuery(fmt.Sprintf(sql, filter)).Bind(params).All(&rows); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Map)
	}
	sort.Strings(ids)

	return ids, nil
}

func buildPlan(app core.App, mapId string) (*mapPlan, error) {
	var meta struct {
		Description string `db:"description"`
		Type        string `db:"type"`
	}
	if err := app.DB().NewQuery(`SELECT description, type FROM maps WHERE id = {:id}`).
		Bind(dbx.Params{"id": mapId}).One(&meta); err != nil {
		return nil, err
	}

	var rows []codeSequence
	if err := app.DB().NewQuery(`
		SELECT code, sequence, COUNT(*) AS n
		FROM addresses
		WHERE map = {:map}
		GROUP BY code, sequence
	`).Bind(dbx.Params{"map": mapId}).All(&rows); err != nil {
		return nil, err
	}

	plan := planColumns(rows)
	plan.Id = mapId
	plan.Name = meta.Description
	plan.Type = meta.Type

	return plan, nil
}

// planColumns is the whole decision, kept free of the database so it can be
// exercised directly against the layouts found in production.
func planColumns(rows []codeSequence) *mapPlan {
	plan := &mapPlan{
		NewSeq: map[string]int{},
		OldSeq: map[string]int{},
	}

	// A code should hold exactly one sequence across all its floors. When it
	// holds several the map has duplicate unit records, which renumbering
	// cannot resolve without deciding which record to drop.
	byCode := map[string][]codeSequence{}
	for _, row := range rows {
		byCode[row.Code] = append(byCode[row.Code], row)
	}

	for code, entries := range byCode {
		if len(entries) > 1 {
			plan.Drift = append(plan.Drift, code)
		}
		best := entries[0]
		for _, entry := range entries[1:] {
			if entry.N > best.N || (entry.N == best.N && entry.Sequence < best.Sequence) {
				best = entry
			}
		}
		plan.OldSeq[code] = best.Sequence
		plan.Order = append(plan.Order, code)
	}
	sort.Strings(plan.Drift)

	// Only codes sharing a sequence can move relative to each other; everything
	// else keeps the order it has today. So the one decision to make is which
	// way to read a tied group, and a corridor numbered high-to-low is just as
	// normal as one numbered low-to-high — take the direction from the columns
	// whose order is already known.
	plan.sortColumns()
	plan.Descending, plan.Unclear = detectDirection(plan.Order, plan.OldSeq)
	plan.sortColumns()

	for i, code := range plan.Order {
		plan.NewSeq[code] = i
	}

	shared := map[int][]string{}
	for _, code := range plan.Order {
		shared[plan.OldSeq[code]] = append(shared[plan.OldSeq[code]], code)
	}
	for _, code := range plan.Order {
		if group := shared[plan.OldSeq[code]]; len(group) > 1 && group[0] == code {
			plan.Collisions = append(plan.Collisions, group)
		}
	}

	return plan
}

func (p *mapPlan) sortColumns() {
	sort.Slice(p.Order, func(i, j int) bool {
		a, b := p.Order[i], p.Order[j]
		if p.OldSeq[a] != p.OldSeq[b] {
			return p.OldSeq[a] < p.OldSeq[b]
		}
		if p.Descending {
			return naturalLess(b, a)
		}
		return naturalLess(a, b)
	})
}

// detectDirection reads the map's existing layout. Pairs sharing a sequence are
// skipped — their order is exactly what is undefined, so they cannot vote.
func detectDirection(order []string, oldSeq map[string]int) (descending, unclear bool) {
	var up, down int
	for i := 1; i < len(order); i++ {
		prev, cur := order[i-1], order[i]
		if oldSeq[prev] == oldSeq[cur] {
			continue
		}
		if naturalLess(prev, cur) {
			up++
		} else {
			down++
		}
	}

	total := up + down
	if total == 0 {
		return false, true
	}

	descending = down > up
	winner := up
	if descending {
		winner = down
	}

	return descending, float64(winner)/float64(total) < 0.75
}

func (p *mapPlan) changedCodes() []string {
	var changed []string
	for _, code := range p.Order {
		if p.NewSeq[code] != p.OldSeq[code] {
			changed = append(changed, code)
		}
	}
	return changed
}

// Raw SQL, not the record API: renumbering is not a user edit, so it must not
// touch `updated`/`updated_by` or broadcast realtime events to publishers.
func writePlan(app core.App, plan *mapPlan) error {
	return app.RunInTransaction(func(txApp core.App) error {
		for _, code := range plan.changedCodes() {
			_, err := txApp.DB().NewQuery(`
				UPDATE addresses SET sequence = {:seq}
				WHERE map = {:map} AND code = {:code}
			`).Bind(dbx.Params{
				"seq":  plan.NewSeq[code],
				"map":  plan.Id,
				"code": code,
			}).Execute()
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func describeNew(plan *mapPlan, group []string) string {
	parts := make([]string, 0, len(group))
	for _, code := range group {
		parts = append(parts, fmt.Sprintf("%s=%d", code, plan.NewSeq[code]))
	}
	return strings.Join(parts, ", ")
}

// naturalLess orders codes the way a person reads them: digit runs compare
// numerically so "9" precedes "10", and "10" precedes "10A".
func naturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if isDigit(a[i]) && isDigit(b[j]) {
			si, sj := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na := strings.TrimLeft(a[si:i], "0")
			nb := strings.TrimLeft(b[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if a[i] != b[j] {
			return a[i] < b[j]
		}
		i++
		j++
	}
	return len(a)-i < len(b)-j
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
