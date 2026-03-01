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
1. **Process inbox mode** — READY TO BUILD. Detailed implementation plan below.
2. **Task editing UX enhancements** — Several small improvements to the task detail editing experience. To be scoped after process inbox mode ships.

### Power Features
3. **Views / query DSL / filtering** - Text-based query language for filtered views across all lists (BRD section 2, "View Filtering"). Not started. Includes saved views in config.
4. **Search** - Fuzzy free-text search + structured query DSL (BRD section 2, "Search"). Not started.
5. **Weekly review mode** - Guided review flow checking projects have next actions, reviewing waiting-for items, someday/maybe cleanup (BRD section 3).
6. **Config file** - TOML at `~/.config/gtd/config.toml`. Keybindings, default tags, theme/colors, data directory, saved views, review reminders (BRD section 6). Not started.
7. **Tickler file** - Skeuomorphic 43-folder visualization as a skin on the agenda view (BRD section 2). Not started.

### Known Issues
- None currently open. All tests pass (31 total: 8 parser + 23 service).

---

## Implementation Plan: Process Inbox Mode

A guided GTD decision tree for processing inbox items one at a time. All service primitives already exist — this is purely a TUI feature.

### Activation

- **`P`** (uppercase) from Inbox view in `modeNormal`
- Loads inbox, snapshots tasks into `processItems` (FIFO order = file order)
- If inbox is empty, shows status message "Inbox is empty" and stays in inbox view

### Flow Overview

```
                    +----------------+
          +--------|  ACTIONABLE?   |--------+
          | [y]    |  y/n/s/q       |  [n]   |
          v        +----------------+        v
   +-----------+                     +-----------------+
   |  ENRICH   |                     | NOT ACTIONABLE  |
   | t/g/d/c/n |                     |    t / m        |
   | [enter]-> |                     +----+-------+----+
   +-----+-----+                    [t]  |        | [m]
         |                         Trash |        v
         v                        +next  | +-----------+
   +-----------+                         | |  ENRICH   |
   |   ROUTE   |                         | | t/g/d/c/n |
   | d/w/r/p/n |                         | | [enter]-> |
   +-+--+--+---+                         | +-----+-----+
     |  |  |  |                          |       |
     |  |  |  | [n] New Project          | refile someday
     |  |  |  +-> title input            | + next item
     |  |  |      -> refile + next       |
     |  |  |                             |
     |  |  | [p] Pick Project            |
     |  |  +-> project picker            |
     |  |      -> refile + next          |
     |  |                                |
     |  | [w] Waiting For                |
     |  +-> delegated_to input           |
     |      -> refile + next             |
     |                                   |
     | [d] Done   -> archive + next      |
     | [r] Refile -> single-actions+next |
```

### Step-by-Step UX

#### Step 1: ACTIONABLE? (`stepActionable`)

```
+-- Process Inbox (3 of 12) -------------------+
|                                               |
|  Book flights to/from Hawaii                  |
|  Tags: travel, @computer                      |
|  Deadline: 2026-03-15                         |
|                                               |
|  Is it actionable?                            |
|                                               |
|  y yes  n no  s skip  q quit                  |
+-----------------------------------------------+
```

- Shows current task with all existing attributes
- `y` -> stepEnrich (with `processRoute = "actionable"`)
- `n` -> stepNotActionable
- `s` -> advance to next item (skip, no mutation)
- `q` or `esc` -> quit processing, return to inbox view

#### Step 2a: NOT ACTIONABLE (`stepNotActionable`)

```
|  Not actionable. What to do?                  |
|                                               |
|  t trash    m someday/maybe                   |
|  esc back                                     |
```

- `t` -> calls `svc.TrashTask()`, advances to next item
- `m` -> stepEnrich (with `processRoute = "someday"`)
- `esc` -> back to stepActionable

#### Step 2b/3: ENRICH (`stepEnrich`)

```
+-- Process Inbox (3 of 12) -------------------+
|                                               |
|  Book flights to/from Hawaii                  |
|                                               |
|  Tags:      travel, @computer                 |
|  Deadline:  2026-03-15                        |
|  Scheduled: -                                 |
|  Notes:     Check Southwest and United...     |
|                                               |
|  t edit text  g add tags                      |
|  d deadline   c schedule                      |
|  n notes                                      |
|  enter continue  esc back                     |
+-----------------------------------------------+
```

This is a **hub** — user can do zero or more edits, then press `enter` to proceed.

- `t` -> edit task text: opens inline text input (reuses `modeEditingField` + shared `m.input`), returns here after confirm/cancel
- `g` -> add tags: enters `stepEnrichTags` (see below)
- `d` -> set deadline: enters `modePickingDate` with existing date picker, returns here after confirm/cancel
- `c` -> set schedule: enters `modePickingDate`, returns here after confirm/cancel
- `n` -> edit notes: opens inline text input (single-line, same as task detail), returns here after confirm/cancel
- `enter` -> if `processRoute == "someday"`: persist enrichment + refile to single-actions as someday/maybe + advance. If `processRoute == "actionable"`: proceed to stepRoute
- `esc` -> back to previous step (stepActionable or stepNotActionable). Discards enrichment edits made in this visit.

**All edits mutate `processTask` (the working copy). Nothing is persisted until the final action.**

#### Step 3a: ENRICH TAGS (`stepEnrichTags`)

```
|  Tags: travel, @computer                     |
|                                               |
|  Add tag: @erran_                             |
|                                               |
|  tab add tag  enter done  esc cancel          |
```

- `tab` -> appends current input to `processTags`, clears input for next tag
- `enter` -> finishes tag entry, merges `processTags` into `processTask.Tags`, returns to stepEnrich
- `esc` -> discards tags added in this session, returns to stepEnrich

Note: no autocomplete for now. Future enhancement will add autocomplete here using existing tags from the data directory.

#### Step 4: ROUTE (`stepRoute`)

```
|  Route this task:                             |
|                                               |
|  d done (<2 min, did it)                      |
|  w waiting for (delegate)                     |
|  r single actions                             |
|  p add to project                             |
|  n new project                                |
|  esc back                                     |
```

- `d` -> calls `svc.UpdateState(ListIn, id, StateDone)` (auto-archives). Advances.
- `w` -> stepDelegatedTo
- `r` -> persist enrichment + calls `svc.MoveToList(ListIn, id, ListSingleActions, StateNextAction)`. Advances.
- `p` -> enters `modePickingProject` (reuses existing project picker). On completion, advances.
- `n` -> stepNewProject
- `esc` -> back to stepEnrich

#### Step 4a: DELEGATED TO (`stepDelegatedTo`)

```
|  Delegated to: Marvi_                         |
|                                               |
|  enter confirm  esc cancel                    |
```

- `enter` -> sets `processTask.DelegatedTo`, persist enrichment + calls `svc.MoveToList(ListIn, id, ListSingleActions, StateWaitingFor)` (service auto-sets waiting_since). Advances.
- `esc` -> back to stepRoute

#### Step 4b: NEW PROJECT (`stepNewProject`)

```
|  Project title: Launch webs_                  |
|                                               |
|  enter create & refile  esc cancel            |
```

- `enter` -> calls `svc.CreateProject(title, "Tasks")`, gets filename, persist enrichment + calls `svc.MoveToProject(ListIn, id, filename, 0, StateNextAction)`. Advances.
- `esc` -> back to stepRoute

#### Step 5: COMPLETE (`stepComplete`)

```
+-- Process Inbox ------------------------------+
|                                               |
|  Inbox processed!                             |
|  12 items reviewed                            |
|                                               |
|  Trashed:        2                            |
|  Someday/Maybe:  1                            |
|  Done:           3                            |
|  Waiting For:    1                            |
|  Single Actions: 3                            |
|  To Projects:    2                            |
|  Skipped:        0                            |
|                                               |
|  press any key to return                      |
+-----------------------------------------------+
```

Shown when `processIdx >= len(processItems)`. Any keypress returns to inbox view.

### New Constants and Model Fields

#### Constants (add to app.go)

```go
// viewState
viewProcessInbox viewState = 4  // (after viewTaskDetail = 3)

// processStep enum (new type)
type processStep int
const (
    stepActionable    processStep = iota  // "Is it actionable?"
    stepNotActionable                     // "Trash or Someday?"
    stepEnrich                            // Edit fields hub
    stepEnrichTags                        // Adding tags one at a time
    stepRoute                             // "Where does it go?"
    stepDelegatedTo                       // Text input for delegated_to
    stepNewProject                        // Text input for project title
    stepComplete                          // Summary screen
)

// processRoute tracks what happens after enrichment
type processRoute string
const (
    routeActionable processRoute = "actionable"  // enrich -> route step
    routeSomeday    processRoute = "someday"      // enrich -> refile as someday/maybe
)
```

#### Model fields (add to Model struct)

```go
// Process inbox state
processItems  []model.Task    // snapshot of inbox tasks at activation
processIdx    int             // current item index (0-based)
processStep   processStep     // current decision tree step
processTask   model.Task      // working copy of current item for enrichment
processRoute  processRoute    // determines behavior after enrich step
processTags   []string        // accumulator for tags in stepEnrichTags
processStats  processStats    // counts per action type for completion screen
```

#### Stats struct

```go
type processStats struct {
    trashed   int
    someday   int
    done      int
    waiting   int
    refiled   int  // to single actions
    toProject int
    skipped   int
}
```

### Key Implementation Patterns

#### 1. Reuse existing components, don't duplicate

- **Text input** (`m.input`): Already shared across modeAdding, modeEditingField, etc. Process inbox reuses it for editing task text, notes, delegated_to, and project title. Set `m.mode = modeEditingField` when editing, and check `m.view == viewProcessInbox` in the modeEditingField handler to route the result back to process inbox state.
- **Date picker** (`m.datePicker`): Set `m.mode = modePickingDate`. The existing `updatePickingDate` handler returns to the previous view. Check `m.view == viewProcessInbox` to return to stepEnrich instead of task detail.
- **Project picker** (`modePickingProject`): The existing `updatePickingProject` handler calls `moveToProject` and handles `taskRefiledMsg`. Check `m.view == viewProcessInbox` in `taskRefiledMsg` to advance to the next item instead of reloading a list view.

#### 2. Working copy pattern

Same as task detail view. `processTask` is a copy of `processItems[processIdx]`. Enrichment edits mutate `processTask`. When the final action fires (refile/done/trash), `processTask` fields are used to build the service call. For enrichment persistence before refile:

```go
// Persist enrichment: update the task in inbox first, then refile
// 1. Call svc.UpdateTask(ListIn, processTask) to save enriched fields
// 2. Then call the refile/state-change service method
```

This two-step approach (update then refile) ensures enrichment data survives the move. Wrap both calls in a single `tea.Cmd` closure.

#### 3. Advancing to next item

After each action completes (via message handler):

```go
func (m *Model) advanceProcessInbox() {
    m.processIdx++
    if m.processIdx >= len(m.processItems) {
        m.processStep = stepComplete
    } else {
        m.processStep = stepActionable
        m.processTask = m.processItems[m.processIdx]  // fresh working copy
    }
}
```

#### 4. Back navigation

Each step has an `esc` that goes back one step:

| Current Step | esc Goes To |
|---|---|
| stepActionable | Quit process inbox, return to inbox view |
| stepNotActionable | stepActionable |
| stepEnrich | stepActionable (if came from yes) or stepNotActionable (if came from someday) |
| stepEnrichTags | stepEnrich (discard tags from this session) |
| stepRoute | stepEnrich |
| stepDelegatedTo | stepRoute |
| stepNewProject | stepRoute |
| stepComplete | Return to inbox view |

Note: going back from stepEnrich discards any enrichment edits made during that visit. The working copy is reset from `processItems[processIdx]`.

#### 5. Update dispatch

In the `Update()` method's `tea.KeyPressMsg` handler, add a check for `m.view == viewProcessInbox` that dispatches to `updateProcessInbox(msg)`. This should be checked:
- **After** mode-specific handlers (modeEditingField, modePickingDate, modePickingProject) — so those shared modes work correctly during process inbox
- **Before** the default view-specific handlers

The mode-specific handlers (modeEditingField, modePickingDate, modePickingProject) need small additions to check `m.view == viewProcessInbox` and route their results back to the process inbox flow instead of the normal views.

#### 6. View dispatch

In `View()`, add a case for `viewProcessInbox` that calls `renderProcessInbox()`. This renders differently based on `m.processStep`. If a shared mode is active (modeEditingField, modePickingDate, modePickingProject), those renderers take priority (they already do in the current View dispatch).

### Render Function Structure

```go
func (m Model) renderProcessInbox() string {
    switch m.processStep {
    case stepActionable:
        // Header with progress, task display, "Is it actionable?" prompt
    case stepNotActionable:
        // "Not actionable" prompt with t/m options
    case stepEnrich:
        // Task fields display + edit keybindings
    case stepEnrichTags:
        // Current tags + text input + tab/enter/esc help
    case stepRoute:
        // Route options
    case stepDelegatedTo:
        // Text input for delegated_to
    case stepNewProject:
        // Text input for project title
    case stepComplete:
        // Summary stats
    }
}
```

Each case builds a styled string using Lipgloss. The task display (used in actionable and enrich steps) should be a shared helper that renders the task's current attributes from `processTask`.

### Files to Modify

Only one file: **`internal/tui/app.go`**

- Add processStep type and constants (~15 lines)
- Add processStats struct (~10 lines)
- Add model fields (~8 lines)
- Add `updateProcessInbox()` handler (~200 lines)
- Modify `updatePickingProject` to handle process inbox context (~10 lines)
- Modify `updateEditingField` / modeEditingField handler to handle process inbox context (~15 lines)
- Modify `updatePickingDate` handler to handle process inbox context (~10 lines)
- Modify `taskRefiledMsg` handler to advance process inbox when applicable (~10 lines)
- Add `renderProcessInbox()` (~150 lines)
- Add `advanceProcessInbox()` helper (~10 lines)
- Add activation keybinding `P` in inbox normal mode (~10 lines)
- Modify View() dispatch (~5 lines)
- Modify Update() dispatch (~5 lines)

Estimated total: ~450 lines of new/modified code.

### Not in Scope

- Tag autocomplete (future enhancement — input mechanism is designed to support it)
- URL editing during enrichment (can be added later with same pattern)
- Multi-line notes editing (single-line input, same as task detail view)
- Reordering within project after refile (use project detail view)
- Undo/back after a mutation has been persisted (item is gone from inbox)

---

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
