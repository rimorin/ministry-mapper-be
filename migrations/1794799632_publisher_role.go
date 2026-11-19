package migrations

import (
	"slices"
	"strings"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

const (
	publisherRole = "publisher"
	// A link is worked by a car group at most.
	maxAssignees = 20

	// The assignments list rule names the filters a caller may list by.
	listByUser         = `@request.query.filter ~ "user="`
	listByCongregation = listByUser + ` || @request.query.filter ~ "congregation="`
)

// Adds the publisher role and ties a link to accounts: assignments.assignees
// are the members a link was given to (assignments.user stays the one who made
// it). The log keeps them and their emails like its other user fields.
//
// Links can also be listed by congregation, which is how the link form counts
// the open links each member holds; the list hook scopes the rows as before.
func init() {
	m.Register(func(app core.App) error {
		if err := setPublisherRole(app, true); err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return nil
		}
		assignees := func() core.Field {
			return &core.RelationField{Name: "assignees", CollectionId: users.Id, MaxSelect: maxAssignees}
		}
		if err := addFieldsIfMissing(app, "assignments", assignees()); err != nil {
			return err
		}
		if err := addFieldsIfMissing(app, "assignments_log", assignees(), &core.TextField{Name: "assignee_emails"}); err != nil {
			return err
		}
		return replaceInAssignmentsListRule(app, listByUser, listByCongregation)
	}, func(app core.App) error {
		if err := replaceInAssignmentsListRule(app, listByCongregation, listByUser); err != nil {
			return err
		}
		for table, fields := range map[string][]string{
			"assignments":     {"assignees"},
			"assignments_log": {"assignees", "assignee_emails"},
		} {
			collection, err := app.FindCollectionByNameOrId(table)
			if err != nil {
				continue
			}
			for _, field := range fields {
				collection.Fields.RemoveByName(field)
			}
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		// A select field refuses a stored value it no longer lists.
		if _, err := app.DB().NewQuery("DELETE FROM roles WHERE role = {:role}").
			Bind(map[string]any{"role": publisherRole}).Execute(); err != nil {
			return err
		}
		return setPublisherRole(app, false)
	})
}

func setPublisherRole(app core.App, allowed bool) error {
	collection, err := app.FindCollectionByNameOrId("roles")
	if err != nil {
		return nil
	}
	field, ok := collection.Fields.GetByName("role").(*core.SelectField)
	if !ok || slices.Contains(field.Values, publisherRole) == allowed {
		return nil
	}
	if allowed {
		field.Values = append(field.Values, publisherRole)
	} else {
		field.Values = slices.DeleteFunc(field.Values, func(v string) bool { return v == publisherRole })
	}
	return app.Save(collection)
}

// replaceInAssignmentsListRule swaps one clause of the rule for another, and
// does nothing once the new clause is in place.
func replaceInAssignmentsListRule(app core.App, from, to string) error {
	collection, err := app.FindCollectionByNameOrId("assignments")
	if err != nil || collection.ListRule == nil {
		return nil
	}
	rule := *collection.ListRule
	// The longer clause contains the shorter, so check for it first.
	longer := max(from, to)
	if strings.Contains(rule, longer) == (to == longer) {
		return nil
	}
	rule = strings.Replace(rule, from, to, 1)
	collection.ListRule = &rule
	return app.Save(collection)
}
