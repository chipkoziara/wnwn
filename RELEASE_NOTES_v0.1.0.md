# wnwn v0.1.0

Initial public release of `wnwn`, a GTD-inspired terminal app built with Bubbletea v2.

## Highlights

- Inbox capture and processing flow, including guided Process Inbox mode.
- Full task lifecycle semantics: done/canceled state changes, explicit archive, permanent trash.
- Project workflows with sub-groups, task reordering, subgroup move/rename/delete, and project metadata editing.
- Views tab with built-in saved views, ad-hoc query support, and archive-aware task review.
- Query DSL with:
  - field filters (`state:next-action`, `tag:@home`, `has:url`)
  - boolean logic (`AND`, `OR`, `NOT`)
  - grouping via parentheses
  - date comparisons (`<`, `<=`, `>`, `>=`)
  - inclusive date ranges (`today..7d`)
- Fuzzy search from the Views tab (`?`) across task text, notes, tags, URL, `waiting_on`, and source/project provenance.
- Weekly Review mode across stale projects, waiting-for, someday/maybe, and recent archives.
- URL-first workflow improvements:
  - CLI capture supports `--url`
  - `o` opens selected task URL from list/project/view/weekly-review task rows
  - `🔗` metadata marker appears on tasks with URLs
- Config-driven customization:
  - key remapping and per-scope action disable
  - startup view, top tab order, pinned saved views in tabs
  - saved view definitions and archive inclusion behavior
  - undo grace window (`enabled`, `seconds`, `undo_key`)
- Data portability:
  - SQLite runtime backend
  - Markdown import/export (`import-md` / `export-md`) with merge/replace and dry-run support
- CLI automation support:
  - `wnwn add`
  - `wnwn query`
  - `wnwn update`

## Release build / verification steps

Recommended release procedure for `v0.1.0`:

1. Ensure the working tree is clean and no local binary/build artifact is staged.
2. Run verification commands:
   - `go test ./...`
   - `go test -race ./...`
3. Build the release binary:
   - `go build -o wnwn ./cmd/wnwn/`
4. Run a fresh-data-dir smoke flow:
   - `WNWN_DATA_DIR=/tmp/wnwn-release-smoke ./wnwn add "release smoke test"`
   - `WNWN_DATA_DIR=/tmp/wnwn-release-smoke ./wnwn`
5. Verify Markdown portability:
   - `WNWN_DATA_DIR=/tmp/wnwn-release-smoke ./wnwn export-md --out /tmp/wnwn-export`
   - `./wnwn import-md --from /tmp/wnwn-export --dry-run`
6. Confirm `README.md` and `RELEASE_NOTES_v0.1.0.md` still match the release candidate.
7. Tag the release once the checks above pass.

### Current verification status

Verified in this release-prep session:
- `go test ./...` ✅
- `go test -race ./...` ✅
- `go build -o wnwn ./cmd/wnwn/` ✅
- fresh-data-dir CLI smoke flow (`wnwn add`) ✅
- Markdown export + import dry-run verification ✅

Manual interactive TUI smoke testing remains a good final pre-tag check.

### Assumptions

- `v0.1.0` is a source-build release only
- Go 1.25+ is available
- `mise` is optional convenience, not required
- Expected binary name: `wnwn`

## Known Limits (v0.1.0)

These are intentionally **non-blocking** for the first public release:

- Sync / multi-device durability is not implemented (local only).
- Recurring tasks are not implemented.
- Fuzzy search is currently a separate mode from the DSL rather than a combined query pipeline.
- Process Inbox undo steering is still an improvement area.
- Tickler-file visualization is not implemented.
- `v0.1.0` ships as a source-build release; prebuilt binaries are not yet part of the distribution story.
- Repo test fixtures can be used as reproducible demo data via `./testdata` and `./testdata/weekly-review`.

## Release boundary for v0.1.0

Before tagging:

- Public-facing docs (`README.md`, `RELEASE_NOTES_v0.1.0.md`) should be accurate
- Module path, repository URL, and install instructions should stay aligned
- Core release checks should pass:
  - `go test ./...`
  - `go test -race ./...`
  - `go build -o wnwn ./cmd/wnwn/`
  - fresh-data-dir CLI smoke flow
  - Markdown export/import dry-run verification
- No obvious privacy/secrets issues should be present in tracked release materials
- At least one final manual interactive TUI smoke test should be performed before tagging
