package jobs

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/routine"
)

type assigneeEmail struct {
	To, Name, Subject, HTML, Text string
}

type assignmentGivenTmplData struct {
	emailChrome
	UserName     string
	MapName      string
	Territory    string
	Congregation string
	Assigner     string
	Expires      string
	IsPersonal   bool
}

// NotifyAssignees emails each account a new link was given to, so the map
// reaches someone whose app is closed. The creator is skipped when they gave
// the link to themselves, and so is anyone who has not turned these emails on. With
// only set, just those accounts are emailed: the ones added to a link that
// already existed. A failed send is logged and never fails the link.
//
// The emails are built on the request, while the records are at hand, and
// sent off it: the conductor does not wait on the mail provider.
func NotifyAssignees(app core.App, assignment *core.Record, only []string) {
	emails := assigneeEmails(app, assignment, only)
	if len(emails) == 0 {
		return
	}
	routine.FireAndForget(func() { sendAssigneeEmails(emails, assignment.Id) })
}

func sendAssigneeEmails(emails []assigneeEmail, linkID string) {
	for _, email := range emails {
		if err := sendPlainEmail(email.To, email.Name, email.Subject, email.HTML, email.Text); err != nil {
			log.Printf("NotifyAssignees: email failed for link %s: %v", linkID, err)
		}
	}
}

func assigneeEmails(app core.App, assignment *core.Record, only []string) []assigneeEmail {
	assignees := assignment.GetStringSlice("assignees")
	if only != nil {
		assignees = only
	}
	if len(assignees) == 0 {
		return nil
	}

	var details struct {
		MapName      string `db:"map_name"`
		Territory    string `db:"territory"`
		Congregation string `db:"congregation"`
		Timezone     string `db:"timezone"`
		Assigner     string `db:"assigner"`
	}
	err := app.DB().NewQuery(`
		SELECT COALESCE(m.description, '') AS map_name,
		       COALESCE(t.code, '')        AS territory,
		       COALESCE(c.name, '')        AS congregation,
		       COALESCE(c.timezone, '')    AS timezone,
		       COALESCE(u.name, '')        AS assigner
		FROM maps m
		JOIN congregations c ON c.id = {:congregation}
		LEFT JOIN territories t ON t.id = m.territory
		LEFT JOIN users u ON u.id = {:user}
		WHERE m.id = {:map}
	`).Bind(dbx.Params{
		"map":          assignment.GetString("map"),
		"congregation": assignment.GetString("congregation"),
		"user":         assignment.GetString("user"),
	}).One(&details)
	if err != nil {
		log.Printf("NotifyAssignees: cannot load link %s: %v", assignment.Id, err)
		return nil
	}

	location, err := time.LoadLocation(details.Timezone)
	if err != nil {
		location = time.UTC
	}
	expires := assignment.GetDateTime("expiry_date").Time().In(location).Format("Mon 2 Jan, 3:04 PM")
	mapName := englishName(details.MapName)
	isPersonal := assignment.GetString("type") == "personal"
	// The link id is the token, as in a link a conductor shares by hand.
	mapURL := strings.TrimRight(os.Getenv("PB_APP_URL"), "/") + "/map/" + assignment.Id

	var emails []assigneeEmail
	for _, id := range assignees {
		if id == assignment.GetString("user") {
			continue
		}
		user, err := app.FindRecordById("users", id)
		if err != nil || user.GetBool("disabled") || !user.GetBool("assignment_emails") || user.Email() == "" {
			continue
		}
		data := assignmentGivenTmplData{
			emailChrome: emailChrome{
				Preheader:   fmt.Sprintf("%s, until %s.", mapName, expires),
				Kicker:      details.Congregation,
				Title:       "You were given a map",
				Subtitle:    mapName,
				ButtonLabel: "Open the map",
				ButtonURL:   mapURL,
				Footer:      "Sent when a map is given to your Ministry Mapper account. The button stops working when the link expires.",
			},
			UserName:     displayName(user.GetString("name"), user.Email()),
			MapName:      mapName,
			Territory:    details.Territory,
			Congregation: details.Congregation,
			Assigner:     details.Assigner,
			Expires:      expires,
			IsPersonal:   isPersonal,
		}
		htmlBody, textBody, err := renderEmail("assignment_given.html", data)
		if err != nil {
			log.Printf("NotifyAssignees: %v", err)
			return nil
		}
		emails = append(emails, assigneeEmail{
			To:      user.Email(),
			Name:    user.GetString("name"),
			Subject: "Ministry Mapper: You were given " + mapName,
			HTML:    htmlBody,
			Text:    textBody,
		})
	}
	return emails
}

// englishName reads a map description as stored: a plain string, or a JSON
// object of translations. Emails are in English, so that is preferred.
func englishName(stored string) string {
	var names map[string]string
	if !strings.HasPrefix(stored, "{") || json.Unmarshal([]byte(stored), &names) != nil {
		return stored
	}
	if name := names["en"]; name != "" {
		return name
	}
	for _, name := range names {
		if name != "" {
			return name
		}
	}
	return stored
}
