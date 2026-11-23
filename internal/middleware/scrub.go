package middleware

import (
	"net/http"
	"regexp"
	"strings"
)

// A share link's id is the whole of a publisher's access, and it sits in the
// app's map address and in the path of its own record. What goes to Sentry
// keeps the shape of the address without the id. Only a record id is
// replaced, so this server's own /map/... routes read as they are.
var linkIDInPath = regexp.MustCompile(`(/map/|/assignments/records/)[a-z0-9]{15}\b`)

// ScrubURL returns a URL or path as it may be sent to Sentry: no query or
// fragment (filters and sign-in replies carry ids, names and codes) and no
// share link id.
func ScrubURL(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	return linkIDInPath.ReplaceAllString(raw, "${1}:id")
}

// scrubbedRequest copies a request for Sentry without the share link header,
// the query, or a link id in the path or referer. The body is left out: the
// SDK is set not to send it.
func scrubbedRequest(req *http.Request) *http.Request {
	clean := req.Clone(req.Context())
	clean.Body = http.NoBody
	clean.Header.Del("link-id")
	if referer := clean.Header.Get("Referer"); referer != "" {
		clean.Header.Set("Referer", ScrubURL(referer))
	}
	clean.URL.Path = ScrubURL(clean.URL.Path)
	clean.URL.RawPath = ""
	clean.URL.RawQuery = ""
	return clean
}
