package handlers

import (
	"net/http"
	"slices"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/subscriptions"
)

// myMapsTopic is not a collection. "My maps" follows the assignments that name
// its account, and a link stops naming an account the moment it is taken off,
// so that one update never reaches it. This topic carries it instead.
const myMapsTopic = "my-maps/*"

// nudgeMyMaps tells the listed accounts' open sessions to load their links again.
func nudgeMyMaps(app core.App, users ...string) {
	if len(users) == 0 {
		return
	}
	message := subscriptions.Message{Name: myMapsTopic, Data: []byte("{}")}
	for _, client := range app.SubscriptionsBroker().Clients() {
		auth, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
		if auth == nil || !slices.Contains(users, auth.Id) || !client.HasSubscription(myMapsTopic) {
			continue
		}
		// Send waits for the session to read it; the request should not.
		go client.Send(message)
	}
}

// A link is worked by a car group at most; the field's own limit matches.
const maxAssignees = 20

// AssigneeNotifierFn emails the listed accounts about a link they were given.
// Injected from the jobs package at registration time to avoid import cycles.
type AssigneeNotifierFn func(app core.App, assignment *core.Record, only []string)

type UpdateAssigneesRequest struct {
	Assignment string   `json:"assignment"`
	Assignees  []string `json:"assignees"`
	// The name shown for the link, when the caller wants it to follow the members.
	Publisher string `json:"publisher"`
}

// HandleUpdateAssignees changes which accounts an open link is given to. It is
// a route because the collection has no update rule: the rest of a link, its
// map and expiry above all, stays fixed once made.
func HandleUpdateAssignees(c *core.RequestEvent, app core.App, notify AssigneeNotifierFn) error {
	var req UpdateAssigneesRequest
	if err := c.BindBody(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}
	if req.Assignment == "" {
		return apis.NewBadRequestError("assignment is required", nil)
	}
	if len(req.Assignees) > maxAssignees {
		return apis.NewBadRequestError("Too many assignees", nil)
	}

	assignment, err := app.FindRecordById("assignments", req.Assignment)
	if err != nil {
		return apis.NewNotFoundError("Assignment not found", nil)
	}
	congregation := assignment.GetString("congregation")
	if !AuthorizeByRole(app, c.Auth.Id, congregation, "administrator", "conductor") {
		return apis.NewForbiddenError("Administrator or conductor access required", nil)
	}
	if !assignment.GetDateTime("expiry_date").Time().After(time.Now()) {
		return apis.NewBadRequestError("This link has expired", nil)
	}

	before := assignment.GetStringSlice("assignees")
	assignees := []string{}
	for _, id := range req.Assignees {
		if slices.Contains(assignees, id) {
			continue
		}
		// Someone already on the link stays on it when the rest are changed.
		if !slices.Contains(before, id) && !mayHoldLink(app, id, congregation) {
			return apis.NewBadRequestError("An assignee is not a member of this congregation", nil)
		}
		assignees = append(assignees, id)
	}

	added := []string{}
	for _, id := range assignees {
		if !slices.Contains(before, id) {
			added = append(added, id)
		}
	}
	removed := slices.DeleteFunc(slices.Clone(before), func(id string) bool {
		return slices.Contains(assignees, id)
	})

	assignment.Set("assignees", assignees)
	if req.Publisher != "" {
		assignment.Set("publisher", req.Publisher)
	}
	if err := app.Save(assignment); err != nil {
		return newServerError(err)
	}
	writeAssignmentLog(app, assignment, c.Auth, "reassigned")
	nudgeMyMaps(app, removed...)
	if len(added) > 0 {
		notify(app, assignment, added)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"assignees": assignees,
		"publisher": assignment.GetString("publisher"),
	})
}

// RemoveFromHeldLinks takes an account off the links it was given in a
// congregation it no longer belongs to: "My maps" would otherwise keep
// reaching those maps until they expire.
//
// It should be called from OnRecordAfterDeleteSuccess("roles").
func RemoveFromHeldLinks(e *core.RecordEvent) {
	userID := e.Record.GetString("user")
	congregation := e.Record.GetString("congregation")
	if userID == "" || congregation == "" || holdsAnyRole(e.App, userID, congregation) {
		return
	}

	var held []struct {
		ID string `db:"id"`
	}
	err := e.App.DB().NewQuery(`
		SELECT id FROM assignments
		WHERE congregation = {:congregation}
		  AND EXISTS (SELECT 1 FROM json_each(assignees) WHERE value = {:user})
	`).Bind(dbx.Params{"congregation": congregation, "user": userID}).All(&held)
	if err != nil {
		e.App.Logger().Error("RemoveFromHeldLinks: query failed", "user", userID, "error", err)
		return
	}
	for _, row := range held {
		assignment, err := e.App.FindRecordById("assignments", row.ID)
		if err != nil {
			continue
		}
		remaining := slices.DeleteFunc(assignment.GetStringSlice("assignees"), func(id string) bool {
			return id == userID
		})
		assignment.Set("assignees", remaining)
		// SaveNoValidate: the link may already be past a validation the
		// removal has nothing to do with, and the others must still see it.
		if err := e.App.SaveNoValidate(assignment); err != nil {
			e.App.Logger().Error("RemoveFromHeldLinks: save failed", "assignment", row.ID, "error", err)
		}
	}
	if len(held) > 0 {
		nudgeMyMaps(e.App, userID)
	}
}
