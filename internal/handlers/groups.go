package handlers

import (
	"slices"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// registerGroupHooks scopes groups like territories: the congregation's staff
// see them and its administrators change them. A group only narrows the member
// picker, so it opens nothing and the link-id never reaches it.
func registerGroupHooks(app core.App) {
	staff := func(auth *core.Record, congId string) bool {
		return congId != "" && AuthorizeByRole(app, auth.Id, congId)
	}
	noLink := func(string) bool { return false }

	app.OnRecordsListRequest("groups").BindFunc(func(e *core.RecordsListRequestEvent) error {
		congIds := uniqueStrings(extractIdsFromFilter(congIdPattern, e.Request.URL.Query().Get("filter")))
		if err := authorizeList(e,
			func() bool { return authorizedForAllCongregations(app, e.Auth.Id, congIds) },
			noLink,
		); err != nil {
			return err
		}
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		keep, err := congregationScopeKeep(app, e.Auth.Id)
		if err != nil {
			return apis.NewForbiddenError("Unauthorized", nil)
		}
		filterListResults(e, keep)
		return e.Next()
	})
	app.OnRecordViewRequest("groups").BindFunc(func(e *core.RecordRequestEvent) error {
		return authorizeView(e, func() bool { return staff(e.Auth, getCongId(e, false)) }, noLink)
	})
	app.OnRecordEnrich("groups").BindFunc(func(e *core.RecordEnrichEvent) error {
		congId := e.Record.GetString("congregation")
		return scopeEnrich(e, func(auth *core.Record) bool { return staff(auth, congId) }, noLink)
	})

	app.OnRecordCreateRequest("groups").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, false))
	})
	app.OnRecordUpdateRequest("groups").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, true))
	})
	app.OnRecordDeleteRequest("groups").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, true))
	})
	app.OnRecordCreateRequest("groups").BindFunc(func(e *core.RecordRequestEvent) error {
		return checkGroup(e, app)
	})
	app.OnRecordUpdateRequest("groups").BindFunc(func(e *core.RecordRequestEvent) error {
		return checkGroup(e, app)
	})
}

// checkGroup keeps a group inside its congregation: it stays where it was
// made, and everyone added to it belongs there. Bind it after adminOnly, so a
// caller without access learns nothing about who is a member.
func checkGroup(e *core.RecordRequestEvent, app core.App) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	congId := getCongId(e, false)
	var before []string
	if !e.Record.IsNew() {
		original := e.Record.Original()
		if original.GetString("congregation") != congId {
			return apis.NewBadRequestError("A group cannot be moved to another congregation", nil)
		}
		before = original.GetStringSlice("members")
	}
	for _, member := range e.Record.GetStringSlice("members") {
		if !slices.Contains(before, member) && !holdsAnyRole(app, member, congId) {
			return apis.NewBadRequestError("A member is not in this congregation", nil)
		}
	}
	return e.Next()
}

// RemoveFromGroups takes an account out of the groups of a congregation it no
// longer belongs to.
//
// It should be called from OnRecordAfterDeleteSuccess("roles").
func RemoveFromGroups(e *core.RecordEvent) {
	userID := e.Record.GetString("user")
	congregation := e.Record.GetString("congregation")
	if userID == "" || congregation == "" || holdsAnyRole(e.App, userID, congregation) {
		return
	}
	groups, err := e.App.FindAllRecords("groups", dbx.NewExp(
		"congregation = {:congregation} AND EXISTS (SELECT 1 FROM json_each(members) WHERE value = {:user})",
		dbx.Params{"congregation": congregation, "user": userID}))
	if err != nil {
		e.App.Logger().Error("RemoveFromGroups: query failed", "user", userID, "error", err)
		return
	}
	for _, group := range groups {
		group.Set("members", slices.DeleteFunc(group.GetStringSlice("members"), func(id string) bool {
			return id == userID
		}))
		if err := e.App.SaveNoValidate(group); err != nil {
			e.App.Logger().Error("RemoveFromGroups: save failed", "group", group.Id, "error", err)
		}
	}
}
