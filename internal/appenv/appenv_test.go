package appenv

import "testing"

func TestNameFallsBackToProduction(t *testing.T) {
	t.Setenv("APP_ENV", "")
	if Name() != Production || !IsProduction() {
		t.Fatalf("Name() = %q, want production", Name())
	}
	t.Setenv("APP_ENV", "  Staging ")
	if Name() != "staging" || IsProduction() {
		t.Fatalf("Name() = %q, want staging", Name())
	}
}

func TestMarkSubject(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	if got := MarkSubject("Hello"); got != "Hello" {
		t.Errorf("production subject = %q", got)
	}
	t.Setenv("APP_ENV", "")
	if got := MarkSubject("Hello"); got != "Hello" {
		t.Errorf("unset subject = %q", got)
	}
	t.Setenv("APP_ENV", "staging")
	if got := MarkSubject("Hello"); got != "[STAGING] Hello" {
		t.Errorf("staging subject = %q", got)
	}
}

func TestMayEmail(t *testing.T) {
	t.Setenv("EMAIL_ALLOWLIST", "")
	t.Setenv("APP_ENV", "production")
	if !MayEmail("anyone@elsewhere.test") {
		t.Error("production must email everyone")
	}
	t.Setenv("APP_ENV", "")
	if !MayEmail("anyone@elsewhere.test") {
		t.Error("an unset environment must email everyone")
	}

	t.Setenv("APP_ENV", "staging")
	if MayEmail("anyone@elsewhere.test") {
		t.Error("an empty allow-list must mean nobody")
	}

	t.Setenv("EMAIL_ALLOWLIST", " Tester@One.test , @team.test ,")
	cases := map[string]bool{
		"tester@one.test":       true,
		"TESTER@one.test":       true,
		"other@one.test":        false,
		"a@team.test":           true,
		"a@notteam.test":        false,
		"a@team.test.evil.test": false,
		"":                      false,
	}
	for address, want := range cases {
		if got := MayEmail(address); got != want {
			t.Errorf("MayEmail(%q) = %v, want %v", address, got, want)
		}
	}
}
