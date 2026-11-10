package assistant

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Who may use each function. The assistant follows what each role sees and
// does on the admin screens, which is sometimes stricter than the API: a
// read-only user does not see the user list, and a conductor cannot reset a
// territory there. Roles rank as the app ranks them (ACCESS_LEVEL_MAPPING in
// the front end), so a function open to conductors is open to administrators.
//
// A function above the asker's role is never offered to the model, and a call
// to one is refused, so this table is the whole policy. Confirming a card
// then runs through the screen's own call, which the backend checks again.

const (
	roleReadOnly      = "read_only"
	roleConductor     = "conductor"
	roleAdministrator = "administrator"
)

var roleRank = map[string]int{roleReadOnly: 1, roleConductor: 2, roleAdministrator: 3}

// atLeast reports whether role ranks at or above min. An unknown role ranks
// below everything.
func atLeast(role, min string) bool {
	have, ok := roleRank[role]
	return ok && have >= roleRank[min]
}

var toolRoles = map[string]string{
	// What every role sees: territories, maps, homes, messages and notes.
	"find_places":        roleReadOnly,
	"calculate":          roleReadOnly,
	"territory_overview": roleReadOnly,
	"map_overview":       roleReadOnly,
	"map_details":        roleReadOnly,
	"maps_near":          roleReadOnly,
	"activity":           roleReadOnly,
	"territory_rounds":   roleReadOnly,
	"household_types":    roleReadOnly,
	"not_home_backlog":   roleReadOnly,
	"do_not_call_review": roleReadOnly,
	"data_quality":       roleReadOnly,
	"feedback":           roleReadOnly,

	// Conductors run the field service: who holds which map and who came out.
	"assignments":           roleConductor,
	"who_worked":            roleConductor,
	"change_history":        roleConductor, // role changes need administrator; see argRoles
	"congregation_settings": roleConductor,
	"propose_share_link":    roleConductor, // personal links need administrator; see argRoles
	"propose_link_revoke":   roleConductor,
	"propose_quick_link":    roleConductor,
	"propose_home_update":   roleConductor,
	"propose_tries_reset":   roleConductor,
	"propose_homes_status":  roleConductor,

	// The Congregation menu and every structural change are administrators'.
	"people_and_roles":         roleAdministrator,
	"propose_role_change":      roleAdministrator,
	"propose_role_grant":       roleAdministrator,
	"propose_map_rename":       roleAdministrator,
	"propose_territory_rename": roleAdministrator,
	"propose_pin_message":      roleAdministrator,
	"propose_report_email":     roleAdministrator,
	"propose_mark_read":        roleAdministrator,
	"propose_unpin_message":    roleAdministrator,
	"propose_map_location":     roleAdministrator,
	"propose_map_reset":        roleAdministrator,
	"propose_territory_reset":  roleAdministrator,
	"propose_dnc_return":       roleAdministrator,
	"propose_settings":         roleAdministrator,
	"propose_map_move":         roleAdministrator,
	"propose_floor_add":        roleAdministrator,
	"propose_unit_add":         roleAdministrator,
	"propose_territory_create": roleAdministrator,
	"propose_maps_create":      roleAdministrator,
}

// argRoles raise the role for one value of one argument, where a function is
// open to a role for everything but that.
var argRoles = []struct{ tool, arg, value, role string }{
	{"change_history", "kind", "role", roleAdministrator},
	{"propose_share_link", "type", "personal", roleAdministrator},
}

// allowed says whether role may make this call. The refusal goes back to the
// model, which tells the asker which role it needs.
func allowed(role, tool string, args json.RawMessage) error {
	min, ok := toolRoles[tool]
	if !ok {
		return badArgs("unknown function")
	}
	if !atLeast(role, min) {
		return badArgs("this needs the %s role; tell the asker so", roleName(min))
	}
	var values map[string]any
	_ = json.Unmarshal(args, &values)
	for _, r := range argRoles {
		if r.tool == tool && fmt.Sprint(values[r.arg]) == r.value && !atLeast(role, r.role) {
			return badArgs("%s %s needs the %s role; tell the asker so", r.arg, r.value, roleName(r.role))
		}
	}
	return nil
}

// toolsFor is the functions offered to the model for role.
func toolsFor(role string) []Tool {
	var out []Tool
	for _, t := range Tools {
		if atLeast(role, toolRoles[t.Name]) {
			out = append(out, t)
		}
	}
	return out
}

// rolePrompt tells the model who is asking and what needs a higher role, so
// it can say so instead of guessing at an answer it has no function for.
func rolePrompt(role string) string {
	byRole := map[string][]string{}
	for _, t := range Tools {
		if min := toolRoles[t.Name]; !atLeast(role, min) {
			byRole[min] = append(byRole[min], t.Name)
		}
	}
	for _, r := range argRoles {
		if atLeast(role, toolRoles[r.tool]) && !atLeast(role, r.role) {
			byRole[r.role] = append(byRole[r.role], fmt.Sprintf("%s with %s %s", r.tool, r.arg, r.value))
		}
	}
	var b strings.Builder
	// Saying what is allowed first matters: told only what is not, the model
	// refused changes the role may make.
	fmt.Fprintf(&b, "The asker's role is %s. Every function you are given is allowed for this role: use it "+
		"whenever the question needs it, and never refuse something a given function can do.", roleName(role))
	for _, min := range []string{roleConductor, roleAdministrator} {
		if names := byRole[min]; len(names) > 0 {
			sort.Strings(names)
			fmt.Fprintf(&b, " Only these need the %s role: %s. If the asker needs one of them, say it needs that "+
				"role and do not answer it another way.", roleName(min), strings.Join(names, ", "))
		}
	}
	return b.String()
}

func roleName(role string) string {
	switch role {
	case roleAdministrator:
		return "administrator"
	case roleConductor:
		return "conductor"
	default:
		return "read-only"
	}
}
