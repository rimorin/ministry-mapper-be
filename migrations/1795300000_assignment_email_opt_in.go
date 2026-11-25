package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// Assignment emails become opt-in: every account starts with them off.
func init() {
	m.Register(func(app core.App) error {
		if err := addFieldsIfMissing(app, "users", &core.BoolField{Name: "assignment_emails"}); err != nil {
			return err
		}
		return removeUserField(app, "assignment_emails_off")
	}, func(app core.App) error {
		if err := addFieldsIfMissing(app, "users", &core.BoolField{Name: "assignment_emails_off"}); err != nil {
			return err
		}
		return removeUserField(app, "assignment_emails")
	})
}

func removeUserField(app core.App, name string) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil || users.Fields.GetByName(name) == nil {
		return nil
	}
	users.Fields.RemoveByName(name)
	return app.Save(users)
}
