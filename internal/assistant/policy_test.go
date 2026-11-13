//go:build testdata

package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The policy, pinned: changing who may do what must be a deliberate edit here
// as well as in policy.go. It follows the admin screens (ComponentAuthorizer
// in the front end), not the looser API rules.
func TestPolicy_FollowsTheScreens(t *testing.T) {
	for _, tool := range Tools {
		if _, ok := toolRoles[tool.Name]; !ok {
			t.Errorf("%s has no role in toolRoles; every function must say who may use it", tool.Name)
		}
	}
	if len(toolRoles) != len(Tools) {
		t.Errorf("toolRoles has %d entries for %d functions", len(toolRoles), len(Tools))
	}
	want := map[string][]string{
		roleReadOnly: {"activity", "calculate", "data_quality", "feedback", "find_places",
			"homes", "map_details", "map_overview", "maps_near", "territory_overview",
			"territory_rounds"},
		roleConductor: {"assignments", "change_history", "congregation_settings", "propose_home_update",
			"propose_homes_status", "propose_link_revoke", "propose_quick_link", "propose_share_link", "propose_tries_reset", "who_worked"},
	}
	for role, names := range want {
		var got []string
		for name, min := range toolRoles {
			if min == role {
				got = append(got, name)
			}
		}
		if strings.Join(sorted(got), ",") != strings.Join(names, ",") {
			t.Errorf("%s functions = %v, want %v", role, sorted(got), names)
		}
	}
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestPolicy_OffersOnlyWhatTheRoleMayUse(t *testing.T) {
	names := func(role string) string {
		var out []string
		for _, tool := range toolsFor(role) {
			out = append(out, tool.Name)
		}
		return "," + strings.Join(out, ",") + ","
	}
	readOnly, conductor, admin := names(roleReadOnly), names(roleConductor), names(roleAdministrator)
	for _, c := range []struct {
		list, name string
		offered    bool
	}{
		{readOnly, "find_places", true},
		{readOnly, "people_and_roles", false},
		{readOnly, "who_worked", false},
		{readOnly, "congregation_settings", false},
		{readOnly, "propose_quick_link", false},
		{conductor, "who_worked", true},
		{conductor, "change_history", true},
		{conductor, "propose_home_update", true},
		{conductor, "people_and_roles", false},
		{conductor, "propose_reset", false},
		{conductor, "propose_settings", false},
	} {
		if strings.Contains(c.list, ","+c.name+",") != c.offered {
			t.Errorf("%s offered = %v, want %v", c.name, !c.offered, c.offered)
		}
	}
	if len(toolsFor(roleAdministrator)) != len(Tools) || !strings.Contains(admin, ",propose_unit_add,") {
		t.Errorf("administrators must be offered every function")
	}
	if len(toolsFor("")) != 0 || len(toolsFor("publisher")) != 0 {
		t.Errorf("an unknown role must be offered nothing")
	}
}

func TestPolicy_RefusesACallAboveTheRole(t *testing.T) {
	s := scopeFor(t, newTestApp(t), alphaCong)
	people := newPseudonyms(alphaCong)
	run := func(role, tool, args string) string {
		s.Role = role
		step, view, err := runTool(s, people, tool, args)
		if err != nil {
			t.Fatalf("%s as %s: %v", tool, role, err)
		}
		if step.Error != "" {
			return step.Error
		}
		return view
	}

	for _, c := range []struct{ role, tool, args, want string }{
		{roleReadOnly, "people_and_roles", `{}`, "needs the administrator role"},
		{roleReadOnly, "who_worked", `{"from": "2026-01-01", "to": "2026-11-06"}`, "needs the conductor role"},
		{roleConductor, "people_and_roles", `{}`, "needs the administrator role"},
		{roleConductor, "propose_reset", `{"what": "map", "name": "Blk 100A"}`, "needs the administrator role"},
		{roleConductor, "change_history", `{"kind": "role", "within_days": 3650}`, "kind role needs the administrator role"},
		{roleConductor, "propose_share_link", `{"map": "Blk 100B", "publisher": "Sis Ong", "type": "personal", "days": 7}`,
			"type personal needs the administrator role"},
		{"", "find_places", `{"text": "Blk"}`, "needs the read-only role"},
	} {
		if got := run(c.role, c.tool, c.args); !strings.Contains(got, c.want) {
			t.Errorf("%s(%s) as %q = %s, want %q", c.tool, c.args, c.role, got, c.want)
		}
	}

	// What a role may do still works through the same path.
	for _, c := range []struct{ role, tool, args string }{
		{roleReadOnly, "find_places", `{"text": "Blk"}`},
		{roleConductor, "change_history", `{"kind": "home", "within_days": 3650}`},
		{roleConductor, "propose_share_link", `{"map": "Blk 100B", "publisher": "Sis Ong", "type": "normal"}`},
		{roleAdministrator, "change_history", `{"kind": "role", "within_days": 3650}`},
	} {
		if got := run(c.role, c.tool, c.args); strings.Contains(got, "needs the") {
			t.Errorf("%s(%s) as %s was refused: %s", c.tool, c.args, c.role, got)
		}
	}
}

func TestPolicy_PromptNamesWhatNeedsAHigherRole(t *testing.T) {
	if !strings.Contains(rolePrompt(roleConductor), "Every function you are given is allowed") {
		t.Error("the prompt must say the given functions are allowed, or the model refuses them")
	}
	readOnly := rolePrompt(roleReadOnly)
	if !strings.Contains(readOnly, "role is read-only") || !strings.Contains(readOnly, "people_and_roles") ||
		!strings.Contains(readOnly, "who_worked") {
		t.Errorf("read-only prompt = %s", readOnly)
	}
	conductor := rolePrompt(roleConductor)
	if strings.Contains(conductor, "who_worked") || !strings.Contains(conductor, "change_history with kind role") ||
		!strings.Contains(conductor, "propose_share_link with type personal") {
		t.Errorf("conductor prompt = %s", conductor)
	}
	if admin := rolePrompt(roleAdministrator); strings.Contains(admin, "Only these") {
		t.Errorf("administrators have nothing above them: %s", admin)
	}
	_ = json.Valid
}

// A read-only asker's request never carries the functions above that role,
// and a call to one anyway returns nothing but the refusal.
func TestAsk_ReadOnlyAskerIsOfferedOnlyReadOnlyFunctions(t *testing.T) {
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)
	s.Role, s.UserID = roleReadOnly, "testuseralpha03"
	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"people_and_roles", `{}`}),
		final("Only administrators can see who has which role.", false),
	}}
	resp, err := Ask(context.Background(), chat, s, Request{Question: "Who are the admins?"})
	if err != nil {
		t.Fatal(err)
	}
	offered := map[string]bool{}
	for _, tool := range chat.calls[0].Tools {
		offered[tool.OfFunction.Name] = true
	}
	if !offered["find_places"] || offered["people_and_roles"] || offered["who_worked"] || offered["propose_reset"] {
		t.Fatalf("offered = %v", offered)
	}
	if len(resp.Steps) != 1 || !strings.Contains(resp.Steps[0].Error, "needs the administrator role") || resp.Steps[0].Result != nil {
		t.Fatalf("steps = %+v", resp.Steps)
	}
	if sent := chat.sent(); strings.Contains(sent, "admin@alpha.test") || strings.Contains(sent, "last_signed_in") {
		t.Fatal("the refused function's data reached the model")
	}
}
