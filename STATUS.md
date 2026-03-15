# wnwn Project Status

Last updated: 2026-03-08 (session 25)

## What This Is

A GTD (Getting Things Done) TUI app built in Go with Bubbletea v2, Lipgloss v2, and Bubbles v2. The spec is in `BRD.md`. SQLite is the runtime data store, with Markdown import/export for portability.

## What's Built

### Data Layer (fully working, 112 tests passing)
- **Data model** (`internal/model/`): Task, TaskList, Project, SubGroup, SavedView types with full GTD attributes, including `modified_at` task metadata for recent-change tracking. Task states: empty, next-action, waiting-for, some-day/maybe, done, canceled. Project states: active, waiting-for, some-day/maybe, done, canceled (`StateActive` is project-only; `StateNextAction` is task-only).
- **SQLite persistence (canonical runtime backend)** (`internal/store/`): `Store` uses SQLite for all runtime reads/writes. Schema covers lists, list tasks, projects, sub-groups, project tasks, and archived tasks, with ordered-position columns for deterministic rendering.
- **Markdown interchange backend** (`internal/store/markdown.go`): Markdown read/write remains first-class for `import-md` / `export-md` workflows, but is no longer a runtime-selectable backend.
- **Store API extension**: Added `ListArchives()` to support full-dataset import/export between SQLite and Markdown.
- **Config loader** (`internal/config/`): reads optional `WNWN_CONFIG_FILE`, then XDG config path, then legacy data-dir config for archive behavior flags, default startup view, configurable undo grace settings (`ui.undo_grace_enabled`, `ui.undo_grace_seconds`, `ui.undo_key`), configurable top-tab order (`ui.tabs`, including pinned saved views via `view:Name`), configurable saved views (`views.use_defaults`, `[[views.saved]]`), and configurable keybindings for list/project/view-results actions.
- **Query package** (`internal/query/`): DSL parser + matcher for cross-list filtering. Supports `field:value`, `field:<value`, `field:>value`, `has:field`, bare `@tag` shorthand, and free text. Date fields support absolute (2026-04-01) and relative (today, tomorrow, 7d) tokens, including `modified` field queries (`modified:>today`, `has:modified`). 45 tests total across parse and match.
- **Markdown parser** (`internal/parser/`): Reads task lists and project files. Handles YAML frontmatter, fenced YAML metadata blocks, checkbox state, indented notes prose.
- **Markdown writer** (`internal/writer/`): Serializes back to spec-compliant Markdown. Auto-quotes `@`-prefixed tags for YAML safety.
- **ULID generation** (`internal/id/`): Task IDs using oklog/ulid.
- **Store** (`internal/store/`): Persistence abstraction over the SQLite runtime backend plus Markdown interchange adapter. Supports read/write for lists, projects, and archives, with slugified project filenames.
- **Service** (`internal/service/`): GTD business logic:
  - Inbox: add tasks with functional options (WithDeadline, WithTags, etc.)
  - State transitions: auto-sets waiting_since. done/canceled now stay in place by default.
  - Explicit archiving: `ArchiveTask` (list tasks) and `ArchiveProjectTask` (project tasks) move items into archive storage on demand.
  - Restore from archive: `RestoreArchivedTask` returns archived tasks to their recorded source (inbox, single-actions, or project), with inbox fallback when source is unavailable.
  - Trash semantics: trashed tasks are permanently deleted and are not archived.
  - List operations: move between inbox/single-actions, refile to projects
  - Project operations: create, add/rename/delete sub-groups, add tasks, reorder tasks within sub-groups, move tasks between sub-groups
	- Archiving: archived task records include source tracking and `archived_at` timestamp
	  - **Full task mutation**: `UpdateTask` (list tasks) and `UpdateProjectTask` (project tasks) replace all mutable fields and auto-set waiting_since when entering waiting-for
	  - **Cross-list aggregation**: `CollectAllTasks()` reads inbox, single-actions, and all project sub-groups, returning `[]ViewTask` with source provenance for each task
	  - **Archive aggregation**: `CollectArchiveTasks()` reads archived tasks and returns archive `[]ViewTask` entries for the dedicated Archives view
	  - **Weekly review aggregation**: `WeeklyReview(now)` groups projects missing next actions, aging waiting-for items, someday/maybe tasks, and recent archived tasks

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
- Config file path resolution:
  - `WNWN_CONFIG_FILE` (override)
  - `$XDG_CONFIG_HOME/wnwn/config.toml` (or `~/.config/wnwn/config.toml`)
  - Legacy fallback: `$WNWN_DATA_DIR/config.toml`
  - `archive.auto_archive_done` / `archive.auto_archive_canceled`
  - `ui.default_view` (`inbox`, `actions`, `projects`, `views`)
  - `keys.list`, `keys.project`, `keys.view_results` action remapping (including `cancel` and `trash`)
  - `keys.disable` for per-scope disabling of direct one-key actions (while keeping prefix workflows)

### TUI (`internal/tui/`)
Three-tab interface (Inbox, Actions, Projects) plus Process Inbox mode, with these features:

**All list views:**
- j/k, arrow keys, g/G navigation
- Tab and 1/2/3 to switch between views
- Status messages with auto-clear
- `e`: open task detail view
- `o`: open selected task URL in browser (if present)
- Tasks with URLs render a `🔗` metadata marker before tags

**Inbox view:**
- `a`: add task inline
- `P`: launch Process Inbox mode (guided GTD decision tree)
- `p`: refile to a project (opens project picker)
- `s`: state prefix (`m` someday, `d` done, `c` canceled, `w` waiting)
- `r`: route prefix (`a` single-actions, `p` project)
- `t`: time prefix (`d` deadline, `s` scheduled)
- `m`: quick someday/maybe (refiles from inbox)
- `w`: set waiting-for (refiles from inbox)
- `d`: mark done (stays in list)
- `c`: mark canceled (stays in list)
- `A`: archive selected task
- `x`: trash (permanent delete)
- Done/canceled/archive/trash/refile actions support undo during the configurable grace window (`ui.undo_key`)

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
- `s` prefix + (`m`/`d`/`c`/`w`): grouped state actions
- direct quick states: `m`/`w`/`d`/`c`
- `t` prefix + (`d`/`s`): deadline/scheduled quick edit
- `A`: archive selected task
- `x`: trash (permanent delete)
- Done/canceled/archive/trash/refile actions support undo during the configurable grace window (`ui.undo_key`)

**Projects list view:**
- Shows all projects with state, task count, deadline, next action preview
- `enter`: expand into project detail
- `a`: create new project

**Project detail view:**
- Flattened sub-group headings + tasks
- Recommended usage: default to one sub-group for small projects; add more only when phase grouping improves review clarity
- `e`: open task detail view for selected task
- `o`: open selected task URL in browser
- `a`: add task to current sub-group
- `n`: add new sub-group
- `R`: rename selected sub-group heading
- `X`: delete selected sub-group heading (empty sub-groups only)
- `s` prefix + (`m`/`d`/`c`/`w`): grouped state actions
- `t` prefix + (`d`/`s`): deadline/scheduled quick edit
- `d`: mark task done
- `c`: mark task canceled
- `A`: archive selected task
- `x`: trash selected task (permanent delete)
- Done/canceled/archive/trash/refile actions support undo during the configurable grace window (`ui.undo_key`)
- `E`: open project edit view (edit metadata)
- `ctrl+j`/`ctrl+k`: reorder task within sub-group (cursor follows)
- `m`: move task to a different sub-group (picker)
- `esc`: back to project list

**Project list view:**
- `E`: open project edit view for the selected project (in addition to existing `enter` to open detail)

**Views tab** (default hotkey `4` when using default tab order; otherwise use its configured tab number):
- Lists the 8 default saved views (Next Actions, Waiting For, Someday/Maybe, Overdue, Due This Week, Recently Created, Recently Modified, Archives)
- Saved views can now be configured in `config.toml` (`views.use_defaults` plus `[[views.saved]]` entries)
- `Recently Modified` includes archived tasks by default; other non-archive views read active tasks only unless a view is explicitly configured to include archives
- `enter`: open a view — collects all tasks and filters via the query DSL
- `W`: launch guided Weekly Review mode
- `/`: ad-hoc query input — type any DSL query, enter to run
- `j`/`k`/`g`/`G`: navigate the view list
- `esc`: return to Inbox

**Weekly Review mode** (from Views via `W`):
- Guided sections: Projects Missing Next Action, Aging Waiting For (7+ days), Someday/Maybe, Recently Archived (7 days)
- `h`/`l` or arrows: previous/next section; `j`/`k`/`g`/`G`: navigate items
- `enter`: open project detail (projects section) or task detail (task sections)
- `o`: open selected task URL in browser (task sections)
- `d`/`c`/`s`/`w`, `A`, `x`: mutate active tasks in waiting/someday sections; `R` refreshes review data
- Archived section is read-only

**View results** (after opening any view):
- Shows filtered tasks from all sources (inbox, single-actions, all projects) with muted source badge `[inbox]`, `[actions]`, `[project-name]`
- Header shows view name and query string
- `j`/`k`/`g`/`G`: navigate results
- `e`: open task detail (full edit; esc/save returns to view results and refreshes)
- `o`: open selected task URL in browser
- `d`/`c`/`s`/`w`: state changes applied directly with source-aware routing; view refreshes automatically
- `s` prefix + (`m`/`d`/`c`/`w`) for grouped state actions; direct quick states remain available (`m`/`d`/`c`/`w`)
- `t` prefix + (`d`/`s`) for quick deadline/scheduled edit
- `A`: archive selected task (source-aware); view refreshes automatically
- `U`: restore selected archived task to its original source (or inbox fallback); view refreshes automatically
- `x`: trash (permanent delete for both list and project tasks); view refreshes
- `R`: manual refresh (re-collect and re-filter)
- `esc`: back to view list
- Archived rows are protected from edit/state/archive/trash mutations, but can be restored with `U`
- Done/canceled/archive/trash/refile actions support undo during the configurable grace window (`ui.undo_key`)

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
- Read-only section shows: created date, modified date (if tracked), waiting_since (if set), ID, source (if archived)

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
  archive/                 # markdown export/import archive data
    archive.md             # unified archive export file
```

Tasks use Markdown checkboxes with indented fenced YAML metadata blocks. See `BRD.md` section 4 for full spec with examples.

### Dependencies
- Go 1.25 (via mise)
- charm.land/bubbletea/v2, charm.land/lipgloss/v2, charm.land/bubbles/v2
- github.com/oklog/ulid/v2
- gopkg.in/yaml.v3

### Project Metadata
- Added `LICENSE` (Apache-2.0) and `CONTRIBUTING.md` with DCO sign-off guidance (`git commit -s`).

## What's NOT Built Yet

Prioritized by impact:

### Bugs (fix now)

- None currently open.

### Recent Changes

- **Text input no longer swallows `u` while undo is armed** — Bubble's textinput default suggestion-accept binding also uses `u`, which conflicted with normal typing in inline editors while the app-level undo key is configured to `u`. The TUI now clears `textinput`'s suggestion-accept binding during model initialization so literal `u` remains typable in task/detail/query editors. App-level undo remains restricted to `modeNormal`, preserving undo behavior without hijacking text entry.
- **Scrollable list-style views** — Inbox/actions lists, project list/detail, saved views, view results, and weekly review now clamp to the visible terminal height instead of rendering past the bottom of the screen. A shared `scrollOffset` + `visibleRange()` pattern keeps the active cursor visible after navigation and window resizes, while preserving the existing cursor semantics (`m.cursor`, `m.projCursor`, `m.viewCursor`, weekly review cursors).
- **Date field editing is now reversible and validated** — Task/project date pickers now support `del`/`backspace` to clear a deadline/scheduled value, and toggling time off with `t` confirms back to a date-only value while preserving the selected day. Manual task detail date text entry now validates input and shows a status message for invalid formats (`YYYY-MM-DD` or `YYYY-MM-DD HH:MM`) instead of silently ignoring bad values.

### Deferred (future sessions)

5. **Undo / error correction in process inbox** — If you mistakenly mark something done or refile to the wrong location during processing, there's no way to fix it without quitting and manually finding the task. Three approaches were considered:
   - *Undo stack:* Most intuitive but complex; breaks the stateless service pattern, needs inverse operations.
   - *Staged changes with delay:* Adds confusion about when things are "really" committed.
   - *"Recently moved" view:* Pragmatic; a query against recently-modified tasks. Natural fit once Views/Query DSL is built.
   - **Recommendation:** Defer until Views/Query DSL exists, then add a "Recently Modified" saved view. In the meantime, users can quit processing, navigate to the task's new location, and edit via task detail view.

6. **Timezone handling** — Don't append timezones. The current naive-local-time approach is correct for a personal GTD app where you're always viewing your own tasks on your own machine. Adding timezone awareness (parsing, display, DST) adds significant complexity with minimal benefit. If syncing across timezones is ever needed, it should be a dedicated feature.

7. **Invalid datetime validation** — The date picker prevents most invalid dates. Text input for dates (in task detail view) could get validation, but it's low priority. The date picker is the primary input mechanism.

8. **Single Actions as a View (model simplification exploration)** — Explore treating Single Actions / Next Actions as a first-class saved view conceptually, rather than a distinct primary tab/list in UX language.
   - **Phase 1 (low-risk):** keep current storage model, but experiment with navigation and labeling so Single Actions behaves like a built-in view.
   - **Phase 2 (optional refactor):** evaluate collapsing list-specific assumptions into a more view-centric task model if Phase 1 validates the UX.
   - **Goal:** reduce mental overhead by making filters/views the primary interaction model without regressing inbox processing and refile workflows.

9. **Force-delete sub-group workflow** — Add an assisted delete flow for non-empty sub-groups in project detail.
   - Current behavior blocks deleting non-empty sub-groups and requires manual task moves.
   - Future flow: on delete of non-empty sub-group, prompt for destination sub-group and move all tasks before deletion.
   - Keep current safe default for empty sub-groups as-is.

10. **Task detail field ordering pass** — Revisit task edit field order and hierarchy (for example where notes should sit) to reduce friction during quick edits.

11. **Keybinding ergonomics audit** — Review default keybindings across list/project/view-results flows for mnemonic consistency and reduced cognitive switching.

12. **Project-grouped view rendering** — Add an optional view-results mode that groups tasks by project heading (instead of a flat mixed list), with quick jump/open behavior.

13. **Configurable data directory in config.toml** — Add an optional config field for default data path so users can persist storage location without relying only on `WNWN_DATA_DIR`.

14. **Go module path alignment** — Optionally align `go.mod` module path with the actual GitHub repository path for convention and discoverability.

### Power Features (prioritized)

15. **Views / query DSL / filtering** - ✅ Shipped (session 4). Config-backed saved views shipped in session 18.
16. **Config file** - ✅ Foundation shipped (session 8) and expanded in session 18: config now supports archive auto-archive toggles, startup default view, top-tab order, saved views, and configurable keybindings for core actions. Remaining config work: theme/colors, default tags, review reminders.
17. **Search** - Fuzzy free-text. The query DSL already handles `text:keyword`; fuzzy matching would be an enhancement on top.
18. **Tickler file** - Skeuomorphic 43-folder visualization as a skin on the agenda view (BRD section 2). Not started.

### Known Issues
- None currently open. All tests pass (112 total: 8 parser + 45 query + 44 service + 3 writer/parser roundtrip + 2 sqlite store + 8 config + 2 model).

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
- **Source-aware state changes** — `d`/`c`/`s`/`w` in view results call `UpdateProjectTaskState` (for project tasks) or `UpdateState` (for list tasks), while `x` now always performs permanent delete (`TrashProjectTask` / `TrashTask`). After any mutation the query is re-run to refresh the results in-place.
- **Task detail routing** — opening task detail from view results sets `detailFromView = viewViewResults`. On save (`saveDetailTask`), the function returns a `viewResultsLoadedMsg` (re-collect + re-filter) instead of `taskUpdatedMsg`, so the view refreshes automatically. On esc (discard), `updateTaskDetail` detects `viewViewResults` and re-runs the query.
- **Ad-hoc queries** — `/` from the view list activates `modeEditingField` within `viewViews`. The `updateViewList` handler forwards key events to the text input and on `enter` calls `runQuery("Ad-hoc", queryStr)`.
- **Default views remain built-in** — `model.DefaultViews()` still returns built-ins; persistence of user-defined saved views in config remains deferred.

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
