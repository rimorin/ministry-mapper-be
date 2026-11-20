package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// appleCallbackPath is the web app's page that finishes an OAuth2 sign-in
// (OAUTH2_REDIRECT_PATH in the front end).
const appleCallbackPath = "/auth/callback"

// maxAppleNameRunes bounds the name passed on, which arrives from the browser.
const maxAppleNameRunes = 100

// HandleAppleCallback receives Apple's reply to a sign-in and hands it to the
// web app, which finishes the sign-in as it does for Google.
//
// Apple answers with a form post, which a page of the web app cannot read, so
// its return URL is this route. Nothing is trusted or stored here: the web
// app checks the state against the one it sent, and PocketBase verifies the
// code with Apple. The redirect goes only to PB_APP_URL.
//
// Apple sends the person's name on their first sign-in only, and only here,
// so it is passed on for the new account.
func HandleAppleCallback(c *core.RequestEvent) error {
	appURL := strings.TrimRight(os.Getenv("PB_APP_URL"), "/")
	if appURL == "" {
		return apis.NewApiError(http.StatusServiceUnavailable, "Sign in with Apple is not configured", nil)
	}
	var data struct {
		State string `form:"state" json:"state"`
		Code  string `form:"code" json:"code"`
		Error string `form:"error" json:"error"`
		User  string `form:"user" json:"user"`
	}
	if err := c.BindBody(&data); err != nil {
		data.Error = "invalid_request"
	}

	query := url.Values{}
	switch {
	case data.Error != "":
		query.Set("error", data.Error)
	case data.Code == "" || data.State == "":
		query.Set("error", "invalid_request")
	default:
		query.Set("code", data.Code)
		query.Set("state", data.State)
		if name := appleName(data.User); name != "" {
			query.Set("name", name)
		}
	}
	return c.Redirect(http.StatusSeeOther, appURL+appleCallbackPath+"?"+query.Encode())
}

// appleName reads the name out of the user details Apple posts, or "".
func appleName(raw string) string {
	var user struct {
		Name struct {
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
		} `json:"name"`
	}
	if raw == "" || json.Unmarshal([]byte(raw), &user) != nil {
		return ""
	}
	name := strings.Join(strings.Fields(user.Name.FirstName+" "+user.Name.LastName), " ")
	if utf8.RuneCountInString(name) > maxAppleNameRunes {
		return ""
	}
	return name
}
