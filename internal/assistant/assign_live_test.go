//go:build testdata

package assistant

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"ministry-mapper/internal/llm"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

// dummyPublishers is a roster the size of a congregation's field service
// group, with the things that make names hard: people who share a first name
// (Sarah, Daniel, Mei), a name written joined (Meiling) and many who share a
// surname. None is a real person.
var dummyPublishers = []string{
	"Tan Mei Ling", "Tan Wei Jie", "Tan Ah Kow", "Lim Mei Hua", "Lim Boon Huat",
	"Ong Siew Ling", "Sarah Lee", "Sarah Wong", "Christopher Ng", "Benjamin Koh",
	"Rachel Goh", "Daniel Chua", "Daniel Teo", "Grace Yeo", "Joshua Tay",
	"Esther Low", "Samuel Ho", "Hannah Sim", "Peter Chong", "Ruth Chia",
	"Michael Foo", "Priya Nair", "Ravi Kumar", "Siti Aminah", "Ahmad Faizal",
	"Nurul Huda", "Kevin Seah", "Jessica Pang", "Andrew Quek", "Michelle Soh",
	"Meiling Chan", "Jonathan Wee", "Deborah Ang", "Timothy Loh", "Naomi Heng",
	"Caleb Toh", "Abigail Poh", "Elijah Mok", "Lydia Kwok", "Marcus Yap",
}

// seedDummyPublishers gives the alpha congregation the roster above, each
// with the publisher role, and twelve maps to hand out. It returns each
// publisher's id by name.
func seedDummyPublishers(t *testing.T, app *tests.TestApp) map[string]string {
	t.Helper()
	save := func(collection string, set func(*core.Record)) {
		col, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			t.Fatal(err)
		}
		rec := core.NewRecord(col)
		set(rec)
		if err := app.SaveNoValidate(rec); err != nil {
			t.Fatal(err)
		}
	}
	ids := map[string]string{}
	for i, name := range dummyPublishers {
		id := fmt.Sprintf("dummypub%07d", i)
		ids[name] = id
		save("users", func(rec *core.Record) {
			rec.Id = id
			rec.Set("email", fmt.Sprintf("dummy%d@example.test", i))
			rec.Set("name", name)
			rec.Set("verified", true)
			rec.Set("disabled", false)
			rec.SetPassword("Test1234!")
		})
		save("roles", func(rec *core.Record) {
			rec.Id = fmt.Sprintf("dummyrol%07d", i)
			rec.Set("congregation", alphaCong)
			rec.Set("user", id)
			rec.Set("role", "publisher")
		})
	}
	for i := 1; i <= 12; i++ {
		save("maps", func(rec *core.Record) {
			rec.Id = fmt.Sprintf("dummymap%07d", i)
			rec.Set("territory", "testterralpha01")
			rec.Set("congregation", alphaCong)
			rec.Set("code", fmt.Sprintf("D%02d", i))
			rec.Set("description", fmt.Sprintf("Blk %d", 300+i))
			rec.Set("type", "single")
			rec.Set("sequence", 100+i)
			rec.Set("progress", 0)
		})
	}
	return ids
}

// TestLiveAssign measures how the assistant hands out maps to a congregation
// of publisher accounts: whether each name lands on the right account, is
// left for the admin to choose, or is rightly given no account, and how long
// and how much each request takes. It asks the real model, so it needs
//
//	OPENAI_LIVE_TEST=1 go test -tags testdata -run TestLiveAssign ./internal/assistant/
//
// OPENAI_LIVE_ASSIGN_RUNS sets how many times each request is asked (default 3).
func TestLiveAssign(t *testing.T) {
	if os.Getenv("OPENAI_LIVE_TEST") == "" {
		t.Skip("set OPENAI_LIVE_TEST=1 to ask the live model")
	}
	chat := llm.New(Model)
	if chat == nil {
		t.Skip("OPENAI_API_KEY not set")
	}
	runs := 3
	if n := os.Getenv("OPENAI_LIVE_ASSIGN_RUNS"); n != "" {
		fmt.Sscan(n, &runs)
	}

	app := newTestApp(t)
	ids := seedDummyPublishers(t, app)
	names := map[string]string{}
	for name, id := range ids {
		names[id] = name
	}
	s := scopeFor(t, app, alphaCong)
	s.Now = time.Now()
	s.Role, s.UserID, s.MemberLinks = roleConductor, "testuseralpha02", true

	// picked are the accounts the cards must choose outright; choose and none
	// count the names left for the admin to pick and those with no account.
	cases := []struct {
		label, question string
		cards           int
		picked          []string
		choose, none    int
	}{
		{"full name", "Give Blk 301 to Tan Mei Ling", 1, []string{"Tan Mei Ling"}, 0, 0},
		{"first name, one match", "Give Blk 302 to Rachel", 1, []string{"Rachel Goh"}, 0, 0},
		{"first name, two matches", "Give Blk 303 to Sarah", 1, nil, 1, 0},
		{"title and surname", "Give Blk 304 to Sis Nair", 1, []string{"Priya Nair"}, 0, 0},
		{"two people, one map", "Give Blk 305 to Christopher and Benjamin", 1, []string{"Christopher Ng", "Benjamin Koh"}, 0, 0},
		{"short forms", "Give Blk 306 to Chris and Ben", 1, []string{"Christopher Ng", "Benjamin Koh"}, 0, 0},
		{"typo", "Give Blk 307 to Jonathon Wee", 1, nil, 1, 0},
		{"name written apart", "Give Blk 308 to Mei Ling Chan", 1, []string{"Meiling Chan"}, 0, 0},
		{"no account", "Give Blk 309 to Bro Fernandez", 1, nil, 0, 1},
		{"one with, one without", "Give Blk 310 to Grace Yeo and Bro Fernandez", 1, []string{"Grace Yeo"}, 0, 1},
		{"shared first name", "Give Blk 311 to Mei", 1, nil, 1, 0},
		{"said in passing", "Daniel Teo and Marcus are working Blk 312 together this morning, set them up", 1,
			[]string{"Daniel Teo", "Marcus Yap"}, 0, 0},
		{"3 pairs, maps not named", "Give these pairs a map each in T01: Tan Wei Jie and Lim Boon Huat; Joshua and Esther; Samuel and Hannah", 3,
			[]string{"Tan Wei Jie", "Lim Boon Huat", "Joshua Tay", "Esther Low", "Samuel Ho", "Hannah Sim"}, 0, 0},
		{"one person, any map", "Give Rachel any map in T01 for tomorrow", 1, []string{"Rachel Goh"}, 0, 0},
		{"6 maps, 12 people", "Give out today's maps: Blk 301 to Tan Wei Jie and Lim Boon Huat; Blk 302 to Ong Siew Ling and Grace; " +
			"Blk 303 to Joshua and Esther; Blk 304 to Samuel and Hannah; Blk 305 to Peter and Ruth; Blk 306 to Michael Foo and Kevin", 6,
			[]string{"Tan Wei Jie", "Lim Boon Huat", "Ong Siew Ling", "Grace Yeo", "Joshua Tay", "Esther Low", "Samuel Ho",
				"Hannah Sim", "Peter Chong", "Ruth Chia", "Michael Foo", "Kevin Seah"}, 0, 0},
		{"10 maps, 20 people", "Give out today's maps: Blk 301 to Tan Wei Jie and Lim Boon Huat; Blk 302 to Ong Siew Ling and Grace; " +
			"Blk 303 to Joshua and Esther; Blk 304 to Samuel and Hannah; Blk 305 to Peter and Ruth; Blk 306 to Michael Foo and Kevin; " +
			"Blk 307 to Priya and Ravi; Blk 308 to Siti and Nurul; Blk 309 to Andrew and Michelle; Blk 310 to Caleb and Abigail", 10,
			[]string{"Tan Wei Jie", "Lim Boon Huat", "Ong Siew Ling", "Grace Yeo", "Joshua Tay", "Esther Low", "Samuel Ho",
				"Hannah Sim", "Peter Chong", "Ruth Chia", "Michael Foo", "Kevin Seah", "Priya Nair", "Ravi Kumar", "Siti Aminah",
				"Nurul Huda", "Andrew Quek", "Michelle Soh", "Caleb Toh", "Abigail Poh"}, 0, 0},
	}

	var prompt, completion, cached int64
	var asked, right, wrongAccounts int
	var allTimes []time.Duration
	for _, c := range cases {
		var times []time.Duration
		var calls, passes int
		var problems []string
		for range runs {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			started := time.Now()
			resp, err := Ask(ctx, chat, s, Request{Question: c.question})
			cancel()
			took := time.Since(started)
			times, allTimes = append(times, took), append(allTimes, took)
			prompt, completion, cached = prompt+resp.PromptTokens, completion+resp.CompletionTokens, cached+resp.CachedTokens
			calls += len(resp.Steps)
			asked++

			var picked []string
			choose, none := 0, 0
			for _, p := range resp.Proposals {
				for _, person := range p.People {
					switch {
					case person.Picked != "":
						picked = append(picked, names[person.Picked])
					case len(person.Suggested) > 0:
						choose++
					default:
						none++
					}
				}
			}
			want := slices.Clone(c.picked)
			sort.Strings(picked)
			sort.Strings(want)
			var problem string
			switch {
			case err != nil:
				problem = err.Error()
			case len(resp.Proposals) != c.cards:
				problem = fmt.Sprintf("%d cards, want %d", len(resp.Proposals), c.cards)
			case !slices.Equal(picked, want):
				problem = fmt.Sprintf("picked %v, want %v", picked, want)
			case choose != c.choose || none != c.none:
				problem = fmt.Sprintf("%d to choose and %d without, want %d and %d", choose, none, c.choose, c.none)
			}
			// An account nobody asked for is the costly mistake: it emails a stranger.
			for _, name := range picked {
				if !slices.Contains(want, name) {
					wrongAccounts++
				}
			}
			if problem == "" {
				passes++
				right++
			} else {
				problems = append(problems, problem+" | "+resp.Answer)
			}
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		t.Logf("%-24s %d/%d right  median %5.1fs  slowest %5.1fs  %.1f function calls",
			c.label, passes, runs, times[len(times)/2].Seconds(), times[len(times)-1].Seconds(), float64(calls)/float64(runs))
		for _, problem := range problems {
			t.Logf("    %s", strings.ReplaceAll(problem, "\n", " "))
		}
	}

	sort.Slice(allTimes, func(i, j int) bool { return allTimes[i] < allTimes[j] })
	const inRate, cachedRate, outRate = 1.0, 0.05, 5.0 // USD per 1M tokens, gpt-6.1-terra
	cost := float64(prompt-cached)/1e6*inRate + float64(cached)/1e6*cachedRate + float64(completion)/1e6*outRate
	t.Logf("%d publishers: %d/%d requests right, %d wrong accounts picked; median %.1fs, slowest %.1fs; "+
		"tokens %d in (%d cached), %d out; est. $%.4f total, $%.5f per request",
		len(dummyPublishers), right, asked, wrongAccounts, allTimes[len(allTimes)/2].Seconds(),
		allTimes[len(allTimes)-1].Seconds(), prompt, cached, completion, cost, cost/float64(asked))
}
