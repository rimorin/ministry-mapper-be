// Package assistant answers administrators' questions about their congregation's
// territories in natural language. The model only chooses query functions and their
// filters; every function runs fixed, parameterised SQL scoped to the congregation
// taken from the authenticated request, never from the model's arguments.
package assistant

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// Scope is everything a query function may read beyond its own arguments.
type Scope struct {
	App            core.App
	CongregationID string
	Location       *time.Location
	Now            time.Time
	// Locale picks the language of names entered in several languages.
	Locale string
	// UserID and Role are the asker's; actions are for administrators only.
	UserID string
	Role   string
	// Origin is the admin's location when they chose to share it. Like the
	// congregation, it reaches functions only through the scope, never the model.
	Origin *Location
	// View is what the admin has open in the app, when they let it be used.
	View *View
	// seen is everything the model has been shown so far in this question, so
	// calculate can refuse numbers that are not in the data.
	seen string
	// knownTokens maps each person token to its name, loaded once per question.
	knownTokens map[string]string
}

// Location is a point on the map, as latitude and longitude in degrees.
type Location = point

// Result is one function's answer: rows keyed by column, shown to the admin as a
// table under the written answer. Keys ending in "_id" carry record ids for links;
// they are kept from the model, which works with codes and names.
type Result struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	// Total counts the rows before the limit, so the model can say "12 of 40".
	Total int `json:"total"`
	// Note carries a definition or caveat the model must respect, such as when a
	// record type started being tracked.
	Note string `json:"note,omitempty"`
	// Proposal is a change an action function prepared; it goes to the panel's
	// card, not into the table.
	Proposal *Proposal `json:"-"`
}

// Tool is one query function offered to the model.
type Tool struct {
	Name        string
	Description string
	Params      map[string]any
	// PersonColumns name the columns holding people's names. The model sees them
	// as tokens such as Person QXWBKDMR and the answer is translated back before it reaches the admin.
	PersonColumns []string
	Run           func(s Scope, args json.RawMessage) (Result, error)
}

// mapArg and mapTerritoryArg are the parameters that name one map.
var (
	mapArg          = map[string]any{"type": "string", "description": "The map's exact name. Use find_places first if unsure."}
	mapTerritoryArg = nullable("string", "The map's territory code, when several maps share the name.")
)

// argError is a mistake in the model's arguments. It goes back to the model as the
// function's output so it can correct itself, instead of failing the request.
type argError struct{ msg string }

func (e argError) Error() string { return e.msg }

func badArgs(format string, a ...any) error { return argError{fmt.Sprintf(format, a...)} }

func decodeArgs(raw json.RawMessage, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return badArgs("arguments are not valid JSON for this function: %v", err)
	}
	return nil
}

// Strict function schemas need every property listed in "required"; optional
// filters are nullable instead of omitted.
func object(props map[string]any) map[string]any {
	required := make([]string, 0, len(props))
	for k := range props {
		required = append(required, k)
	}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

func nullable(kind, description string) map[string]any {
	return map[string]any{"type": []string{kind, "null"}, "description": description}
}

func nullableEnum(description string, values ...string) map[string]any {
	enum := make([]any, 0, len(values)+1)
	for _, v := range values {
		enum = append(enum, v)
	}
	enum = append(enum, nil)
	return map[string]any{"type": []string{"string", "null"}, "enum": enum, "description": description}
}

func enumOf(description string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": description}
}

const (
	defaultLimit = 25
	maxLimit     = 100
)

func limitParam() map[string]any {
	return nullable("integer", fmt.Sprintf("Most rows to return, default %d, at most %d.", defaultLimit, maxLimit))
}

func clampLimit(limit *int) int {
	if limit == nil || *limit <= 0 {
		return defaultLimit
	}
	return min(*limit, maxLimit)
}

// truncate applies the limit and records the total before it.
func truncate(r Result, limit int) Result {
	r.Total = len(r.Rows)
	if len(r.Rows) > limit {
		r.Rows = r.Rows[:limit]
	}
	return r
}

// localDate formats a stored PocketBase datetime as a date in the congregation's
// time zone, or "" when empty.
func localDate(s Scope, stored string) string {
	return localTime(s, stored, time.DateOnly)
}

// localTime formats a stored datetime in the congregation's time zone, or ""
// when empty.
func localTime(s Scope, stored, layout string) string {
	dt, err := types.ParseDateTime(stored)
	if err != nil || dt.IsZero() {
		return ""
	}
	return dt.Time().In(s.Location).Format(layout)
}

const dateTimeLayout = "2006-01-02 15:04"

// daysAgo returns the stored datetime for the start of the local day n days ago.
func daysAgo(s Scope, n int) string {
	local := s.Now.In(s.Location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, s.Location).AddDate(0, 0, -n)
	return dbTime(start)
}

// dbTime formats t the way PocketBase stores datetimes, so string comparison in
// SQL orders correctly.
func dbTime(t time.Time) string {
	dt, _ := types.ParseDateTime(t)
	return dt.String()
}

// parseLocalDate reads a YYYY-MM-DD date in the congregation's time zone.
func parseLocalDate(s Scope, field, value string) (time.Time, error) {
	t, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(value), s.Location)
	if err != nil {
		return time.Time{}, badArgs("%s must be a date like 2026-11-06", field)
	}
	return t, nil
}

func validDays(field string, v *int) error {
	if v != nil && (*v < 1 || *v > 3650) {
		return badArgs("%s must be between 1 and 3650", field)
	}
	return nil
}

func eqFold(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }

// localizedColumns hold names an admin may have entered in several languages,
// stored as a JSON object keyed by locale, like the frontend's resolveLocalized.
var localizedColumns = []string{"map", "description", "territory_description"}

// resolveLocalized returns the name in the locale, else English, else any language.
func resolveLocalized(value, locale string) string {
	byLocale, ok := localizedNames(value)
	if !ok {
		return value
	}
	if v := byLocale[localeKey(byLocale, locale)]; v != "" {
		return v
	}
	return value
}

// localizedNames parses a name kept in several languages, such as
// {"en":"Rich Map","zh":"富地图"}.
func localizedNames(stored string) (map[string]string, bool) {
	if !strings.HasPrefix(strings.TrimSpace(stored), "{") {
		return nil, false
	}
	var byLocale map[string]string
	if json.Unmarshal([]byte(stored), &byLocale) != nil || len(byLocale) == 0 {
		return nil, false
	}
	return byLocale, true
}

// localeKey picks which language to read: the locale, then English, then the
// first language that has a name.
func localeKey(byLocale map[string]string, locale string) string {
	for _, k := range []string{locale, "en"} {
		if byLocale[k] != "" {
			return k
		}
	}
	keys := make([]string, 0, len(byLocale))
	for k := range byLocale {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if byLocale[k] != "" {
			return k
		}
	}
	return keys[0]
}

func localizeRows(r *Result, locale string) {
	for _, row := range r.Rows {
		for _, c := range localizedColumns {
			if v, ok := row[c].(string); ok {
				row[c] = resolveLocalized(v, locale)
			}
		}
	}
}
