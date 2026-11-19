package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// Lets an account turn off the email it gets when a map is given to it. The
// field is the "off" switch so that every existing account keeps the default.
func init() {
	m.Register(func(app core.App) error {
		return addFieldsIfMissing(app, "users", &core.BoolField{Name: "assignment_emails_off"})
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil || users.Fields.GetByName("assignment_emails_off") == nil {
			return nil
		}
		users.Fields.RemoveByName("assignment_emails_off")
		return app.Save(users)
	})
}
