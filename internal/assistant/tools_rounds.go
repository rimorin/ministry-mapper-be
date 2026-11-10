package assistant

import (
	"encoding/json"
	"math"
	"sort"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// A round of a territory: from the reset that starts it to its completion.
type round struct {
	territoryID, territory string
	start, completed       time.Time // zero when not recorded, or not yet completed
	firstWork              time.Time
}

var territoryRounds = Tool{
	Name: "territory_rounds",
	Description: "The territory coverage record, like the S-13: each round of a territory, from the reset that " +
		"starts it to its completion, read from the round history. view rounds gives one row per round: when " +
		"it started, when work on it began, when it was completed and the days from first work to completion " +
		"(a round still open has no completion). view territories gives one row per current territory: rounds " +
		"completed in the period, the average days a round took, when it was last completed and since when " +
		"its current round is open. The period defaults to the current service year, September to August. " +
		"Rounds from before the history began are not recorded.",
	Params: object(map[string]any{
		"view":      enumOf("rounds: one row per round; territories: one row per territory.", "rounds", "territories"),
		"from":      nullable("string", "First day, YYYY-MM-DD, or null for the start of the current service year."),
		"to":        nullable("string", "Last day, inclusive, YYYY-MM-DD, or null for today."),
		"territory": nullable("string", "Exact territory code, or null for every territory."),
		"limit":     limitParam(),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			View      string  `json:"view"`
			From      *string `json:"from"`
			To        *string `json:"to"`
			Territory *string `json:"territory"`
			Limit     *int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		today := s.Now.In(s.Location)
		from, to := serviceYearStart(today).Format(time.DateOnly), today.Format(time.DateOnly)
		if args.From != nil {
			from = *args.From
		}
		if args.To != nil {
			to = *args.To
		}
		start, end, err := period(s, from, to)
		if err != nil {
			return Result{}, err
		}
		territoryID, err := territoryIDByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}
		rounds, err := roundsOf(s, territoryID, start, end)
		if err != nil {
			return Result{}, err
		}
		if args.View == "territories" {
			return territoryRoundSummary(s, territoryID, rounds, clampLimit(args.Limit))
		}
		r := Result{Columns: []string{"territory", "started", "work_began", "completed", "days"}}
		for _, rd := range rounds {
			row := map[string]any{
				"territory_id": rd.territoryID, "territory": rd.territory,
				"started": dateIn(s, rd.start), "work_began": dateIn(s, rd.firstWork), "completed": dateIn(s, rd.completed),
				"days": nil,
			}
			if days, ok := roundDays(s, rd); ok {
				row["days"] = days
			}
			r.Rows = append(r.Rows, row)
		}
		return truncate(r, clampLimit(args.Limit)), nil
	},
}

// territoryRoundSummary is one row per current territory: what its rounds came
// to in the period.
func territoryRoundSummary(s Scope, territoryID string, rounds []round, limit int) (Result, error) {
	var territories []struct {
		ID            string `db:"id"`
		Code          string `db:"code"`
		RoundStarted  string `db:"round_started_at"`
		LastCompleted string `db:"last_completed_at"`
	}
	err := s.App.DB().NewQuery(`
		SELECT id, code, COALESCE(round_started_at, '') AS round_started_at, COALESCE(last_completed_at, '') AS last_completed_at
		FROM territories WHERE congregation = {:congregation} AND ({:territory} = '' OR id = {:territory})
		ORDER BY code
	`).Bind(dbx.Params{"congregation": s.CongregationID, "territory": territoryID}).All(&territories)
	if err != nil {
		return Result{}, err
	}
	type tally struct{ completed, timed, days int }
	tallies := map[string]*tally{}
	for _, rd := range rounds {
		if rd.completed.IsZero() {
			continue
		}
		t := tallies[rd.territoryID]
		if t == nil {
			t = &tally{}
			tallies[rd.territoryID] = t
		}
		t.completed++
		if days, ok := roundDays(s, rd); ok {
			t.timed++
			t.days += days
		}
	}
	r := Result{Columns: []string{"territory", "completed_rounds", "average_days", "last_completed", "open_since"}}
	for _, t := range territories {
		row := map[string]any{
			"territory_id": t.ID, "territory": t.Code,
			"completed_rounds": 0, "average_days": nil,
			"last_completed": localDate(s, t.LastCompleted), "open_since": "",
		}
		if t.RoundStarted > t.LastCompleted {
			row["open_since"] = localDate(s, t.RoundStarted)
		}
		if tl := tallies[t.ID]; tl != nil {
			row["completed_rounds"] = tl.completed
			if tl.timed > 0 {
				row["average_days"] = int(math.Round(float64(tl.days) / float64(tl.timed)))
			}
		}
		r.Rows = append(r.Rows, row)
	}
	return truncate(r, limit), nil
}

// roundsOf rebuilds each territory's rounds from its history: a reset, or a map
// reset that began a new round, opens one; a completion closes it. A completion
// with no recorded start is a round from before the history began. It returns
// the rounds completed in [start, end), and those still open that began before
// end.
func roundsOf(s Scope, territoryID, start, end string) ([]round, error) {
	var events []struct {
		TerritoryID string `db:"territory"`
		Territory   string `db:"territory_code"`
		Action      string `db:"action"`
		NewRound    bool   `db:"new_round"`
		Created     string `db:"created"`
	}
	err := s.App.DB().NewQuery(`
		SELECT COALESCE(territory, '') AS territory, COALESCE(territory_code, '') AS territory_code, action,
		       COALESCE(json_extract(NULLIF(details, ''), '$.new_round'), 0) AS new_round, created
		FROM territories_log
		WHERE congregation = {:congregation} AND ({:territory} = '' OR territory = {:territory})
		  AND action IN ('reset', 'map_reset', 'completed', 'deleted')
		ORDER BY territory, created
	`).Bind(dbx.Params{"congregation": s.CongregationID, "territory": territoryID}).All(&events)
	if err != nil {
		return nil, err
	}
	var rounds []round
	open := map[string]*round{}
	for _, e := range events {
		at, err := types.ParseDateTime(e.Created)
		if err != nil {
			continue
		}
		switch {
		case e.Action == "reset" || e.Action == "map_reset" && e.NewRound:
			// A reset before completion starts the round over.
			open[e.TerritoryID] = &round{territoryID: e.TerritoryID, territory: e.Territory, start: at.Time()}
		case e.Action == "completed":
			rd := open[e.TerritoryID]
			if rd == nil {
				rd = &round{territoryID: e.TerritoryID, territory: e.Territory}
			}
			rd.completed = at.Time()
			rounds = append(rounds, *rd)
			delete(open, e.TerritoryID)
		case e.Action == "deleted":
			delete(open, e.TerritoryID)
		}
	}
	for _, rd := range open {
		rounds = append(rounds, *rd)
	}
	inPeriod := rounds[:0]
	for _, rd := range rounds {
		if rd.completed.IsZero() && dbTime(rd.start) < end ||
			!rd.completed.IsZero() && dbTime(rd.completed) >= start && dbTime(rd.completed) < end {
			if err := setFirstWork(s, &rd); err != nil {
				return nil, err
			}
			inPeriod = append(inPeriod, rd)
		}
	}
	// By territory; each one's rounds are already in the order they ran.
	sort.SliceStable(inPeriod, func(i, j int) bool { return inPeriod[i].territory < inPeriod[j].territory })
	return inPeriod, nil
}

// setFirstWork finds when work on a round began: its first home marked done or
// nobody home, which is when the S-13 counts it as assigned.
func setFirstWork(s Scope, rd *round) error {
	if rd.start.IsZero() {
		return nil
	}
	end := rd.completed
	if end.IsZero() {
		end = s.Now
	}
	var first string
	err := s.App.DB().NewQuery(`
		SELECT COALESCE(MIN(created), '') FROM addresses_log
		WHERE territory = {:territory} AND old_status = 'not_done' AND new_status IN ('done', 'not_home')
		  AND created >= {:start} AND created <= {:end}
	`).Bind(dbx.Params{"territory": rd.territoryID, "start": dbTime(rd.start), "end": dbTime(end)}).Row(&first)
	if err != nil || first == "" {
		return err
	}
	at, err := types.ParseDateTime(first)
	if err == nil {
		rd.firstWork = at.Time()
	}
	return nil
}

// roundDays is the days from first work to completion, counted in local dates.
func roundDays(s Scope, rd round) (int, bool) {
	if rd.firstWork.IsZero() || rd.completed.IsZero() {
		return 0, false
	}
	day := func(t time.Time) time.Time {
		local := t.In(s.Location)
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	}
	return int(day(rd.completed).Sub(day(rd.firstWork)).Hours() / 24), true
}

func dateIn(s Scope, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(s.Location).Format(time.DateOnly)
}

// serviceYearStart is the September 1 that begins the service year of day.
func serviceYearStart(day time.Time) time.Time {
	year := day.Year()
	if day.Month() < time.September {
		year--
	}
	return time.Date(year, time.September, 1, 0, 0, 0, 0, day.Location())
}
