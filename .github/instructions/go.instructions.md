---
applyTo:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---

# Go + PocketBase Quick Reference

> **Applies to:** All Go source files

## 🎯 Context

Ministry Mapper backend using **Go 1.24+** and **PocketBase v0.30.4** (BaaS with embedded SQLite, real-time, auth, file storage).

**Stack:** PocketBase | SQLite | Sentry | LaunchDarkly | MailerSend

## 🚨 Critical Rules

**ALWAYS:**
- ✅ Use `app.Dao()` for database operations (triggers hooks & validation)
- ✅ Parameterized queries: `dbx.Params{"field": value}` (prevent SQL injection)
- ✅ Call `e.Next()` in all hooks
- ✅ Log to Sentry before returning API errors
- ✅ Use `apis.RequireAuth()` for protected routes

**NEVER:**
- ❌ String concatenation in SQL
- ❌ Use `app.Dao()` inside transactions (use `txDao`)
- ❌ Modify in after-success hooks with `Save()` (use `SaveNoValidate()`)

## Essential Patterns

### Hooks

```go
// Before create - modify before save
app.OnRecordCreate("addresses").BindFunc(func(e *core.RecordEvent) error {
    e.Record.Set("created_at", time.Now())
    return e.Next() // REQUIRED
})

// After update - side effects
app.OnRecordAfterUpdateSuccess("addresses").BindFunc(func(e *core.RecordEvent) error {
    ProcessMapAggregates(e.Record.Get("map").(string), e.App, false)
    return e.Next()
})

// Compare old vs new
app.OnRecordUpdate("addresses").BindFunc(func(e *core.RecordEvent) error {
    if e.Record.Original().Get("notes") != e.Record.Get("notes") {
        e.Record.Set("notes_updated", time.Now())
    }
    return e.Next()
})
```

**Hook Order:** OnBefore → Validate → OnAfter → OnAfterSuccess/Error

### Database Queries

```go
// Find one (PREFERRED)
record, err := app.Dao().FindFirstRecordByFilter(
    "users",
    "email = {:email} && active = {:active}",
    dbx.Params{"email": email, "active": true},
)

// Find many
records, err := app.Dao().FindRecordsByFilter(
    "posts",
    "status = {:status}",
    "-created",  // sort DESC
    50, 0,       // limit, offset
    dbx.Params{"status": "published"},
)

// Raw SQL (complex queries)
var count struct { Total int `db:"total"` }
app.DB().NewQuery("SELECT COUNT(*) as total FROM table WHERE id = {:id}").
    Bind(dbx.Params{"id": id}).One(&count)
```

### CRUD Operations

```go
// Create
collection, _ := app.Dao().FindCollectionByNameOrId("users")
record := core.NewRecord(collection)
record.Set("email", "user@example.com")
app.Dao().Save(record) // With validation

// Update
record, _ := app.Dao().FindRecordById("users", userId)
record.Set("name", "New Name")
app.Dao().Save(record)

// Delete
app.Dao().Delete(record)

// Expand relations (avoid N+1)
app.Dao().ExpandRecord(record, []string{"author", "category"}, nil)
author := record.ExpandedOne("author")
```

### API Routes

```go
app.OnServe().BindFunc(func(e *core.ServeEvent) error {
    e.Router.POST("/api/custom", func(c *core.RequestEvent) error {
        // Auth
        authRecord := c.Auth()
        if authRecord == nil {
            return apis.NewUnauthorizedError("Login required", nil)
        }
        
        // Request data
        requestInfo, _ := c.RequestInfo()
        data := requestInfo.Body
        
        // Process
        result, err := processData(data["field"].(string))
        if err != nil {
            sentry.CaptureException(err)
            return apis.NewBadRequestError("Failed", nil)
        }
        
        return c.JSON(200, map[string]interface{}{"result": result})
    }).Bind(apis.RequireAuth())
    
    return e.Next()
})
```

### Transactions

```go
err := app.Dao().RunInTransaction(func(txDao *daos.Dao) error {
    // Use txDao, NOT app.Dao()
    r1, _ := txDao.FindRecordById("col1", id1)
    r1.Set("value", 1)
    if err := txDao.Save(r1); err != nil {
        return err // Rollback
    }
    
    r2, _ := txDao.FindRecordById("col2", id2)
    if err := txDao.Save(r2); err != nil {
        return err // Rollback
    }
    
    return nil // Commit
})
```

### Error Handling

```go
if err != nil {
    sentry.CaptureException(err)
    return apis.NewBadRequestError("User message", nil)
}

// Other errors:
// apis.NewNotFoundError("message", nil)
// apis.NewForbiddenError("message", nil)
// apis.NewUnauthorizedError("message", nil)
```

## Common Gotchas

```go
// ❌ WRONG - panics if nil/wrong type
value := record.Get("field").(string)

// ✅ CORRECT - safe type assertion
value, ok := record.Get("field").(string)
if !ok { return errors.New("invalid") }

// ❌ WRONG - files are always slices
filename := record.Get("avatar").(string)

// ✅ CORRECT
files := record.Get("avatar").([]string)
if len(files) > 0 { filename := files[0] }

// ❌ WRONG - infinite loop
app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
    e.Record.Set("x", true)
    app.Dao().Save(e.Record) // Triggers hook again!
    return e.Next()
})

// ✅ CORRECT - modify then let it save
app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
    e.Record.Set("x", true)
    return e.Next() // Saved after hooks
})

// ❌ WRONG - using app.Dao() in transaction
app.Dao().RunInTransaction(func(txDao *daos.Dao) error {
    app.Dao().FindRecordById("x", id) // WRONG!
})

// ✅ CORRECT - use txDao
app.Dao().RunInTransaction(func(txDao *daos.Dao) error {
    txDao.FindRecordById("x", id) // CORRECT
})
```

## Quick Reference

**Filter Operators:** `=` `!=` `>` `>=` `<` `<=` `~`(LIKE) `?=`(contains) `&&`(AND) `||`(OR)

**Key Functions:**
- `app.Dao().FindRecordById(collection, id)`
- `app.Dao().FindFirstRecordByFilter(col, filter, params)`
- `app.Dao().FindRecordsByFilter(col, filter, sort, limit, offset, params)`
- `app.Dao().Save(record)` / `SaveNoValidate(record)`
- `app.Dao().Delete(record)`
- `app.Dao().ExpandRecord(record, []string{"relation"}, nil)`
- `app.Dao().RunInTransaction(func(txDao *daos.Dao) error {...})`

**Auth Middleware:**
- `apis.RequireAuth()` - Any authenticated user
- `apis.RequireAdminAuth()` - Admin only
- `apis.RequireAuth("users")` - Specific collection

**Resources:**
- [PocketBase Docs](https://pocketbase.io/docs/)
- [Go API](https://pkg.go.dev/github.com/pocketbase/pocketbase)
