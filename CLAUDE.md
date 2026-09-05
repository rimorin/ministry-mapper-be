# CLAUDE.md

Go/PocketBase backend for Ministry Mapper (door-to-door ministry territory management): a congregation has territories, a territory has maps, a map has addresses (household units). Publishers work maps through time-limited link tokens; admins manage everything through custom routes.

Trust code over docs: `readme.md`'s aggregate, quicklink and scheduled-job sections are verified against source; treat the rest as unverified.

## Working principles
From Andrej Karpathy's guidelines (github.com/multica-ai/andrej-karpathy-skills), reproduced verbatim.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

### 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

### 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

### 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

### 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.
## Layout and bootstrap
- PocketBase v0.40.x (`core.App`, `*core.RequestEvent`, `e.Next()` chains). Bootstrap order in `main.go` matters: `RegisterAuthHooks` → `RegisterRoutes` → `RegisterDomainHooks` → `ConfigureScheduler`.
- Routes: `internal/setup/routes.go`. One handler file per endpoint in `internal/handlers/`. Cron jobs, email and the Excel report in `internal/jobs/`.
- `serve` applies pending migrations at startup in every environment. Only `Automigrate`, which generates migration files from admin-UI edits, is limited to `go run`.

## Auth
- Two worlds: admin JWT (`apis.RequireAuth()` via `authRoute` plus `AuthorizeByRole`) and the publisher `link-id` header. A link token is an unexpired `assignments` record id, validated in SQL (`expiry_date > datetime('now')`).
- When both JWT and link-id are present, link-id takes precedence and must be valid (`AuthorizeMapAccess`, `internal/handlers/common.go`).
- Header naming: Go reads `link-id` (hyphen); PocketBase API-rule strings see `@request.headers.link_id` (underscore).
- List/view authorization is post-query filtering: `OnRecordsListRequest` hooks extract ids from the client filter, authorize, then prune results via `filterListResults` (`scope_filters.go`). New list endpoints follow this pattern.

## Writes
- `addresses`, `address_options` and `messages` have superuser-only create/update/delete API rules by design; every mutation goes through a custom route (`/address/update`, `/address/add`, ...). A hook on `OnRecordUpdateRequest("addresses")` never fires. Find the route that owns a write path before designing a mutation.
- `SaveNoValidate` is a trusted server-side write that fires hooks and realtime; `Save` is for when validation matters. The raw SQL `DELETE` in `delete_territory.go` deliberately suppresses cascade realtime events; `txApp.Delete` in `assignment_cleanup.go` deliberately fires them. Changing one changes what the frontend receives.
- Multi-record writes go inside `app.RunInTransaction`.
- Bulk address writes: set `app.Store().Set("bulk_reset:"+mapId, true)` before the transaction, `defer` its removal, then call `ProcessMapAggregates` once. Otherwise the per-address hook fires N async recalcs (`aggregate_hook.go`, `reset_map.go`).
- `ProcessTerritoryAggregates` reads `completed`/`total` from `maps.aggregates` JSON. Changing the map aggregate shape silently breaks territory progress.
- Every created address gets an `address_options` row with the congregation's default option; map create, code add and floor add all maintain this.
- Audit logs (`addresses_log`, `assignments_log`, `roles_log`): superuser actors map to `""` in `changed_by` via `authID()`, because there is no users record to relate to.
- Error returns control Sentry noise: `apis.New*Error` for expected 4xx; `newServerError(err)` or `wrapTransactionError(err)` for infrastructure failures. Never return a bare `err` from a handler.
- New handlers parse bodies with `e.BindBody(&struct{...})` plus explicit validation (see `update_address.go`). Older handlers use unchecked `data["x"].(string)` assertions; don't copy that style and don't fix them incidentally.
- Query with `app.FindRecordById` / `FindFirstRecordByFilter` and `{:param}` placeholders; raw SQL via `app.DB().NewQuery` is the norm for aggregates and auth checks. Use `FindCachedCollectionByNameOrId` in loops.

## Realtime
- IMPORTANT: subscription strings in `OnRealtimeSubscribeRequest` are SSE channel names. Validate and drop, never rewrite. A rewritten string publishes to a channel nobody listens on and every event is silently lost. Scope is enforced by refusing over-broad filters (`filterEscapesMapScope`); see `internal/setup/realtime_subscribe_test.go`.
- Async recalcs use `routine.FireAndForget`, not bare goroutines.

## Domain semantics
- Address `sequence` is per map and shared across floors for the same code; new codes get `MAX+1`. Map `sequence` is per territory; `/maps/sequence` requires every map id in the territory and renumbers 1..N.
- Adding a floor copies the codes of the current highest or lowest floor; going below floor 1 skips 0 and jumps to -1. Removing the last floor or deleting the last code is refused.
- Map and territory reset flip only `not_home`/`done` back to `not_done`; DNC and invalid are untouched.

## Tests and CI
- Unit tests: `go test ./...`. Integration tests sit behind the `testdata` build tag in `internal/setup/` and `internal/jobs/`; run `./scripts/test.sh` (needs the `sqlite3` CLI). It builds with `-tags testdata`, generates `test_pb_data/` from migrations plus seed, runs the tagged tests and removes the DB afterwards.
- Seed ids are stable and meant to be hard-coded: `testcongalpha01`, `testmapalpha01a`, `admin@alpha.test` / `Test1234!`. Endpoint tests use `tests.ApiScenario`; `setupTestApp` re-registers routes and hooks per test app.
- CI on PRs to master/staging: `go mod tidy && go mod verify`, `go build`, `go vet`, unit tests, plus the integration job. `go vet` is the only linter; a PostToolUse hook runs `gofmt -l` and `go vet` after each Go edit.
- Run locally with `./scripts/start.sh` (exports `.env`, serves on :8090).

## Conventions
- Conventional Commits (`fix:`, `feat:`, `chore:`). No AI co-author trailers. Keep messages simple.
- The README is tracked as `readme.md`; on this case-insensitive filesystem `git add README.md` stages nothing.
- Migration and jobs conventions live in `.claude/rules/` and load when you touch `migrations/` or `internal/jobs/`.

Maintaining this file: treat it like code. If Claude makes a mistake this file should have prevented, add the rule; if a rule is always followed without being stated, delete it.
