package jobs

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mailersend/mailersend-go"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// unprovisionedUser holds the database row fields needed to process an unprovisioned user.
type unprovisionedUser struct {
	ID                              string `db:"id"`
	Name                            string `db:"name"`
	Email                           string `db:"email"`
	Disabled                        bool   `db:"disabled"`
	Created                         string `db:"created"`
	UnprovisionedSince              string `db:"unprovisioned_since"`
	UnprovisionedWarningSentAt      string `db:"unprovisioned_warning_sent_at"`
	UnprovisionedFinalWarningSentAt string `db:"unprovisioned_final_warning_sent_at"`
	AdminAlertedAt                  string `db:"admin_alerted_at"`
}

type unprovisionedUserTmplData struct {
	UserName      string
	DaysRemaining int
	AppURL        string
}

type unprovisionedAdminAlertTmplData struct {
	AdminName string
	NewUsers  []unprovisionedNewUser
	AppURL    string
}

type unprovisionedNewUser struct {
	ID      string
	Name    string
	Email   string
	Created string
}

// processUnprovisionedUsers enforces the unprovisioned-user lifecycle aligned with NIST SP 800-53 AC-2
// (automated account management) and IAM best-practice least-privilege principles.
//
// Timeline:
//   - On detection: Alert congregation administrators the first time the job runs after an
//     unprovisioned account is found (guarded by admin_alerted_at field — fires exactly once
//     regardless of job timing drift or downtime).
//   - Day 3:   Send first warning email to the user with two clear action paths.
//   - Day 6:   Send final warning email (24 h notice).
//   - Day 7:   Disable the account (NIST AC-2 "prompt deprovisioning").
//   - Day 37+: Permanently delete the disabled account (30-day investigation window per
//     "disable first, delete later" IAM best practice).
func processUnprovisionedUsers(app *pocketbase.PocketBase) error {
	log.Println("processUnprovisionedUsers: starting")

	appURL := os.Getenv("PB_APP_URL")

	// Fetch all users with zero role assignments.
	var users []unprovisionedUser
	err := app.DB().NewQuery(`
		SELECT
			u.id, u.name, u.email, u.disabled, u.created,
			COALESCE(u.unprovisioned_since, '')               AS unprovisioned_since,
			COALESCE(u.unprovisioned_warning_sent_at, '')       AS unprovisioned_warning_sent_at,
			COALESCE(u.unprovisioned_final_warning_sent_at, '') AS unprovisioned_final_warning_sent_at,
			COALESCE(u.admin_alerted_at, '')                    AS admin_alerted_at
		FROM users u
		LEFT JOIN roles r ON r.user = u.id
		GROUP BY u.id
		HAVING COUNT(r.id) = 0
	`).All(&users)
	if err != nil {
		return fmt.Errorf("processUnprovisionedUsers: query failed: %w", err)
	}

	if len(users) == 0 {
		log.Println("processUnprovisionedUsers: no unprovisioned users found")
		return nil
	}

	log.Printf("processUnprovisionedUsers: processing %d unprovisioned user(s)", len(users))

	now := time.Now().UTC()
	var newlyCreated []unprovisionedNewUser

	for _, u := range users {
		// Use unprovisioned_since if set (role was revoked from an existing user).
		// Fall back to created for brand-new accounts that never had a role.
		ageRef := u.Created
		if u.UnprovisionedSince != "" {
			ageRef = u.UnprovisionedSince
		}
		age := accountAgeDays(ageRef, now)

		// Admin alert — queue if admins have not yet been notified about this account.
		// Guard against disabled accounts: if email delivery was down when the account
		// was first disabled, admin_alerted_at is never stamped. Without the !u.Disabled
		// check, the alert would re-queue on every run until Day 37 deletion.
		if u.AdminAlertedAt == "" && !u.Disabled {
			newlyCreated = append(newlyCreated, unprovisionedNewUser{
				ID:      u.ID,
				Name:    u.Name,
				Email:   u.Email,
				Created: formatCreated(u.Created),
			})
		}

		if u.Disabled {
			// Delete — disabled accounts past the 30-day investigation window.
			if age >= 37 {
				record, err := app.FindRecordById("users", u.ID)
				if err != nil {
					log.Printf("processUnprovisionedUsers: cannot find user %s for deletion: %v", u.ID, err)
					continue
				}
				if err := app.Delete(record); err != nil {
					log.Printf("processUnprovisionedUsers: failed to delete user %s: %v", u.ID, err)
					continue
				}
				log.Printf("processUnprovisionedUsers: deleted unprovisioned user %s (account age %d days)", u.Email, age)
			}
			continue
		}

		// Disable — unprovisioned for 7+ days without a role assignment (NIST AC-2).
		if age >= 7 {
			record, err := app.FindRecordById("users", u.ID)
			if err != nil {
				log.Printf("processUnprovisionedUsers: cannot find user %s to disable: %v", u.ID, err)
				continue
			}
			record.Set("disabled", true)
			if err := app.SaveNoValidate(record); err != nil {
				log.Printf("processUnprovisionedUsers: failed to disable user %s: %v", u.ID, err)
				continue
			}
			log.Printf("processUnprovisionedUsers: disabled unprovisioned user %s (account age %d days)", u.Email, age)
			continue
		}

		// Final warning — 24 h before disable (day 6).
		if age >= 6 && u.UnprovisionedFinalWarningSentAt == "" {
			daysLeft := 7 - age
			if err := sendUnprovisionedUserEmail(u.Email, u.Name, true, daysLeft, appURL); err != nil {
				log.Printf("processUnprovisionedUsers: final warning email failed for %s: %v", u.Email, err)
				continue
			}
			if err := updateUserField(app, u.ID, "unprovisioned_final_warning_sent_at", now.Format(time.RFC3339)); err != nil {
				log.Printf("CRITICAL processUnprovisionedUsers: final warning sent to %s but timestamp not saved — duplicate email may be sent on next run: %v", u.Email, err)
			}
			continue
		}

		// First warning — 4 days remaining (day 3).
		if age >= 3 && u.UnprovisionedWarningSentAt == "" {
			daysLeft := 7 - age
			if err := sendUnprovisionedUserEmail(u.Email, u.Name, false, daysLeft, appURL); err != nil {
				log.Printf("processUnprovisionedUsers: warning email failed for %s: %v", u.Email, err)
				continue
			}
			if err := updateUserField(app, u.ID, "unprovisioned_warning_sent_at", now.Format(time.RFC3339)); err != nil {
				log.Printf("CRITICAL processUnprovisionedUsers: warning sent to %s but timestamp not saved — duplicate email may be sent on next run: %v", u.Email, err)
			}
		}
	}

	// Alert congregation admins about any unprovisioned accounts not yet flagged.
	// Only stamp admin_alerted_at after at least one admin was successfully notified.
	if len(newlyCreated) > 0 {
		sent, err := alertAdminsUnprovisionedUsers(app, newlyCreated, appURL)
		if err != nil {
			log.Printf("processUnprovisionedUsers: admin alert failed: %v", err)
		} else if sent > 0 {
			for _, nu := range newlyCreated {
				if err := updateUserField(app, nu.ID, "admin_alerted_at", now.Format(time.RFC3339)); err != nil {
					log.Printf("CRITICAL processUnprovisionedUsers: admin alert sent for %s but admin_alerted_at not saved — alert may re-send: %v", nu.Email, err)
				}
			}
		}
	}

	log.Println("processUnprovisionedUsers: completed")
	return nil
}

// sendUnprovisionedUserEmail sends a warning or final-warning email to an unprovisioned user.
func sendUnprovisionedUserEmail(toEmail, toName string, isFinal bool, daysRemaining int, appURL string) error {
	templateFile := "templates/user_unprovisioned_warning.html"
	subject := "Ministry Mapper: Please complete your account setup"
	if isFinal {
		templateFile = "templates/user_unprovisioned_final_warning.html"
		subject = "Ministry Mapper: Your account will be deactivated in 24 hours"
	}

	tmpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return fmt.Errorf("sendUnprovisionedUserEmail: parse template: %w", err)
	}

	data := unprovisionedUserTmplData{
		UserName:      displayName(toName, toEmail),
		DaysRemaining: daysRemaining,
		AppURL:        appURL,
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("sendUnprovisionedUserEmail: execute template: %w", err)
	}

	return sendPlainEmail(toEmail, toName, subject, body.String())
}

// alertAdminsUnprovisionedUsers notifies all PocketBase superadmins about unprovisioned accounts.
// Superadmins are system-level owners and are the correct recipients since unprovisioned users
// have no congregation linkage yet — there is no congregation-scoped admin to notify.
// Returns the number of superadmins successfully notified so the caller can decide whether to
// stamp admin_alerted_at.
func alertAdminsUnprovisionedUsers(app *pocketbase.PocketBase, newUsers []unprovisionedNewUser, appURL string) (int, error) {
	superusers, err := app.FindRecordsByFilter(core.CollectionNameSuperusers, "", "", 0, 0)
	if err != nil {
		return 0, fmt.Errorf("alertAdminsUnprovisionedUsers: query failed: %w", err)
	}

	if len(superusers) == 0 {
		log.Println("alertAdminsUnprovisionedUsers: no superadmins found to alert")
		return 0, nil
	}

	tmpl, err := template.ParseFiles("templates/user_unprovisioned_admin_alert.html")
	if err != nil {
		return 0, fmt.Errorf("alertAdminsUnprovisionedUsers: parse template: %w", err)
	}

	subject := fmt.Sprintf("Ministry Mapper: %d unprovisioned account(s) require attention", len(newUsers))
	sent := 0

	for _, su := range superusers {
		email := su.GetString("email")
		data := unprovisionedAdminAlertTmplData{
			AdminName: "Superadmin",
			NewUsers:  newUsers,
			AppURL:    appURL,
		}
		var body bytes.Buffer
		if err := tmpl.Execute(&body, data); err != nil {
			log.Printf("alertAdminsUnprovisionedUsers: template error for superadmin %s: %v", email, err)
			continue
		}
		if err := sendPlainEmail(email, "Superadmin", subject, body.String()); err != nil {
			log.Printf("alertAdminsUnprovisionedUsers: email failed for superadmin %s: %v", email, err)
			continue
		}
		sent++
	}

	return sent, nil
}

// updateUserField sets a single string field on a user record without triggering full validation.
func updateUserField(app *pocketbase.PocketBase, userID, field, value string) error {
	record, err := app.FindRecordById("users", userID)
	if err != nil {
		return err
	}
	record.Set(field, value)
	return app.SaveNoValidate(record)
}

// sendPlainEmail sends a single HTML email via MailerSend.
// Shared by all user management job emails.
func sendPlainEmail(toEmail, toName, subject, htmlBody string) error {
	apiKey := os.Getenv("MAILERSEND_API_KEY")
	fromEmail := os.Getenv("MAILERSEND_FROM_EMAIL")
	if apiKey == "" || fromEmail == "" {
		return fmt.Errorf("sendPlainEmail: MAILERSEND_API_KEY or MAILERSEND_FROM_EMAIL not configured")
	}

	ms := mailersend.NewMailersend(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	message := ms.Email.NewMessage()
	message.SetFrom(mailersend.From{Email: fromEmail, Name: "Ministry Mapper"})
	message.SetRecipients([]mailersend.Recipient{{Email: toEmail, Name: toName}})
	message.SetSubject(subject)
	message.SetHTML(htmlBody)

	_, err := ms.Email.Send(ctx, message)
	return err
}

// accountAgeDays calculates how many full days have elapsed since the account was created.
func accountAgeDays(created string, now time.Time) int {
	t, err := parsePBDate(created)
	if err != nil {
		return 0
	}
	return int(now.Sub(t).Hours() / 24)
}

// formatCreated returns a human-readable creation timestamp for display in emails.
func formatCreated(created string) string {
	t, err := parsePBDate(created)
	if err != nil {
		return created
	}
	return t.Format("2 Jan 2006 15:04 UTC")
}

// displayName returns a user-friendly display name.
// Falls back to the email local-part if name is blank (e.g. user skipped the name field on sign-up).
func displayName(name, email string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	if idx := strings.Index(email, "@"); idx > 0 {
		return email[:idx]
	}
	return email
}

// parsePBDate parses a PocketBase date string (multiple formats are attempted).
func parsePBDate(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05.999Z07:00",
		"2006-01-02 15:04:05.999Z",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05Z",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("parsePBDate: cannot parse %q", s)
}
