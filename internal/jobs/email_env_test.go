package jobs

import "testing"

func TestAllowedRecipientsByEnvironment(t *testing.T) {
	everyone := []Recipient{
		{Name: "A", Email: "a@one.test"},
		{Name: "B", Email: "b@two.test"},
	}

	t.Setenv("EMAIL_ALLOWLIST", "")
	t.Setenv("APP_ENV", "")
	if got := allowedRecipients(everyone); len(got) != 2 {
		t.Errorf("unset environment kept %d of 2 recipients", len(got))
	}

	t.Setenv("APP_ENV", "staging")
	if got := allowedRecipients(everyone); len(got) != 0 {
		t.Errorf("staging with no allow-list kept %d recipients", len(got))
	}

	t.Setenv("EMAIL_ALLOWLIST", "b@two.test")
	got := allowedRecipients(everyone)
	if len(got) != 1 || got[0].Email != "b@two.test" {
		t.Errorf("staging kept %v, want only b@two.test", got)
	}
}

func TestDeliverSkipsWhenNobodyIsAllowed(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("EMAIL_ALLOWLIST", "")
	t.Setenv("MAILERSEND_API_KEY", "not-a-real-key")
	t.Setenv("MAILERSEND_FROM_EMAIL", "from@one.test")

	if err := deliver([]Recipient{{Name: "A", Email: "a@one.test"}}, "Subject", "<p>x</p>", "x"); err != nil {
		t.Fatalf("deliver returned %v, want nil", err)
	}
}
