package middleware

import (
	"net/http/httptest"
	"testing"
)

func TestScrubURL(t *testing.T) {
	cases := map[string]string{
		"https://app.example.test/map/abc123def456ghi":                   "https://app.example.test/map/:id",
		"https://app.example.test/map/abc123def456ghi?x=1#top":           "https://app.example.test/map/:id",
		"/api/collections/assignments/records/abc123def456ghi?fields=id": "/api/collections/assignments/records/:id",
		"/api/collections/users/records?filter=email%3D%22a%40b.test%22": "/api/collections/users/records",
		"/auth/callback?code=secret&state=s&name=Someone":                "/auth/callback",
		"/map/add":              "/map/add",
		"/map/territory/update": "/map/territory/update",
		"/address/update":       "/address/update",
		"":                      "",
	}
	for in, want := range cases {
		if got := ScrubURL(in); got != want {
			t.Errorf("ScrubURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScrubbedRequestDropsLinkIDAndQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/collections/assignments/records/abc123def456ghi?filter=secret", nil)
	req.Header.Set("link-id", "abc123def456ghi")
	req.Header.Set("Referer", "https://app.example.test/map/abc123def456ghi?x=1")
	req.Header.Set("User-Agent", "test")

	clean := scrubbedRequest(req)

	if got := clean.Header.Get("link-id"); got != "" {
		t.Errorf("link-id header kept: %q", got)
	}
	if got := clean.Header.Get("Referer"); got != "https://app.example.test/map/:id" {
		t.Errorf("Referer = %q", got)
	}
	if got := clean.URL.String(); got != "/api/collections/assignments/records/:id" {
		t.Errorf("URL = %q", got)
	}
	if clean.Header.Get("User-Agent") != "test" {
		t.Error("other headers should be kept")
	}
	if req.Header.Get("link-id") == "" || req.URL.RawQuery == "" {
		t.Error("the original request was changed")
	}
}
