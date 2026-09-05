---
paths:
  - "internal/jobs/**"
---

# Jobs, email and the report

- Jobs are registered in `job_scheduler.go` via `app.Cron()` + `scheduler.MustAdd`, wrapped in `middleware.WithJobRecovery`, and gated by LaunchDarkly flags. An unset `LAUNCHDARKLY_SDK_KEY` means every flag defaults to enabled.
- Two mail paths: MailerSend for digests and reports (`MAILERSEND_API_KEY`), PocketBase SMTP for auth emails. Every MailerSend email renders through `renderEmail`: `templates/layout.html` wraps a content template that defines `"content"`, each data struct embeds `emailChrome`, and a plain-text part is derived from the HTML. Templates load by relative path (`templateDir`), so the binary runs from the repo root and tests point it at `../../templates`.
- `report_workbook.go` builds the Excel report. The Addresses sheet uses excelize's StreamWriter: create every sheet before writing it so tab order is fixed, and add nothing to it after `Flush`. The `analytics_*` views feed `summary_data.go`; keep their congregation filters pushable.
- `TestReportWorkbook_SeedData` and `TestBuildSummaryData_SeedData` pin the workbook layout and summary figures; change them deliberately when the design changes.
