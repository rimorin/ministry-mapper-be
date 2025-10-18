---
applyTo:
  - "internal/handlers/**/*.go"
  - "internal/jobs/**/*.go"
  - "migrations/**/*.go"
---

# Ministry Mapper Domain Logic

> **Applies to:** Business logic, handlers, jobs, migrations

## 🎯 Context

Territory management system for religious congregations. Tracks field ministry work across territories, maps, and addresses.

**Tech:** PocketBase v0.30.4 | SQLite | Go 1.24.7 | Sentry | LaunchDarkly | MailerSend

## 🗂️ Data Model

### Core Collections

1. **congregations** - Organization units
   - Fields: `name`, `code`, `expiry_hours`, `max_tries`, `timezone`

2. **territories** - Geographical zones
   - Fields: `code`, `description`, `progress`, `congregation`
   - Aggregates: Completion % from child maps

3. **maps** - Work areas (single/multi-floor)
   - Fields: `code`, `type`, `coordinates`, `aggregates`, `territory`
   - Types: "single" or "multi" (multi-floor buildings)
   - Coordinates: `{lat, lng}` for proximity assignment
   - Aggregates: `{notDone, done, notHome, invalid, dnc}`

4. **addresses** - Individual units
   - Fields: `code`, `floor`, `sequence`, `status`, `type`, `notes`
   - Status: `not_done`, `done`, `not_home`, `dnc`, `invalid`
   - Hooks: Updates trigger aggregate recalc

5. **assignments** - User-to-map links
   - Fields: `map`, `user`, `expiry_date`
   - Used for: Workload balancing

6. **options** - Address types per congregation
   - Fields: `code`, `description`, `is_countable`, `is_default`

7. **roles** - User permissions
   - Fields: `user`, `congregation`, `role` (admin/user)

8. **users** - Authentication
   - Hooks: Email normalization, last_login tracking

## 🎯 Key Features

### 1. Intelligent Map Assignment (Quicklink)

**Algorithm Priority:**
1. **Workload** - Maps with fewer assignments
2. **Proximity** - Distance from user location (Haversine)
3. **Progress** - Lower completion percentage

```go
// File: internal/handlers/get_quicklink.go
// Endpoint: POST /territory/link
// Request: { "territory": "id", "coordinates": {"lat": x, "lng": y} }
// Response: { "map_id": "xxx", "distance": 35.7, "assignment_count": 1 }
```

**Business Rules:**
- 50-meter proximity threshold
- Skip 100% completed maps
- Create assignment on match

### 2. Real-time Aggregates

**Map Aggregates:**
```sql
-- Counts: done, not_done, not_home (with max_tries), dnc, invalid
-- Progress: (done + not_home_max_tries) / total * 100
```

**Territory Aggregates:**
- Rolls up from all maps
- Respects `is_countable` flag from options
- Updates via cron every 10 minutes

**File:** `internal/handlers/update_aggregates.go`

### 3. Background Jobs

**Scheduler:** `internal/jobs/job_scheduler.go`

| Job | Frequency | Flag | Purpose |
|-----|-----------|------|---------|
| cleanUpAssignments | */5 min | `enable-assignments-cleanup` | Remove expired |
| updateTerritoryAggregates | */10 min | `enable-territory-aggregations` | Recalc stats |
| processMessages | */30 min | `enable-message-processing` | Queue processing |
| processNotes | Hourly | `enable-note-processing` | Update notes |
| generateMonthlyReport | Monthly | `enable-monthly-report` | Excel reports |

### 4. Monthly Excel Reports

**File:** `internal/jobs/generate_report.go` (~1,474 lines)

**Sheets:**
1. Details - Congregation info, options, roles
2. DNC - "Do not call" addresses
3. Territory Sheets - Progress, maps, address grids

**Distribution:** MailerSend to all administrators

**Styling:** Professional colors (blues: #1F4E79, #4A90B8)

## 📍 API Routes

All routes require `apis.RequireAuth()`:

| Route | Handler | Purpose |
|-------|---------|---------|
| `/map/codes` | GetMapCodes | Get address codes |
| `/map/code/add` | MapAdd | Add address |
| `/map/code/delete` | MapDelete | Delete address |
| `/map/codes/update` | MapUpdateSequence | Reorder |
| `/map/floor/add` | MapFloor | Add floor |
| `/map/floor/remove` | RemoveMapFloor | Remove floor |
| `/map/reset` | ResetMap | Clear addresses |
| `/territory/reset` | ResetTerritory | Clear territory |
| `/territory/link` | TerritoryQuicklink | Smart assign |
| `/map/add` | NewMap | Create map |
| `/map/territory/update` | MapTerritoryUpdate | Move map |
| `/options/update` | OptionUpdate | Update options |

## 🔧 Common Patterns

### Handler Structure

```go
func HandleSomething(e *core.RequestEvent, app *pocketbase.PocketBase) error {
    // 1. Get request data
    requestInfo, _ := e.RequestInfo()
    data := requestInfo.Body
    
    // 2. Validate & fetch
    field := data["field"].(string)
    record, err := fetchHelper(app, field)
    if err != nil {
        sentry.CaptureException(err)
        return apis.NewNotFoundError("Not found", nil)
    }
    
    // 3. Process
    // Business logic here
    
    // 4. Return
    return e.String(http.StatusOK, "Success")
}
```

### Aggregate Calculation

```go
// Use CTE for countable options
WITH countable_options AS (
    SELECT id FROM options WHERE is_countable = TRUE AND congregation = {:cong}
)
SELECT 
    COALESCE(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END), 0) as done,
    COALESCE(SUM(CASE WHEN status = 'not_done' THEN 1 ELSE 0 END), 0) as not_done
FROM addresses a
WHERE map = {:mapId}
  AND EXISTS (
      SELECT 1 FROM countable_options co
      JOIN json_each(a.type) jt ON jt.value = co.id
  )
```

### Background Job Pattern

```go
func processJob(app *pocketbase.PocketBase) error {
    // LaunchDarkly flag checked in scheduler
    // Fetch data
    records, err := app.FindRecordsByFilter(...)
    if err != nil {
        sentry.CaptureException(err)
        return err
    }
    
    // Process
    for _, record := range records {
        // Logic
    }
    
    return nil
}
```

### Haversine Distance

```go
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
    const R = 6371000 // Earth radius in meters
    φ1 := lat1 * math.Pi / 180
    φ2 := lat2 * math.Pi / 180
    Δφ := (lat2 - lat1) * math.Pi / 180
    Δλ := (lon2 - lon1) * math.Pi / 180
    
    a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
         math.Cos(φ1)*math.Cos(φ2)*
         math.Sin(Δλ/2)*math.Sin(Δλ/2)
    c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
    
    return R * c // meters
}
```

## 🎯 Business Rules

### Address Status Logic

- **done** - Work completed
- **not_done** - Not yet contacted
- **not_home** - Not home, try again (max_tries from congregation)
- **dnc** - Do not call
- **invalid** - Invalid address

### Aggregate Calculation

- Only count `is_countable = true` types
- Progress = (done + not_home_max_tries) / total × 100
- Zero values handled with `COALESCE`

### Assignment Expiry

- Set by `expiry_hours` in congregation
- Cleaned up every 5 minutes
- Used for workload balancing

## 🔍 Troubleshooting

**Aggregates not updating?**
- Check `is_countable` flag on options
- Verify cron job enabled: `enable-territory-aggregations`
- Check for errors in Sentry

**Quicklink not working?**
- Verify maps have coordinates
- Check 50m threshold
- Ensure maps not 100% complete

**Reports not sending?**
- Check MailerSend API key
- Verify administrator roles exist
- Check LaunchDarkly flag: `enable-monthly-report`

## 📚 Files Reference

**Handlers:** `internal/handlers/`
- `common.go` - Shared query helpers
- `update_aggregates.go` - Stats processing
- `get_quicklink.go` - Smart assignment
- `add_new_map.go` - Map creation

**Jobs:** `internal/jobs/`
- `job_scheduler.go` - Cron config
- `process_territory_aggregates.go` - Territory stats
- `generate_report.go` - Excel reports

**Migrations:** `migrations/`
- `1760705281_collections_snapshot.go` - Schema

## 🌐 Environment Variables

```bash
MAILERSEND_API_KEY=        # Email service
LAUNCHDARKLY_SDK_KEY=      # Feature flags
LAUNCHDARKLY_CONTEXT_KEY=  # LD environment
SENTRY_DSN=                # Error tracking
PB_APP_URL=                # Frontend URL
```

## 📖 Resources

- PocketBase API: Standard CRUD on `/api/collections/{collection}/records`
- Real-time: `/api/realtime` (automatic SSE subscriptions)
- Admin UI: `/_/` (when authenticated)
