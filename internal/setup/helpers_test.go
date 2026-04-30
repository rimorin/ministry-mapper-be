package setup

import (
	"testing"

	"ministry-mapper/internal/handlers"

	"github.com/pocketbase/pocketbase/tests"
)

const testDataDir = "../../test_pb_data"

func generateToken(email string) (string, error) {
	app, err := tests.NewTestApp(testDataDir)
	if err != nil {
		return "", err
	}
	defer app.Cleanup()

	record, err := app.FindAuthRecordByEmail("users", email)
	if err != nil {
		return "", err
	}
	return record.NewAuthToken()
}

func setupTestApp(t testing.TB) *tests.TestApp {
	testApp, err := tests.NewTestApp(testDataDir)
	if err != nil {
		t.Fatal(err)
	}
	RegisterRoutes(testApp)
	handlers.RegisterAuthHooks(testApp)
	RegisterDomainHooks(testApp)
	return testApp
}
