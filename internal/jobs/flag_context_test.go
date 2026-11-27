//go:build testdata

package jobs

import (
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/tests"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ld "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdata"
)

// Rules written in LaunchDarkly against the frontend's context (a
// congregation's name and the caller's role there, a user's groups) have to
// match the backend's check too.
func TestAssistantFlagMatchesTheFrontendContext(t *testing.T) {
	app, err := tests.NewTestApp("../../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	td := ldtestdata.DataSource()
	td.Update(td.Flag("enable-admin-assistant").BooleanFlag().FallthroughVariation(false).
		IfMatchContext("congregation", "name", ldvalue.String("Alpha Congregation")).
		AndMatchContext("congregation", "activeRole", ldvalue.String("administrator")).
		ThenReturn(true).
		IfMatchContext("congregation", "name", ldvalue.String("Alpha Congregation")).
		AndMatchContext(ldcontext.DefaultKind, "groups", ldvalue.String("Group 1")).
		ThenReturn(true))
	client, err := ld.MakeCustomClient("test", ld.Config{DataSource: td, Events: ldcomponents.NoEvents()}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	previousClient, previousContext := sharedLDClient, sharedLDContext
	sharedLDClient, sharedLDContext = client, ldcontext.NewWithKind("environment", "test")
	defer func() { sharedLDClient, sharedLDContext = previousClient, previousContext }()

	for _, c := range []struct {
		name, user, congregation string
		want                     bool
	}{
		{"an administrator of the named congregation", "testuseralpha01", "testcongalpha01", true},
		{"a conductor in the group the rule names", "testuseralpha02", "testcongalpha01", true},
		{"a read-only member in no group", "testuseralpha03", "testcongalpha01", false},
		{"an administrator of another congregation", "testuserbeta001", "testcongbeta001", false},
	} {
		if got := IsAssistantEnabled(app, c.user, c.congregation); got != c.want {
			t.Errorf("%s: enabled = %v, want %v", c.name, got, c.want)
		}
	}
}
