# wnwn v0.1.0

Initial public release of `wnwn`, a GTD-focused terminal app built with Bubbletea v2.

## Highlights

- Inbox capture and processing flow, including guided Process Inbox mode.
- Full task lifecycle semantics: done/canceled state changes, explicit archive, permanent trash.
- Project workflows with sub-groups, task reordering, subgroup move/rename/delete, and project metadata editing.
- Views and query DSL (`field:value`, date operators, `has:field`, tags, ad-hoc query input).
- Weekly Review mode across stale projects, waiting-for, someday/maybe, and recent archives.
- URL-first workflow improvements:
  - CLI capture supports `--url`.
  - `o` opens selected task URL from list/project/view/weekly-review task rows.
  - `🔗` metadata marker appears on tasks with URLs.
- Config-driven customization:
  - key remapping and per-scope action disable
  - startup view, top tab order, pinned saved views in tabs
  - saved view definitions and archive inclusion behavior
  - undo grace window (`enabled`, `seconds`, `undo_key`)
- Data portability:
  - SQLite runtime backend
  - Markdown import/export (`import-md` / `export-md`) with merge/replace and dry-run support

## Stability / Hardening

- Test suite passing: `go test ./...`
- Race detector passing: `go test -race ./...`
- Build verified: `go build -o wnwn ./cmd/wnwn/`
- CLI smoke flow verified on fresh data dir:
  - add task with URL/tag
  - export markdown
  - import dry-run

## Known Limits (v0.1.0)

- No fuzzy full-text search yet (exact DSL matching only).
- Tickler-file visualization not yet implemented.
- Process Inbox undo steering is still an improvement area.
