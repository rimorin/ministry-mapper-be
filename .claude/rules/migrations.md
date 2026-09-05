---
paths:
  - "migrations/**"
---

# Migrations

- Register with `m.Register(up, down)` in a file named `<unix-timestamp>_snake_description.go`.
- Write both directions idempotently: check-exists-then-skip, `return nil` on a missing collection. They must run cleanly on a fresh test DB.
- Env vars read inside migrations (`PB_ADMIN_EMAIL`, SMTP settings, OAuth keys, OTP/MFA flags) apply only on first run against a DB; changing them later does not re-apply.
- `1780000000_seed_test_data.go` is behind `//go:build testdata` and exists only in test builds.
- PocketBase's view-query parser accepts only plain identifiers or parenthesised expressions as columns, so write composite columns as `(a || '_' || b) AS id`. A `ROW_NUMBER() OVER()` id blocks SQLite from pushing filters into the view; give views a deterministic id instead.
- When a migration fails at boot, `scripts/test.sh` prints the server's `failed to apply migration ...` line before "Server did not become ready".
