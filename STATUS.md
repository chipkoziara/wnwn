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
- `a`: add task to current sub-group
- `n`: add new sub-group
- `d`: mark task done
- `ctrl+j`/`ctrl+k`: reorder task within sub-group (cursor follows)
- `m`: move task to a different sub-group (picker)
- `esc`: back to project list

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
- Go 1.26 (via mise)
- charm.land/bubbletea/v2, charm.land/lipgloss/v2, charm.land/bubbles/v2
- github.com/oklog/ulid/v2
- gopkg.in/yaml.v3

## What's NOT Built Yet

Prioritized by impact:

### High Value (daily usability)
1. **Task detail/edit view** - Cannot view or edit task attributes (notes, deadline, tags, URL, delegated_to) from TUI. Only way to set these is via CLI flags on `gtd add`. This is the biggest usability gap.
2. **Process inbox mode** - The guided GTD decision tree for processing inbox items one at a time. All service primitives exist, just need the wizard UX flow (BRD section 3).

### Power Features
3. **Views / query DSL / filtering** - Text-based query language for filtered views across all lists (BRD section 2, "View Filtering"). Not started. Includes saved views in config.
4. **Search** - Fuzzy free-text search + structured query DSL (BRD section 2, "Search"). Not started.
5. **Weekly review mode** - Guided review flow checking projects have next actions, reviewing waiting-for items, someday/maybe cleanup (BRD section 3).
6. **Config file** - TOML at `~/.config/gtd/config.toml`. Keybindings, default tags, theme/colors, data directory, saved views, review reminders (BRD section 6). Not started.
7. **Tickler file** - Skeuomorphic 43-folder visualization as a skin on the agenda view (BRD section 2). Not started.

### Known Issues
- None currently open. All tests pass (31 total: 8 parser + 23 service).

## Architecture Notes

- **Bubbletea v2** (released 2026-02-24): Uses `tea.View` struct return from `View()` (not string), `tea.KeyPressMsg` (not `tea.KeyMsg`), import paths at `charm.land/*` (not `github.com/charmbracelet/*`).
- The TUI uses a `viewState` enum (`viewList`, `viewProjects`, `viewProjectDetail`) and a `mode` enum for input states (`modeNormal`, `modeAdding`, `modePickingProject`, etc.).
- `loadProjectDetail(filename)` resets cursor (for initial entry); `reloadProjectDetail()` preserves cursor (for mutations). This pattern was added to fix cursor-reset bugs.
- The parser handles two types of YAML fenced blocks: indented (2-space, for task metadata) and top-level (for sub-group metadata in project files).
- `@`-prefixed tags must be quoted in YAML (`"@computer"` not `@computer`). The writer handles this automatically.

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
