---
applyTo:
  - "**/*_test.go"
---

# Testing Instructions

> **Applies to:** All Go test files

## 🧪 Testing Philosophy

Tests ensure reliability for production congregations. Focus on critical business logic, data integrity, and API contracts.

## Critical Testing Rules

**ALWAYS:**
- ✅ Clean up test data with `defer testApp.Cleanup()`
- ✅ Test both success and error paths
- ✅ Make tests independent (no shared state)
- ✅ Use descriptive test names explaining what's being tested
- ✅ Create minimal test data needed for each test
- ✅ Validate expected behavior, not implementation details

**NEVER:**
- ❌ Depend on test execution order
- ❌ Use production data or credentials
- ❌ Leave test data in database after test runs
- ❌ Test external services without mocks
- ❌ Write flaky tests that sometimes pass/fail

## Test File Structure

```go
package handlers_test

import (
    "testing"
    "github.com/pocketbase/pocketbase/tests"
    "github.com/pocketbase/pocketbase/core"
)

func TestFeatureName(t *testing.T) {
    // Arrange - Setup test app and data
    testApp, err := tests.NewTestApp()
    if err != nil {
        t.Fatal(err)
    }
    defer testApp.Cleanup()
    
    // Create test data
    collection, _ := testApp.Dao().FindCollectionByNameOrId("users")
    record := core.NewRecord(collection)
    record.Set("email", "test@example.com")
    testApp.Dao().Save(record)
    
    // Act - Execute the functionality
    result, err := YourFunction(testApp, record.Id)
    
    // Assert - Verify expectations
    if err != nil {
        t.Errorf("Expected no error, got %v", err)
    }
    
    if result != expectedValue {
        t.Errorf("Expected %v, got %v", expectedValue, result)
    }
}
```

## PocketBase Test Patterns

### Unit Testing with Test App

```go
func TestDatabaseOperation(t *testing.T) {
    testApp, err := tests.NewTestApp()
    if err != nil {
        t.Fatal(err)
    }
    defer testApp.Cleanup()
    
    // Test your database logic
    record, err := testApp.Dao().FindFirstRecordByFilter(
        "collection",
        "field = {:value}",
        dbx.Params{"value": "test"},
    )
    
    if err != nil {
        t.Fatal(err)
    }
    
    if record.Get("field") != "test" {
        t.Error("Field value mismatch")
    }
}
```

### API Scenario Testing

```go
func TestAPIEndpoint(t *testing.T) {
    tests.ApiScenario{
        Method: "POST",
        URL:    "/api/custom/endpoint",
        Body:   strings.NewReader(`{"field": "value"}`),
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
        TestAppFactory: func(t testing.TB) *tests.TestApp {
            testApp, _ := tests.NewTestApp()
            // Setup test data
            return testApp
        },
        ExpectedStatus:  200,
        ExpectedContent: []string{`"success"`},
        ExpectedEvents:  map[string]int{"OnRecordCreate": 1},
        BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
            // Pre-test setup
        },
        AfterTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
            // Post-test verification
        },
    }.Test(t)
}
```

### Testing with Authentication

```go
func TestAuthenticatedEndpoint(t *testing.T) {
    testApp, _ := tests.NewTestApp()
    defer testApp.Cleanup()
    
    // Create test user
    collection, _ := testApp.Dao().FindCollectionByNameOrId("users")
    user := core.NewRecord(collection)
    user.Set("email", "test@example.com")
    user.SetPassword("test123456")
    testApp.Dao().Save(user)
    
    // Generate auth token
    token, err := user.NewAuthToken()
    if err != nil {
        t.Fatal(err)
    }
    
    // Test with authentication
    tests.ApiScenario{
        Method: "GET",
        URL:    "/api/protected",
        Headers: map[string]string{
            "Authorization": "Bearer " + token,
        },
        TestAppFactory: func(t testing.TB) *tests.TestApp {
            return testApp
        },
        ExpectedStatus: 200,
    }.Test(t)
}
```

### Testing Hooks

```go
func TestRecordHook(t *testing.T) {
    testApp, _ := tests.NewTestApp()
    defer testApp.Cleanup()
    
    hookCalled := false
    
    // Register test hook
    testApp.OnRecordCreate("posts").BindFunc(func(e *core.RecordEvent) error {
        hookCalled = true
        return e.Next()
    })
    
    // Create record to trigger hook
    collection, _ := testApp.Dao().FindCollectionByNameOrId("posts")
    record := core.NewRecord(collection)
    record.Set("title", "Test")
    testApp.Dao().Save(record)
    
    if !hookCalled {
        t.Error("Hook was not called")
    }
}
```

## Test Data Best Practices

### Minimal Test Data

Create only what's needed for the specific test:

```go
// Good: Minimal required fields
record := core.NewRecord(collection)
record.Set("email", "test@example.com")
record.Set("name", "Test User")

// Avoid: Setting unnecessary fields
record.Set("profile_picture", "...")
record.Set("bio", "...")
record.Set("preferences", "...")
```

### Realistic Test Data

Use realistic values that match production patterns:

```go
// Good: Realistic email
record.Set("email", "john.doe@example.com")

// Avoid: Unrealistic values
record.Set("email", "test@test.test")
```

### Unique Test Data

Prevent conflicts with unique constraints:

```go
// Use timestamps or random strings for uniqueness
import "time"

email := fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
record.Set("email", email)
```

## Testing Complex Scenarios

### Testing Transactions

```go
func TestTransaction(t *testing.T) {
    testApp, _ := tests.NewTestApp()
    defer testApp.Cleanup()
    
    err := testApp.Dao().RunInTransaction(func(txDao *daos.Dao) error {
        // Create first record
        collection1, _ := txDao.FindCollectionByNameOrId("collection1")
        record1 := core.NewRecord(collection1)
        record1.Set("field", "value")
        if err := txDao.Save(record1); err != nil {
            return err
        }
        
        // Create second record
        collection2, _ := txDao.FindCollectionByNameOrId("collection2")
        record2 := core.NewRecord(collection2)
        record2.Set("related_id", record1.Id)
        return txDao.Save(record2)
    })
    
    if err != nil {
        t.Errorf("Transaction failed: %v", err)
    }
}
```

### Testing Aggregates

```go
func TestAggregateCalculation(t *testing.T) {
    testApp, _ := tests.NewTestApp()
    defer testApp.Cleanup()
    
    // Create test addresses
    collection, _ := testApp.Dao().FindCollectionByNameOrId("addresses")
    
    for i := 0; i < 5; i++ {
        record := core.NewRecord(collection)
        record.Set("map", "map123")
        if i < 3 {
            record.Set("status", "done")
        } else {
            record.Set("status", "not_done")
        }
        testApp.Dao().Save(record)
    }
    
    // Run aggregate function
    result := CalculateMapProgress(testApp, "map123")
    
    // Verify: 3 done out of 5 total = 60%
    expected := 0.6
    if result != expected {
        t.Errorf("Expected progress %.2f, got %.2f", expected, result)
    }
}
```

### Testing Background Jobs

```go
func TestBackgroundJob(t *testing.T) {
    testApp, _ := tests.NewTestApp()
    defer testApp.Cleanup()
    
    // Setup test data
    // ... create test records
    
    // Execute job function directly (not via scheduler)
    err := CleanUpExpiredAssignments(testApp)
    
    if err != nil {
        t.Errorf("Job failed: %v", err)
    }
    
    // Verify job effects
    records, _ := testApp.Dao().FindRecordsByFilter(
        "assignments",
        "expiry_date < {:now}",
        dbx.Params{"now": time.Now()},
    )
    
    if len(records) != 0 {
        t.Error("Expired assignments were not cleaned up")
    }
}
```

## Common Testing Patterns

### Table-Driven Tests

```go
func TestValidation(t *testing.T) {
    tests := []struct {
        name      string
        input     string
        wantError bool
    }{
        {"Valid email", "test@example.com", false},
        {"Invalid email", "not-an-email", true},
        {"Empty email", "", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateEmail(tt.input)
            if (err != nil) != tt.wantError {
                t.Errorf("ValidateEmail(%q) error = %v, wantError %v", 
                    tt.input, err, tt.wantError)
            }
        })
    }
}
```

### Subtests for Organization

```go
func TestMapOperations(t *testing.T) {
    testApp, _ := tests.NewTestApp()
    defer testApp.Cleanup()
    
    t.Run("CreateMap", func(t *testing.T) {
        // Test map creation
    })
    
    t.Run("UpdateMap", func(t *testing.T) {
        // Test map update
    })
    
    t.Run("DeleteMap", func(t *testing.T) {
        // Test map deletion
    })
}
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run specific package
go test ./internal/handlers/...

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific test
go test -run TestSpecificTest

# Run tests with race detector
go test -race ./...

# Run tests with timeout
go test -timeout 30s ./...
```

## Test Naming Conventions

```go
// Good: Descriptive names
func TestHandleMapAdd_ValidInput_CreatesMap(t *testing.T) { }
func TestHandleMapAdd_DuplicateCode_ReturnsError(t *testing.T) { }
func TestAggregateCalculation_EmptyMap_ReturnsZero(t *testing.T) { }

// Avoid: Vague names
func TestMap(t *testing.T) { }
func TestFunction1(t *testing.T) { }
func TestStuff(t *testing.T) { }
```

## Error Message Best Practices

```go
// Good: Helpful error messages
if result != expected {
    t.Errorf("CalculateProgress(%q) = %d; want %d", 
        mapId, result, expected)
}

// Good: Show actual vs expected
if len(records) != 5 {
    t.Errorf("Expected 5 records, got %d. Records: %+v", 
        len(records), records)
}

// Avoid: Unclear messages
if result != expected {
    t.Error("Wrong result")
}
```

## Mocking External Services

```go
// Mock MailerSend in tests
type MockMailer struct {
    SentEmails []string
}

func (m *MockMailer) SendEmail(to, subject, body string) error {
    m.SentEmails = append(m.SentEmails, to)
    return nil
}

func TestEmailNotification(t *testing.T) {
    mockMailer := &MockMailer{}
    
    // Use mock in your function
    SendNotification(mockMailer, "test@example.com", "Subject", "Body")
    
    if len(mockMailer.SentEmails) != 1 {
        t.Error("Email was not sent")
    }
}
```

## Test Coverage Goals

- **Critical paths**: 100% coverage (auth, data integrity, aggregates)
- **Business logic**: 80%+ coverage (handlers, jobs)
- **Utility functions**: 70%+ coverage
- **Happy path + error cases**: Always test both

## Debugging Failing Tests

```go
// Add debug output
t.Logf("Debug: record = %+v", record)
t.Logf("Debug: query returned %d results", len(results))

// Use t.Helper() for test helpers
func createTestUser(t *testing.T, app *tests.TestApp) *core.Record {
    t.Helper()  // Makes error traces point to caller
    // ... creation logic
}

// Skip flaky tests temporarily
if testing.Short() {
    t.Skip("Skipping in short mode")
}
```

## Best Practices Summary

1. **Isolation**: Each test is independent
2. **Cleanup**: Always defer cleanup
3. **Minimal Data**: Create only what's needed
4. **Realistic Data**: Match production patterns
5. **Both Paths**: Test success and errors
6. **Descriptive Names**: Explain what's being tested
7. **Clear Messages**: Show expected vs actual
8. **No External Deps**: Mock APIs, email, etc.
9. **Fast Tests**: Keep test execution under 1 second per test
10. **Maintainable**: Tests should be easy to understand and update
