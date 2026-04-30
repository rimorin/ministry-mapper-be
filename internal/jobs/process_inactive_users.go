package jobs

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"os"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// inactiveUser holds the database row fields needed to process an inactive user.
type inactiveUser struct {
	ID                         string `db:"id"`
	Name                       string `db:"name"`
	Email                      string `db:"email"`
	LastLogin                  string `db:"last_login"`
	Created                    string `db:"created"`
	InactiveWarningSentAt      string `db:"inactive_warning_sent_at"`
	InactiveFinalWarningSentAt string `db:"inactive_final_warning_sent_at"`
}

type inactiveUserTmplData struct {
	UserName     string
	LastLogin    string
	DeadlineDate string
	DaysLeft     int
	AppURL       string
}

const (
	// inactivityWarnDays is the day threshold for the first warning (~3 months).
	inactivityWarnDays = 91
	// inactivityFinalWarnDays is the threshold for the final warning (~5 months, 30 days before disable).
	inactivityFinalWarnDays = 152
	// inactivityDisableDays is the threshold for disabling the account (~6 months).
	// Aligns with NIST SP 800-53 AC-2(3): automatic disabling of inactive accounts.
	inactivityDisableDays = 183
)

// processInactiveUsers enforces the inactive-user lifecycle aligned with NIST SP 800-53 AC-2(3)
// (disable inactive accounts) and IAM best practices for retention-focused applications.
//
// Timeline:
//   - Day 91  (~3 months): Send first warning — "account disabled in ~3 months".
//   - Day 152 (~5 months): Send final warning — "account disabled in 30 days".
//   - Day 183 (~6 months): Disable the account.
//
// Accounts are never auto-deleted — congregation ministry history must be preserved.
// Administrators may re-enable or permanently delete accounts manually.
//
// The inactivity clock uses last_login if available, falling back to the account
// creation date for users who have never logged in.
func processInactiveUsers(app core.App) error {
	log.Println("processInactiveUsers: starting")

	appURL := os.Getenv("PB_APP_URL")

	// Fetch all enabled users who may be inactive (last_login or created is old enough).
	// The threshold is intentionally loose (>= warn threshold) so we process all candidates
	// in Go; the exact day-count logic is computed from the actual date values.
	var users []inactiveUser
	err := app.DB().NewQuery(`
		SELECT
			id, name, email,
			COALESCE(last_login, '')                    AS last_login,
			created,
			COALESCE(inactive_warning_sent_at, '')      AS inactive_warning_sent_at,
			COALESCE(inactive_final_warning_sent_at,'') AS inactive_final_warning_sent_at
		FROM users
		WHERE disabled = false
		  AND (
		        (last_login IS NOT NULL AND last_login != '' AND CAST(JULIANDAY('now') - JULIANDAY(last_login) AS INTEGER) >= {:warnDays})
		     OR ((last_login IS NULL OR last_login = '') AND CAST(JULIANDAY('now') - JULIANDAY(created) AS INTEGER) >= {:warnDays})
		  )
	`).Bind(map[string]any{"warnDays": inactivityWarnDays}).All(&users)
	if err != nil {
		return fmt.Errorf("processInactiveUsers: query failed: %w", err)
	}

	if len(users) == 0 {
		log.Println("processInactiveUsers: no inactive users found")
		return nil
	}

	log.Printf("processInactiveUsers: processing %d inactive user(s)", len(users))

	now := time.Now().UTC()

	for _, u := range users {
		inactive := inactiveDays(u, now)

		// Disable — inactive for 6+ months (NIST AC-2(3)).
		if inactive >= inactivityDisableDays {
			record, err := app.FindRecordById("users", u.ID)
			if err != nil {
				log.Printf("processInactiveUsers: cannot find user %s: %v", u.ID, err)
				continue
			}
			record.Set("disabled", true)
			if err := app.SaveNoValidate(record); err != nil {
				log.Printf("processInactiveUsers: failed to disable user %s: %v", u.ID, err)
				continue
			}
			log.Printf("processInactiveUsers: disabled inactive user %s (%d days inactive)", u.Email, inactive)
			continue
		}

		// Final warning — 30 days before disable (~5 months).
		if inactive >= inactivityFinalWarnDays && u.InactiveFinalWarningSentAt == "" {
			daysLeft := inactivityDisableDays - inactive
			deadline := now.AddDate(0, 0, daysLeft).Format("2 January 2006")
			if err := sendInactiveUserEmail(u, true, deadline, daysLeft, appURL); err != nil {
				log.Printf("processInactiveUsers: final warning email failed for %s: %v", u.Email, err)
				continue
			}
			if err := updateUserField(app, u.ID, "inactive_final_warning_sent_at", now.Format(time.RFC3339)); err != nil {
				log.Printf("CRITICAL processInactiveUsers: final warning sent to %s but timestamp not saved — duplicate email may be sent on next run: %v", u.Email, err)
			}
			continue
		}

		// First warning — inactivity threshold reached (~3 months).
		if inactive >= inactivityWarnDays && u.InactiveWarningSentAt == "" {
			daysLeft := inactivityDisableDays - inactive
			deadline := now.AddDate(0, 0, daysLeft).Format("2 January 2006")
			if err := sendInactiveUserEmail(u, false, deadline, daysLeft, appURL); err != nil {
				log.Printf("processInactiveUsers: warning email failed for %s: %v", u.Email, err)
				continue
			}
			if err := updateUserField(app, u.ID, "inactive_warning_sent_at", now.Format(time.RFC3339)); err != nil {
				log.Printf("CRITICAL processInactiveUsers: warning sent to %s but timestamp not saved — duplicate email may be sent on next run: %v", u.Email, err)
			}
		}
	}

	log.Println("processInactiveUsers: completed")
	return nil
}

// sendInactiveUserEmail sends a warning or final-warning email to an inactive user.
func sendInactiveUserEmail(u inactiveUser, isFinal bool, deadlineDate string, daysLeft int, appURL string) error {
	templateFile := "templates/user_inactive_warning.html"
	subject := "Ministry Mapper: Your account will be deactivated due to inactivity"

	if isFinal {
		templateFile = "templates/user_inactive_final_warning.html"
		unit := "days"
		if daysLeft == 1 {
			unit = "day"
		}
		subject = fmt.Sprintf("Ministry Mapper: Your account will be deactivated in %d %s", daysLeft, unit)
	}

	tmpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return fmt.Errorf("sendInactiveUserEmail: parse template: %w", err)
	}

	lastLoginDisplay := "Never"
	if u.LastLogin != "" {
		if t, err := parsePBDate(u.LastLogin); err == nil {
			lastLoginDisplay = t.Format("2 January 2006")
		}
	}

	data := inactiveUserTmplData{
		UserName:     displayName(u.Name, u.Email),
		LastLogin:    lastLoginDisplay,
		DeadlineDate: deadlineDate,
		DaysLeft:     daysLeft,
		AppURL:       appURL,
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("sendInactiveUserEmail: execute template: %w", err)
	}

	return sendPlainEmail(u.Email, u.Name, subject, body.String())
}

// inactiveDays returns the number of days since the user last interacted with the system.
// Falls back to the account creation date for users who have never logged in.
func inactiveDays(u inactiveUser, now time.Time) int {
	ref := u.Created
	if u.LastLogin != "" {
		ref = u.LastLogin
	}
	t, err := parsePBDate(ref)
	if err != nil {
		return 0
	}
	return int(now.Sub(t).Hours() / 24)
}
