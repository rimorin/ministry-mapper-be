package middleware

import (
	"net/http"
	"regexp"
	"strings"
)

// Matches a record id only, so backend routes such as /map/add are untouched.
var linkIDInPath = regexp.MustCompile(`(/map/|/assignments/records/)[a-z0-9]{15}\b`)

// ScrubURL drops the query, fragment and any share link id.
func ScrubURL(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	return linkIDInPath.ReplaceAllString(raw, "${1}:id")
}

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
