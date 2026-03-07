# wnwn Project Status

Last updated: 2026-03-07 (session 6)

## What This Is

A GTD (Getting Things Done) TUI app built in Go with Bubbletea v2, Lipgloss v2, and Bubbles v2. The spec is in `BRD.md`. SQLite is the runtime data store, with Markdown import/export for portability.

## What's Built

### Data Layer (fully working, 84 tests passing)
- **Data model** (`internal/model/`): Task, TaskList, Project, SubGroup, SavedView types with full GTD attributes. Task states: empty, next-action, waiting-for, some-day/maybe, done, canceled. Project states: active, waiting-for, some-day/maybe, done, canceled (`StateActive` is project-only; `StateNextAction` is task-only).
- **SQLite persistence (canonical runtime backend)** (`internal/store/`): `Store` uses SQLite for all runtime reads/writes. Schema covers lists, list tasks, projects, sub-groups, project tasks, archive lists, and archive tasks, with ordered-position columns for deterministic rendering.
- **Markdown interchange backend** (`internal/store/markdown.go`): Markdown read/write remains first-class for `import-md` / `export-md` workflows, but is no longer a runtime-selectable backend.
- **Store API extension**: Added `ListArchives()` to support full-dataset import/export between SQLite and Markdown.
- **Query package** (`internal/query/`): DSL parser + matcher for cross-list filtering. Supports `field:value`, `field:<value`, `field:>value`, `has:field`, bare `@tag` shorthand, and free text. Date fields support absolute (2026-04-01) and relative (today, tomorrow, 7d) tokens. 42 tests total across parse and match.
- **Markdown parser** (`internal/parser/`): Reads task lists and project files. Handles YAML frontmatter, fenced YAML metadata blocks, checkbox state, indented notes prose.
- **Markdown writer** (`internal/writer/`): Serializes back to spec-compliant Markdown. Auto-quotes `@`-prefixed tags for YAML safety.
- **ULID generation** (`internal/id/`): Task IDs using oklog/ulid.
- **Store** (`internal/store/`): Persistence abstraction over the SQLite runtime backend plus Markdown interchange adapter. Supports read/write for lists, projects, and archives, with slugified project filenames.
- **Service** (`internal/service/`): GTD business logic:
  - Inbox: add tasks with functional options (WithDeadline, WithTags, etc.)
  - State transitions: auto-sets waiting_since. done/canceled now stay in place by default.
  - Explicit archiving: `ArchiveTask` (list tasks) and `ArchiveProjectTask` (project tasks) move items to monthly archives on demand.
  - List operations: move between inbox/single-actions, refile to projects
  - Project operations: create, add sub-groups, add tasks, reorder tasks within sub-groups, move tasks between sub-groups
	- Archiving: monthly archive files with source tracking
	  - **Full task mutation**: `UpdateTask` (list tasks) and `UpdateProjectTask` (project tasks) replace all mutable fields and auto-set waiting_since when entering waiting-for
	  - **Cross-list aggregation**: `CollectAllTasks()` reads inbox, single-actions, and all project sub-groups, returning `[]ViewTask` with source provenance for each task

### CLI (`cmd/wnwn/main.go`)
- `wnwn` (no args): launches TUI
- `wnwn add "task" [--deadline DATE] [--scheduled DATE] [--tag TAG]... [--url URL] [--notes TEXT]`: quick capture to inbox
- `wnwn export-md --out DIR`: exports SQLite data to Markdown files
- `wnwn import-md --from DIR [--mode merge|replace] [--dry-run]`: imports Markdown files into SQLite with explicit safety modes
  - `merge` (default): non-destructive import by ID/filename; adds only missing list tasks/projects/archive tasks
  - `replace`: resets destination SQLite store then imports full dataset from Markdown source
  - `--dry-run`: computes and prints import plan/stats without writing data
- `wnwn help`: usage info
- Data dir configurable via `WNWN_DATA_DIR` env var (default: `~/.local/share/wnwn`)

### TUI (`internal/tui/`)
Three-tab interface (Inbox, Actions, Projects) plus Process Inbox mode, with these features:

**All list views:**
- j/k, arrow keys, g/G navigation
- Tab and 1/2/3 to switch between views
- Status messages with auto-clear
- `enter`: open task detail view

**Inbox view:**
- `a`: add task inline
- `P`: launch Process Inbox mode (guided GTD decision tree)
- `r`: refile to single-actions as next-action
- `p`: refile to a project (opens project picker)
- `s`: set someday/maybe (refiles from inbox)
- `w`: set waiting-for (refiles from inbox)
- `d`: mark done (stays in list)
- `A`: archive selected task
- `x`: trash (permanent delete)

**Process Inbox mode** (`P` from inbox):
- Guided GTD decision tree, walks through inbox items FIFO one at a time
- Tab bar shows "Processing Inbox (N of M)" progress
- Step 1 — Actionable?: `y` yes, `n` no, `s` skip, `q`/`esc` quit
- Step 2a — Not Actionable: `t` trash only (no someday here — someday/maybe is for deferred actionable tasks, not true non-actionables)
- Step 2b/3 — Enrich (hub): `t` edit text, `g` add tags (tab to confirm each), `d` deadline (calendar), `c` schedule (calendar), `n` notes; `enter` continue to route, `esc` back
  - Renders a field list with inline input when editing — same UX pattern as task detail view
  - Tags entered one-at-a-time via text input + `tab` to confirm; `enter` finishes; designed for future autocomplete
  - Deadline/schedule reuse the full calendar date picker; date-only selection (no time toggled) stores without time component
  - All edits mutate a working copy (`processTask`); only persisted when the final action fires
- Step 4 — Route: `d` done (<2 min), `w` waiting-for (→ delegated_to input), `s` someday/maybe, `r` single actions, `p` pick project, `n` new project (→ title input)
- Step 5 — Complete: summary stats (trashed/someday/done/waiting/single-actions/to-projects/skipped); any key returns to inbox

**Single Actions view:**
- `p`: refile to a project
- `s`/`w`/`d`/`x`: state changes (same as inbox but updates in-place)
- `A`: archive selected task

**Projects list view:**
- Shows all projects with state, task count, deadline, next action preview
- `enter`: expand into project detail
- `a`: create new project

**Project detail view:**
- Flattened sub-group headings + tasks
- `enter`: open task detail view for selected task
- `a`: add task to current sub-group
- `n`: add new sub-group
- `d`: mark task done
- `A`: archive selected task
- `E`: open project edit view (edit metadata)
- `ctrl+j`/`ctrl+k`: reorder task within sub-group (cursor follows)
- `m`: move task to a different sub-group (picker)
- `esc`: back to project list

**Project list view:**
- `E`: open project edit view for the selected project (in addition to existing `enter` to open detail)

**Views tab** (`4` or `V` from anywhere, or tab from Projects):
- Lists the 5 default saved views (Next Actions, Waiting For, Someday/Maybe, Overdue, Due This Week)
- `enter`: open a view — collects all tasks and filters via the query DSL
- `/`: ad-hoc query input — type any DSL query, enter to run
- `j`/`k`/`g`/`G`: navigate the view list
- `esc`: return to Inbox

**View results** (after opening any view):
- Shows filtered tasks from all sources (inbox, single-actions, all projects) with muted source badge `[inbox]`, `[actions]`, `[project-name]`
- Header shows view name and query string
- `j`/`k`/`g`/`G`: navigate results
- `enter`: open task detail (full edit; esc/save returns to view results and refreshes)
- `d`/`s`/`w`: state changes applied directly with source-aware routing; view refreshes automatically
- `A`: archive selected task (source-aware); view refreshes automatically
- `x`: trash (list tasks) or cancel (project tasks); view refreshes
- `R`: manual refresh (re-collect and re-filter)
- `esc`: back to view list

**Project edit view** (`viewProjectEdit`, opened with `E` from project list or detail):
- Navigable field list: title, state, tags, deadline, URL, definition of done
- `j`/`k`: navigate between fields
- `e` or `enter`: edit selected field
  - **State**: cycles through project-relevant states (empty → next-action → someday → done → canceled)
  - **Deadline**: opens the calendar date picker
  - **All other fields**: inline text input; `enter` confirms, `esc` cancels
  - **Tags**: comma-separated in the text input
- `s`: save all changes (renames file on disk if title changed) and return to previous view
- `esc`: discard changes and return to previous view
- Read-only section shows: ID, original filename
- Rename: changing the title automatically renames the `.md` file on disk (via `store.RenameProject`)

**Task detail view** (`viewTaskDetail`):
- Opens from any list or project detail view with `enter`
- Shows all task fields in a navigable list: task text, state, tags, deadline, scheduled, URL, delegated_to, notes
- `j`/`k`: navigate between fields
- `e` or `enter`: edit the selected field
  - **State field**: cycles through all states (empty → next-action → waiting-for → some-day/maybe → done → canceled); `space` also cycles
  - **Deadline / Scheduled fields**: opens the calendar date picker (see below)
  - **All other fields**: opens inline text input; `enter` confirms, `esc` cancels
- `s`: save all changes and return to previous view
- `esc`: discard changes and return to previous view
- Read-only section shows: created date, waiting_since (if set), ID, source (if archived)

**Date picker** (`internal/tui/datepicker/`):
- Custom-built calendar component (no compatible third-party library exists for Bubbletea v2)
- 7-column monthly calendar grid with day-of-week headers
- Navigation: arrow keys / hjkl move one day; j/k jump a full week; `<`/`>` (also `,`/`.` or `[`/`]`) change month
- Visual feedback: selected day highlighted in purple; today accented in bold purple
- `t`: toggle optional time input row (HH and MM fields, validated 0–23 / 0–59; tab moves between them)
- `enter`: confirm selection; `esc`: cancel (preserves existing value)
- Pre-populates with existing date when editing a field that already has a value

### File Format
```
~/.local/share/wnwn/
  in.md                    # inbox
  single-actions.md        # standalone next actions
  projects/                # one .md file per project
    launch-website.md
  archive/                 # monthly archives
    2026-03.md
```

Tasks use Markdown checkboxes with indented fenced YAML metadata blocks. See `BRD.md` section 4 for full spec with examples.

### Dependencies
- Go 1.25 (via mise)
- charm.land/bubbletea/v2, charm.land/lipgloss/v2, charm.land/bubbles/v2
- github.com/oklog/ulid/v2
- gopkg.in/yaml.v3

## What's NOT Built Yet

Prioritized by impact:

### Bugs (fix now)

- None currently open.

### Deferred (future sessions)

5. **Undo / error correction in process inbox** — If you mistakenly mark something done or refile to the wrong location during processing, there's no way to fix it without quitting and manually finding the task. Three approaches were considered:
   - *Undo stack:* Most intuitive but complex; breaks the stateless service pattern, needs inverse operations.
   - *Staged changes with delay:* Adds confusion about when things are "really" committed.
   - *"Recently moved" view:* Pragmatic; a query against recently-modified tasks. Natural fit once Views/Query DSL is built.
   - **Recommendation:** Defer until Views/Query DSL exists, then add a "Recently Modified" saved view. In the meantime, users can quit processing, navigate to the task's new location, and edit via task detail view.

6. **Timezone handling** — Don't append timezones. The current naive-local-time approach is correct for a personal GTD app where you're always viewing your own tasks on your own machine. Adding timezone awareness (parsing, display, DST) adds significant complexity with minimal benefit. If syncing across timezones is ever needed, it should be a dedicated feature.

7. **Invalid datetime validation** — The date picker prevents most invalid dates. Text input for dates (in task detail view) could get validation, but it's low priority. The date picker is the primary input mechanism.

### Power Features (prioritized)

9. **Views / query DSL / filtering** - ✅ Shipped (session 4). Saved view persistence in config.toml still deferred (see item 12).
10. **"Recently Modified" default view** — Quick win now that Views are shipped. Add a `created:>today` (or similar) built-in view to `model.DefaultViews()` to address the "accidental refile" feedback in ENHANCEMENTS.md.
11. **Weekly review mode** - Guided review flow: projects with no next actions, aging waiting-for items, someday/maybe cleanup (BRD section 3). All data layer primitives now exist via `CollectAllTasks`. Recommended next major feature.
12. **Config file** - TOML at `~/.config/wnwn/config.toml`. Keybindings, default tags, theme/colors, data directory, saved views, review reminders (BRD section 6). Not started.
13. **Search** - Fuzzy free-text. The query DSL already handles `text:keyword`; fuzzy matching would be an enhancement on top.
14. **Tickler file** - Skeuomorphic 43-folder visualization as a skin on the agenda view (BRD section 2). Not started.

### Known Issues
- None currently open. All tests pass (84 total: 8 parser + 42 query + 29 service + 3 writer/parser roundtrip + 2 sqlite store).

---

## Shipped: Process Inbox Mode

Built and shipped in session 2. Full design notes in the commit message and Architecture Notes section above. See the TUI section above for keybinding reference.

## Shipped: Project Editing

Built and shipped in session 3. Key design decisions:

- **`model.Project` extended** with `URL string` and `DefinitionOfDone string` fields (both `omitempty` in YAML).
- **`store.RenameProject(oldFilename, proj)`** — writes new file (slug derived from new title), deletes old if name changed. Handles the no-rename case (same slug) by just rewriting content.
- **`service.UpdateProject(oldFilename, updated)`** — reads current project from disk (preserving sub-groups and ID), applies mutable fields from the working copy, calls `RenameProject`. Returns the (possibly new) filename to the caller.
- **`projEditField` enum + `projEditFieldOrder`** — mirrors the `detailField`/`detailFieldOrder` pattern from task editing. Navigation and rendering both driven from a single slice.
- **`viewProjectEdit`** — a new view state. Opened with `E` from either the project list or project detail view. Holds `projEditProject model.Project` and `projEditFilename string` as the working copy. Save (`s`) calls `UpdateProject`; `esc` discards.
- **Shared mode reuse** — `modeEditingField` writes to `projEditProject` when `m.view == viewProjectEdit` (via `applyProjEditFieldEdit`). `modePickingDate` writes to `projEditProject.Deadline` when in project edit view. Pattern matches how process inbox shares these modes.
- **`projectEditLoadedMsg`** — carries the loaded project + originating view. Follows the same message-passing pattern as `projectDetailMsg`.
- **`projectUpdatedMsg`** — carries the new filename. On receipt, if the originating view was project detail, reloads that view with the (possibly renamed) file; otherwise returns to project list.


## Shipped: Views / Query DSL / Filtering

Built and shipped in session 4. Key design decisions:

- **`internal/query` package** — standalone DSL parser and matcher with no I/O dependency. `Parse(input, now)` returns `[]Clause`; `MatchAll(clauses, task, source)` evaluates them. The `source` parameter is the provenance string (`"in"`, `"single-actions"`, `"projects/<filename>"`) used by the `project:` clause.
- **Relative date tokens** — `today`, `tomorrow`, `Nd` (e.g. `7d`) are resolved to midnight local time at query execution time. This makes the "Overdue" and "Due This Week" default views work correctly every day without user intervention.
- **`CollectAllTasks()`** — stateless read of all active sources. Returns `[]ViewTask` with `Source`, `SgIdx`, `Filename`, `ListType`, `IsProject` so callers can route mutations back to the correct service method.
- **`viewViews` / `viewViewResults`** — two new view states added to the existing `viewState` enum. `viewViews` renders the saved view list; `viewViewResults` renders filtered results. Both share the existing tab bar (extended to 4 tabs) and key routing infrastructure.
- **Source-aware state changes** — `d`/`s`/`w`/`x` in view results call `UpdateProjectTaskState` (for project tasks) or `UpdateState`/`TrashTask` (for list tasks) based on `ViewTask.IsProject`. After any mutation the query is re-run to refresh the results in-place.
- **Task detail routing** — opening task detail from view results sets `detailFromView = viewViewResults`. On save (`saveDetailTask`), the function returns a `viewResultsLoadedMsg` (re-collect + re-filter) instead of `taskUpdatedMsg`, so the view refreshes automatically. On esc (discard), `updateTaskDetail` detects `viewViewResults` and re-runs the query.
- **Ad-hoc queries** — `/` from the view list activates `modeEditingField` within `viewViews`. The `updateViewList` handler forwards key events to the text input and on `enter` calls `runQuery("Ad-hoc", queryStr)`.
- **Hardcoded default views** — `model.DefaultViews()` returns the 5 built-in views. No config file yet; user-defined saved views deferred to the config file session.

## Architecture Notes

- **Bubbletea v2** (released 2026-02-24): Uses `tea.View` struct return from `View()` (not string), `tea.KeyPressMsg` (not `tea.KeyMsg`), import paths at `charm.land/*` (not `github.com/charmbracelet/*`).
- The TUI uses a `viewState` enum (`viewList`, `viewProjects`, `viewProjectDetail`, `viewTaskDetail`, `viewProcessInbox`) and a `mode` enum for input states (`modeNormal`, `modeAdding`, `modePickingProject`, `modeEditingField`, `modePickingDate`, etc.).
- `loadProjectDetail(filename)` resets cursor (for initial entry); `reloadProjectDetail()` preserves cursor (for mutations). This pattern was added to fix cursor-reset bugs.
- The parser handles two types of YAML fenced blocks: indented (2-space, for task metadata) and top-level (for sub-group metadata in project files).
- `@`-prefixed tags must be quoted in YAML (`"@computer"` not `@computer`). The writer handles this automatically.
- **Task detail working copy**: when the user opens task detail, a copy of the task (`detailTask`) is held in the model and mutated locally. Changes are only persisted when the user presses `s` (save), which calls `UpdateTask` or `UpdateProjectTask`. `esc` discards the copy.
- **Process inbox working copy**: same pattern. `processTask` is a copy of the current inbox item, mutated during enrichment. The final action (trash/refile/done/someday) calls `svc.UpdateTask` to flush enrichment to disk first, then the refile/state-change. This two-step approach ensures enrichment survives the move.
- **Process inbox shared mode reuse**: `modeEditingField`, `modePickingDate`, and `modePickingProject` are shared with the normal/task-detail flows. Each handler checks `m.view == viewProcessInbox` and routes results back to the process inbox working copy (`processTask`) instead of `detailTask`. The project picker on `enter` calls `processMoveToProject` instead of `moveToProject`; on `esc` it returns to `stepRoute` instead of reloading the list.
- **`projectCreatedMsg` carries filename**: `projectCreatedMsg` was extended with a `filename string` field (derived from `store.Slugify(title) + ".md"`). The `projectCreatedMsg` handler checks `m.view == viewProcessInbox` and calls `processRefileToNewProject(filename)` instead of showing a status message and reloading the project list.
- **Date picker composition**: `internal/tui/datepicker` is a standalone sub-model following the Bubbletea component pattern. Parent forwards all `tea.Msg` to it while `modePickingDate` is active; result is read back via `Result()` after each `Update` cycle. `Result()` returns `(time.Time, hasTime bool, confirmed bool, cancelled bool)` — `hasTime` is true only when the user explicitly toggled the time input. When false, the stored value is date-only (midnight), and `formatOptionalTime()` omits the time component in display.
- **`formatOptionalTime()`**: helper in `app.go` that formats a `*time.Time` as `"2006-01-02"` when hour/minute are both 0, and `"2006-01-02 15:04"` otherwise. Used consistently across all date display sites.
- **No third-party date picker**: `bubble-datepicker` and similar libraries use the old `github.com/charmbracelet/bubbletea` v1 import path and are incompatible with this project's v2 dependency. The date picker is custom-built.

## How to Run

```bash
# Activate Go via mise
eval "$(mise activate bash)"

# Run tests
go test ./...

# Build
go build -o wnwn ./cmd/wnwn/

# Quick capture
WNWN_DATA_DIR=/tmp/wnwn-test ./wnwn add "Buy milk"

# Launch TUI
WNWN_DATA_DIR=/tmp/wnwn-test ./wnwn
```
