package assistant

import (
	"fmt"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
)

// View is what the admin has open in the app while asking: a territory, and
// the map they have scrolled to in it. Only ids arrive from the browser; the
// names here are read from the congregation's own records.
type View struct {
	TerritoryCode string
	TerritoryName string
	MapName       string
}

// ResolveView reads the territory and map the admin has open, within the
// asker's congregation. Ids that are unknown or belong elsewhere are dropped,
// so a request cannot point the assistant at another congregation's data.
func ResolveView(s Scope, territoryID, mapID string) *View {
	if territoryID == "" {
		return nil
	}
	var t struct {
		Code string `db:"code"`
		Name string `db:"name"`
	}
	if err := s.App.DB().NewQuery(`SELECT code, COALESCE(description, '') AS name FROM territories
		WHERE id = {:id} AND congregation = {:congregation}`).
		Bind(dbx.Params{"id": territoryID, "congregation": s.CongregationID}).One(&t); err != nil || t.Code == "" {
		return nil
	}
	v := &View{TerritoryCode: t.Code, TerritoryName: resolveLocalized(t.Name, s.Locale)}
	if mapID != "" {
		var name string
		if err := s.App.DB().NewQuery(`SELECT COALESCE(description, '') FROM maps
			WHERE id = {:id} AND territory = {:territory} AND congregation = {:congregation}`).
			Bind(dbx.Params{"id": mapID, "territory": territoryID, "congregation": s.CongregationID}).
			Row(&name); err == nil {
			v.MapName = resolveLocalized(name, s.Locale)
		}
	}
	return v
}

// viewPrompt tells the model what "this territory" and "this map" mean.
func viewPrompt(v *View) string {
	if v == nil {
		return ""
	}
	var b strings.Builder
	territory := v.TerritoryCode
	if v.TerritoryName != "" {
		territory = fmt.Sprintf("%s (%s)", v.TerritoryCode, v.TerritoryName)
	}
	fmt.Fprintf(&b, "\n\nVIEW: The admin has territory %s open in the app", territory)
	if v.MapName != "" {
		fmt.Fprintf(&b, ", scrolled to the map %s", v.MapName)
	}
	fmt.Fprintf(&b, `. "This territory", "here" and "these maps" mean %s`, v.TerritoryCode)
	if v.MapName != "" {
		fmt.Fprintf(&b, `; "this map", "this block" and "this street" mean %s in %s`, v.MapName, v.TerritoryCode)
	}
	b.WriteString(". Use the view only when the question refers to it; a question naming another place is about that place.")
	return b.String()
}
