# G-Tuddy Project Status

Last updated: 2026-03-01

## What This Is

A GTD (Getting Things Done) TUI app built in Go with Bubbletea v2, Lipgloss v2, and Bubbles v2. The spec is in `BRD.md`. Data is stored as Markdown files ("File Over App" philosophy).

## What's Built

### Data Layer (fully working, 31 tests passing)
- **Data model** (`internal/model/`): Task, TaskList, Project, SubGroup types with full GTD attributes (state, deadline, scheduled, tags, delegated_to, waiting_since, etc.)
- **Markdown parser** (`internal/parser/`): Reads task lists and project files. Handles YAML frontmatter, fenced YAML metadata blocks, checkbox state, indented notes prose.
- **Markdown writer** (`internal/writer/`): Serializes back to spec-compliant Markdown. Auto-quotes `@`-prefixed tags for YAML safety.
- **ULID generation** (`internal/id/`): Task IDs using oklog/ulid.
- **Store** (`internal/store/`): File system abstraction. Read/write for lists, projects, archives. Auto-init of data directory structure. Slugified project filenames.
- **Service** (`internal/service/`): GTD business logic:
  - Inbox: add tasks with functional options (WithDeadline, WithTags, etc.)
  - State transitions: auto-sets waiting_since, auto-archives done/canceled
  - List operations: move between inbox/single-actions, refile to projects
  - Project operations: create, add sub-groups, add tasks, reorder tasks within sub-groups, move tasks between sub-groups
  - Archiving: monthly archive files with source tracking
  - **Full task mutation**: `UpdateTask` (list tasks) and `UpdateProjectTask` (project tasks) replace all mutable fields, handle archive-on-done/canceled, auto-set waiting_since

### CLI (`cmd/gtd/main.go`)
- `gtd` (no args): launches TUI
- `gtd add "task" [--deadline DATE] [--scheduled DATE] [--tag TAG]... [--url URL] [--notes TEXT]`: quick capture to inbox
- `gtd help`: usage info
- Data dir configurable via `GTD_DATA_DIR` env var (default: `~/.local/share/gtd`)

### TUI (`internal/tui/`)
Three-tab interface (Inbox, Actions, Projects) with these features:

**All list views:**
- j/k, arrow keys, g/G navigation
- Tab and 1/2/3 to switch between views
- Status messages with auto-clear
- `enter`: open task detail view

**Inbox view:**
- `a`: add task inline
- `r`: refile to single-actions as next-action
- `p`: refile to a project (opens project picker)
- `s`: set someday/maybe (refiles from inbox)
- `w`: set waiting-for (refiles from inbox)
- `d`: mark done (archives)
- `x`: trash (permanent delete)

**Single Actions view:**
- `p`: refile to a project
- `s`/`w`/`d`/`x`: state changes (same as inbox but updates in-place)

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
- `ctrl+j`/`ctrl+k`: reorder task within sub-group (cursor follows)
- `m`: move task to a different sub-group (picker)
- `esc`: back to project list

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
~/.local/share/gtd/
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

### High Value (daily usability)
1. **Process inbox mode** - The guided GTD decision tree for processing inbox items one at a time. All service primitives exist, just need the wizard UX flow (BRD section 3).

### Power Features
2. **Views / query DSL / filtering** - Text-based query language for filtered views across all lists (BRD section 2, "View Filtering"). Not started. Includes saved views in config.
3. **Search** - Fuzzy free-text search + structured query DSL (BRD section 2, "Search"). Not started.
4. **Weekly review mode** - Guided review flow checking projects have next actions, reviewing waiting-for items, someday/maybe cleanup (BRD section 3).
5. **Config file** - TOML at `~/.config/gtd/config.toml`. Keybindings, default tags, theme/colors, data directory, saved views, review reminders (BRD section 6). Not started.
6. **Tickler file** - Skeuomorphic 43-folder visualization as a skin on the agenda view (BRD section 2). Not started.

### Known Issues
- None currently open. All tests pass (31 total: 8 parser + 23 service).

## Architecture Notes

- **Bubbletea v2** (released 2026-02-24): Uses `tea.View` struct return from `View()` (not string), `tea.KeyPressMsg` (not `tea.KeyMsg`), import paths at `charm.land/*` (not `github.com/charmbracelet/*`).
- The TUI uses a `viewState` enum (`viewList`, `viewProjects`, `viewProjectDetail`, `viewTaskDetail`) and a `mode` enum for input states (`modeNormal`, `modeAdding`, `modePickingProject`, `modeEditingField`, `modePickingDate`, etc.).
- `loadProjectDetail(filename)` resets cursor (for initial entry); `reloadProjectDetail()` preserves cursor (for mutations). This pattern was added to fix cursor-reset bugs.
- The parser handles two types of YAML fenced blocks: indented (2-space, for task metadata) and top-level (for sub-group metadata in project files).
- `@`-prefixed tags must be quoted in YAML (`"@computer"` not `@computer`). The writer handles this automatically.
- **Task detail working copy**: when the user opens task detail, a copy of the task (`detailTask`) is held in the model and mutated locally. Changes are only persisted when the user presses `s` (save), which calls `UpdateTask` or `UpdateProjectTask`. `esc` discards the copy.
- **Date picker composition**: `internal/tui/datepicker` is a standalone sub-model following the Bubbletea component pattern. Parent forwards all `tea.Msg` to it while `modePickingDate` is active; result is read back via `Result()` after each `Update` cycle.
- **No third-party date picker**: `bubble-datepicker` and similar libraries use the old `github.com/charmbracelet/bubbletea` v1 import path and are incompatible with this project's v2 dependency. The date picker is custom-built.

## How to Run

```bash
# Activate Go via mise
eval "$(mise activate bash)"

# Run tests
go test ./...

# Build
go build -o gtd ./cmd/gtd/

# Quick capture
GTD_DATA_DIR=/tmp/gtd-test ./gtd add "Buy milk"

# Launch TUI
GTD_DATA_DIR=/tmp/gtd-test ./gtd
```
