package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// logEmailFields lists, per audit log, the user relations that get a
// <field>_email copy.
var logEmailFields = map[string][]string{
	"roles_log":       {"user", "changed_by"},
	"assignments_log": {"user", "changed_by"},
	"territories_log": {"changed_by"},
}

// Adds an email copy beside each user relation in the audit logs. Deleting a
// user clears every relation to them, and accounts are purged automatically,
// so the email keeps a row readable afterwards. Existing rows are filled in
// from users that still exist; for the rest it is already gone.
func init() {
	m.Register(func(app core.App) error {
		for table, fields := range logEmailFields {
			var emailFields []core.Field
			for _, field := range fields {
				emailFields = append(emailFields, &core.TextField{Name: field + "_email"})
			}
			if err := addFieldsIfMissing(app, table, emailFields...); err != nil {
				return err
			}
			for _, field := range fields {
				email := field + "_email"
				if _, err := app.DB().NewQuery(
					"UPDATE {{" + table + "}} SET [[" + email + "]] = " +
						"COALESCE((SELECT email FROM users WHERE users.id = {{" + table + "}}.[[" + field + "]]), '') " +
						"WHERE COALESCE([[" + email + "]], '') = ''",
				).Execute(); err != nil {
					return err
				}
			}
		}
		return nil
	}, func(app core.App) error {
		for table, fields := range logEmailFields {
			collection, err := app.FindCollectionByNameOrId(table)
			if err != nil {
				continue
			}
			for _, field := range fields {
				collection.Fields.RemoveByName(field + "_email")
			}
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		return nil
	})
}
