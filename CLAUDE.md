# CLAUDE.md

Behavioral guidelines to reduce common LLM coding mistakes.

**Tradeoff:** These guidelines bias toward caution over speed. If the diff can be described in one sentence, skip the ceremony and just make the change.

**Domain in one line:** Go/PocketBase backend for Ministry Mapper (door-to-door ministry territory management) — a congregation has territories, a territory has maps, a map has addresses (household units). Publishers work maps via time-limited link tokens; admins manage everything through custom routes.

**Trust code over docs:** README.md and `.github/instructions/` contain stale claims (wrong PocketBase/Go versions, outdated aggregate and quicklink descriptions). Verify against source before repeating anything from them.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

- State assumptions explicitly. If uncertain, ask — a clarifying question before coding beats a rewrite after.
- If multiple interpretations exist, present them — don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.

**In this codebase:**
- PocketBase v0.39.x, modern API (`core.App`, `*core.RequestEvent`, `e.Next()` chains). Bootstrap order in `main.go` matters: `RegisterAuthHooks` → `RegisterRoutes` → `RegisterDomainHooks` → `ConfigureScheduler`. Routes live in `internal/setup/routes.go`; one handler file per endpoint in `internal/handlers/`; cron jobs/email/reports in `internal/jobs/`.
- IMPORTANT: `addresses`, `address_options`, and `messages` have superuser-only create/update/delete API rules **by design** — all mutations flow through custom routes (`/address/update`, `/address/add`, etc.). A hook on `OnRecordUpdateRequest("addresses")` will never fire (see the comment in `internal/handlers/auth_hooks.go` near the rules explanation). Before designing a mutation, check which custom route or handler owns that write path.
- Auth is two-world: admin JWT (`apis.RequireAuth()` via the `authRoute` helper + role checks with `AuthorizeByRole`) vs publisher **link-id header**. The link token is just an unexpired `assignments` record id, validated in SQL (`expiry_date > datetime('now')`). When both JWT and link-id are present, **link-id takes precedence and must be valid** (`AuthorizeMapAccess`, `internal/handlers/common.go`). Header naming trap: Go reads `link-id` (hyphen); PocketBase API-rule strings see `@request.headers.link_id` (underscore).
- List/view authorization is **post-query filtering**, not rule rewriting: `OnRecordsListRequest` hooks regex-extract IDs from the client filter, authorize, then prune `e.Records`/`e.Result.Items` via `filterListResults` (`scope_filters.go`). Realtime subscriptions are scoped in `OnRealtimeSubscribeRequest`. New list endpoints must follow this pattern.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features, abstractions, or "configurability" beyond what was asked.
- No defensive try/catch-everything bloat or handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

**In this codebase:**
- Query records with `app.FindRecordById` / `FindFirstRecordByFilter` with `{:param}` placeholders and `dbx.Params`. Raw SQL via `app.DB().NewQuery(...)` is normal and preferred for aggregates and auth checks — don't build an ORM layer. Use `FindCachedCollectionByNameOrId` in loops.
- New endpoints parse bodies with `e.BindBody(&struct{...})` + explicit validation (see `update_address.go`). Older handlers use unchecked `data["x"].(string)` assertions that panic on bad input — don't copy that style, and don't "fix" them incidentally either.
- Multi-record writes go inside `app.RunInTransaction(func(txApp core.App) error {...})`.
- Error returns are a two-way choice that controls Sentry noise: `apis.New*Error` for expected 4xx (not sent to Sentry); `newServerError(err)` — or `wrapTransactionError(err)` after transactions — for infra failures so Sentry captures the cause. Never return a bare `err` from a handler.

## 3. Surgical Changes

**Touch only what you must. Fix causes, not symptoms.**

- Don't "improve" adjacent code, comments, or formatting. Don't refactor things that aren't broken.
- Address the root cause — a narrow diff that suppresses an error or dodges the real problem is still a failure.
- Remove imports/variables YOUR changes orphaned; leave pre-existing dead code alone (mention it instead).

**In this codebase:**
- IMPORTANT: `SaveNoValidate` vs `Save` vs raw SQL is deliberate, not sloppiness. `SaveNoValidate` = trusted server-side write (fires hooks/realtime); `Save` = validation matters; raw SQL `DELETE` in `delete_territory.go` deliberately **suppresses** cascade realtime events, while `txApp.Delete` in `assignment_cleanup.go` deliberately **fires** them. Changing one changes what the frontend receives over realtime.
- Bulk address writes must use the store-flag protocol: set `app.Store().Set("bulk_reset:"+mapId, true)` before the transaction, `defer` the removal, then call `ProcessMapAggregates` once — otherwise the per-address aggregate hook fires N async recalcs (`aggregate_hook.go`, `reset_map.go`).
- `ProcessTerritoryAggregates` reads the `completed`/`total` keys of `maps.aggregates` JSON via `json_extract` — it does not scan addresses. Changing map aggregate output shape silently breaks territory progress.
- Every created address must get an `address_options` row with the congregation's default option — map create, code add, and floor add all maintain this invariant.
- Audit logs (`addresses_log`, `assignments_log`, `roles_log`): superuser actors map to `""` in `changed_by` via `authID()` because there's no users record — passing a superuser id fails the relation.

## 4. Goal-Driven Execution

**Define a check you can run. Loop until it passes. Show evidence, not assertions.**

- "Fix the bug" → write a test that reproduces it, then make it pass. "Refactor X" → tests pass before and after.
- When claiming success, show the command and its output — don't just say "done".

**In this codebase:**
- Two test tiers. Unit tests: plain `go test ./...` (handlers, middleware, jobs). Integration tests: behind the **`testdata` build tag** in `internal/setup/` — plain `go test` compiles nothing there; run `./scripts/test.sh` (needs the `sqlite3` CLI), which builds with `-tags testdata`, generates `test_pb_data/` via migrations + seed, then runs the tagged tests.
- Seed data IDs are stable and meant to be hard-coded in tests (`testcongalpha01`, `testmapalpha01a`, `admin@alpha.test` / `Test1234!` — see README's test-data section). Endpoint tests use PocketBase's `tests.ApiScenario` table style; `setupTestApp` re-registers routes+hooks per test app.
- CI on PRs to master/staging: `go mod tidy && go mod verify`, `go build`, `go vet`, unit tests (excluding `internal/setup`), plus a separate integration job running `scripts/test.sh`. `go vet` is the only linter — there is no golangci-lint.
- Run locally with `./scripts/start.sh` (exports `.env`, serves on :8090). Migrations automigrate only under `go run`; production never automigrates.

---

## 5. Project Conventions

**Migrations**
- Files in `migrations/`, registered with `m.Register(up, down)`, named `<unix-timestamp>_snake_description.go`. Write them **idempotently** (check-exists-then-skip, `return nil` on missing collections) — they must run cleanly on fresh test DBs.
- Env vars read inside migrations (`PB_ADMIN_EMAIL`, SMTP settings, OAuth keys, OTP/MFA flags) apply only on first run against a DB — changing them later does not re-apply.
- `1780000000_seed_test_data.go` is behind `//go:build testdata` and exists only in test builds.

**Cron jobs & email**
- Jobs are registered in `internal/jobs/job_scheduler.go` via `app.Cron()` + `scheduler.MustAdd`, each wrapped in `middleware.WithJobRecovery`, and gated by LaunchDarkly flags — an unset `LAUNCHDARKLY_SDK_KEY` means **all flags default to enabled**.
- Two separate mail paths: MailerSend for digests/reports (`MAILERSEND_API_KEY`), PocketBase SMTP for auth emails. Email templates in `templates/` are parsed by **relative path at runtime** — the binary must run from the repo root (Dockerfile copies `templates/` beside it).
- Async recalcs use PocketBase's `routine.FireAndForget`, not bare goroutines.

**Sequences & floors (subtle semantics)**
- Address `sequence` is per-map and shared across floors for the same code; new codes get `MAX+1`. Map `sequence` is per-territory; `/maps/sequence` requires every map id in the territory and renumbers 1..N.
- Adding a floor copies the codes of the current highest/lowest floor; going below floor 1 skips 0 and jumps to -1. Removing the last floor or deleting the last code is refused.
- Map/territory reset only flips `not_home`/`done` back to `not_done` — DNC and invalid are untouched.

**Commits**
- Conventional Commits in practice (`fix:`, `feat:`, `chore:`, ...). No AI co-author trailers. Keep messages simple.

---

**Maintaining this file:** treat it like code. If Claude makes a mistake this file should have prevented, add the rule; if a rule is always followed without being stated, delete it. Every line must earn its context cost.
