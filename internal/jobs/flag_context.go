package jobs

import (
	"slices"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// staffLevels ranks the staff roles as the frontend's access levels do.
var staffLevels = map[string]int{"read_only": 1, "conductor": 2, "administrator": 3}

// flagContext builds the LaunchDarkly context the frontend builds
// (src/lib/launchdarkly.ts), so a rule written for the app matches here too.
// A lookup that fails leaves its attribute out.
func flagContext(app core.App, userID, congregationID string) ldcontext.Context {
	params := dbx.Params{"user": userID, "congregation": congregationID}

	var memberships []struct {
		Role         string `db:"role"`
		Congregation string `db:"congregation"`
	}
	_ = app.DB().NewQuery(`SELECT role, congregation FROM roles WHERE user = {:user}`).
		Bind(params).All(&memberships)

	var congregationName string
	_ = app.DB().NewQuery(`SELECT name FROM congregations WHERE id = {:congregation}`).
		Bind(params).Row(&congregationName)

	groups := []string{}
	_ = app.DB().NewQuery(`
		SELECT name FROM groups
		WHERE congregation = {:congregation}
		  AND EXISTS (SELECT 1 FROM json_each(members) WHERE value = {:user})
		ORDER BY name
	`).Bind(params).Column(&groups)

	roles, congregations := []string{}, []string{}
	maxAccessLevel, activeRole := 0, ""
	for _, m := range memberships {
		if !slices.Contains(roles, m.Role) {
			roles = append(roles, m.Role)
		}
		if !slices.Contains(congregations, m.Congregation) {
			congregations = append(congregations, m.Congregation)
		}
		if level, staff := staffLevels[m.Role]; staff {
			maxAccessLevel = max(maxAccessLevel, level)
			if m.Congregation == congregationID {
				activeRole = m.Role
			}
		}
	}

	user := ldcontext.NewBuilder(userID).
		SetValue("roles", ldvalue.CopyArbitraryValue(roles)).
		SetInt("maxAccessLevel", maxAccessLevel).
		SetValue("congregations", ldvalue.CopyArbitraryValue(congregations)).
		SetValue("groups", ldvalue.CopyArbitraryValue(groups)).
		Private("email", "name")
	if record, err := app.FindRecordById("users", userID); err == nil {
		user.Name(record.GetString("name")).SetString("email", record.GetString("email"))
	}

	congregation := ldcontext.NewBuilder(congregationID).
		Kind("congregation").
		SetString("name", congregationName).
		SetString("activeRole", activeRole).
		Build()

	return ldcontext.NewMulti(sharedLDContext, user.Build(), congregation)
}
