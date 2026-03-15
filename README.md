# wnwn

A GTD (Getting Things Done) TUI app built in Go. Simple, configurable, and a little quirky — because productivity software shouldn't be boring.

Runtime data is stored in SQLite, with first-class Markdown import/export for portability and backup.

## Features

- **Three-tab TUI** — Inbox, Single Actions, Projects
- **Full GTD workflow** — capture, process, organize, review
- **CLI quick capture** — add tasks from any terminal without opening the TUI
- **SQLite runtime + Markdown interchange** — fast local storage with portable plain-text import/export
- **GTD contexts via tags** — `@home`, `@computer`, `@errands`, etc.
- **Project sub-groups** — organize large projects into named phases or milestones
- **Project editing** — rename projects (auto-renames the file), set state, deadline, URL, definition of done, and waiting-on
- **Process Inbox mode** — guided GTD decision tree to work through inbox items one at a time
- **Explicit archiving** — mark done/canceled without auto-removal; archive manually when ready
- **Weekly Review mode** — guided multi-section sweep for stale projects, aging waiting-for, someday/maybe, and recent archives
- **Hybrid keybindings** — supports quick single-key actions plus two-step prefixes for grouped actions (`s` state, `r` route, `t` time)

### Task Lifecycle Semantics

- `done` / `canceled` are **state changes** (what happened to the task)
- `archive` is a **location change** (move completed/abandoned tasks out of active working lists)
- `trash` is a **hard delete** (remove permanently, not archived)

## Installation

Requires Go 1.25+ (project uses [mise](https://mise.jdx.dev/) to manage the Go version).

```bash
git clone https://github.com/yourusername/wnwn
cd wnwn
eval "$(mise activate bash)"   # if using mise
go build -o wnwn ./cmd/wnwn/
```

Move `wnwn` somewhere on your `$PATH`.

## Usage

### Launch the TUI

```bash
wnwn
```

### Quick capture from the terminal

```bash
wnwn add "Buy milk"
wnwn add "Book flights" --deadline 2026-03-15 --tag travel --tag "@computer" --url https://flights.example.com
wnwn add "Write report" --scheduled 2026-03-10 --notes "Focus on Q1 numbers"

# URL capture examples (Chrome/Slack/etc.)
wnwn add "Review Slack thread" --url "https://app.slack.com/client/T123/C456/thread/C456-1710000000.000100"
wnwn add "Watch talk from Chrome tab" --url "https://www.youtube.com/watch?v=dQw4w9WgXcQ" --tag "@watch"
```

Tasks added via CLI land in your inbox for later processing and are written to your default wnwn data store (unless you override with `WNWN_DATA_DIR`).

### Query and update from the terminal

```bash
# Query tasks using the DSL (JSON by default)
wnwn query --tasks --query "deadline:today..7d"
wnwn query --projects --query "state:active AND tag:project"

# Update a task by stable ID
wnwn update --task-id 01ABCDEF --state done
wnwn update --task-id 01ABCDEF --deadline 2026-03-20 --tags "@computer,deep-work"
wnwn update --task-id 01ABCDEF --clear deadline,scheduled

# Update a project by stable ID
wnwn update --project-id 01PROJECT --title "Launch Website" --state active
wnwn update --project-id 01PROJECT --deadline 2026-06-01 --clear deadline
```

These commands are designed to be script-friendly, which makes them useful for shell automation and local agents that need to inspect or mutate GTD state without launching the TUI.

### Markdown import/export

```bash
# Export current SQLite data to Markdown files
wnwn export-md --out /tmp/wnwn-export

# Preview import plan without writing
wnwn import-md --from /tmp/wnwn-export --dry-run

# Merge import (adds missing tasks/projects by ID/filename)
wnwn import-md --from /tmp/wnwn-export --mode merge

# Replace import (reset DB, then import all markdown data)
wnwn import-md --from /tmp/wnwn-export --replace
```

### Configuration

Set `WNWN_DATA_DIR` to choose where app data is stored.
Defaults follow XDG data conventions: `$XDG_DATA_HOME/wnwn` when set, otherwise `~/.local/share/wnwn`.

```bash
export WNWN_DATA_DIR=~/Dropbox/wnwn
```

Config file resolution order:

1. `WNWN_CONFIG_FILE` (explicit override)
2. `$XDG_CONFIG_HOME/wnwn/config.toml` (or `~/.config/wnwn/config.toml`)
3. Legacy fallback: `$WNWN_DATA_DIR/config.toml`

Create `~/.config/wnwn/config.toml` (or your `XDG_CONFIG_HOME` equivalent) to customize behavior:

```toml
[archive]
auto_archive_done = false
auto_archive_canceled = false

[ui]
default_view = "inbox" # inbox | actions | projects | views

[keys.list]
done = "d"
cancel = "c"
someday = "m"
archive = "A"
trash = "x"
process = "P"

[keys.project]
done = "d"
cancel = "c"
archive = "A"
trash = "x"

[keys.view_results]
done = "d"
cancel = "c"
someday = "m"
archive = "A"
trash = "x"
refresh = "R"

[keys.disable]
# Disable direct one-key actions while still allowing prefixes.
list = ["done", "cancel", "waiting", "someday", "refile_project"]
project = ["done", "cancel"]
view_results = ["done", "cancel", "waiting", "someday"]
```

Keybinding action names supported:

- `keys.list`: `add`, `open_url`, `refile_actions`, `refile_project`, `someday`, `waiting`, `done`, `cancel`, `archive`, `trash`, `process`
- `keys.project`: `add_task`, `add_subgroup`, `open_url`, `rename_subgroup`, `delete_subgroup`, `done`, `cancel`, `archive`, `trash`, `move_subgroup`
- `keys.view_results`: `done`, `cancel`, `open_url`, `someday`, `waiting`, `archive`, `restore`, `trash`, `refresh`
- `keys.disable`: action names to disable as direct one-key shortcuts (per scope)

## Keyboard Shortcuts

### Global

| Key | Action |
|-----|--------|
| `Tab` / `1` / `2` / `3` | Switch between Inbox, Actions, Projects |
| `j` / `k` / arrows | Navigate list |
| `g` / `G` | Jump to top / bottom |

### Inbox

| Key | Action |
|-----|--------|
| `a` | Add task inline |
| `P` | Process inbox (guided GTD decision tree) |
| `e` | Open task detail / edit view |
| `r` then `a` | Route to Single Actions as next action |
| `p` | Refile to a project |
| `s` then `m` | Set someday/maybe |
| `s` then `d/c/w` | Set done / canceled / waiting-for |
| `t` then `d/s` | Edit deadline / scheduled via date picker |
| `w` | Set waiting-for |
| `d` | Mark done (stays in list) |
| `c` | Mark canceled (stays in list) |
| `A` | Archive selected task |
| `x` | Trash (permanent delete) |

### Single Actions

| Key | Action |
|-----|--------|
| `e` | Open task detail / edit view |
| `p` | Refile to a project |
| `s` then `m/d/c/w` | State prefix actions |
| `m` / `w` / `d` / `c` | Quick state changes |
| `t` then `d/s` | Edit deadline / scheduled via date picker |
| `x` | Trash (permanent delete) |
| `A` | Archive selected task |

### Projects

| Key | Action |
|-----|--------|
| `enter` | Open project detail |
| `a` | Create new project |
| `E` | Edit project metadata (title, state, tags, deadline, URL, etc.) |

### Project Structuring Tips

- Default to a single sub-group (for example `Tasks`) for most projects.
- Add sub-groups only when they meaningfully reflect phases/areas you actively review (for example `Research`, `Build`, `Launch`).
- If you rarely move tasks between sub-groups, keep projects flat and use states/tags/views for organization instead.

### Project Detail

| Key | Action |
|-----|--------|
| `e` | Open task detail / edit view |
| `a` | Add task to current sub-group |
| `n` | Add new sub-group |
| `d` | Mark task done |
| `c` | Mark task canceled |
| `s` then `m/d/c/w` | State prefix actions |
| `t` then `d/s` | Edit deadline / scheduled via date picker |
| `A` | Archive selected task |
| `x` | Trash selected task (permanent delete) |
| `E` | Edit project metadata |
| `ctrl+j` / `ctrl+k` | Reorder task within sub-group |
| `m` | Move task to a different sub-group |
| `esc` | Back to project list |

### Views / Weekly Review

| Key | Action |
|-----|--------|
| `4` / `V` | Open Views tab |
| `enter` | Open selected saved view |
| `/` | Run ad-hoc query |
| `W` | Start guided Weekly Review mode |

In Weekly Review mode:

- `h`/`l` (or arrows) move between sections
- `j`/`k` navigate items in a section
- `enter` opens selected item (project detail or task detail)
- `s` then `m/d/c/w` uses state prefix; `t` then `d/s` edits deadline/scheduled quickly
- `d`/`c`/`m`/`w`, `A`, `x` still work as direct shortcuts on active (non-archived) tasks
- archived section is read-only

## Markdown Export Format

Imported/exported Markdown tasks use checkbox items with YAML metadata blocks:

```markdown
- [ ] Book flights to/from Hawaii
  ```yaml
  id: 01JNQX7K9MZYZ3VDEHGR8N0P6F
  created: 2026-02-28T10:30
  state: next-action
  deadline: 2026-03-15T17:00
  tags: [travel, "@computer"]
  ```

  Check Southwest and United for direct flights from SFO.
```

### Data Directory Layout

```
~/.local/share/wnwn/
  in.md                  # inbox
  single-actions.md      # standalone next actions
  projects/              # one .md file per project
    launch-website.md
  archive/               # archived tasks export
    archive.md
```

### Task States

| State | Checkbox | Applies to | Meaning |
|-------|----------|------------|---------|
| *(empty)* | `- [ ]` | Tasks | Unprocessed inbox item |
| `next-action` | `- [ ]` | Tasks | Committed next action |
| `active` | `- [ ]` | Projects | Project is actively being pursued |
| `waiting-for` | `- [ ]` | Tasks & Projects | Blocked on someone or something |
| `some-day/maybe` | `- [ ]` | Tasks & Projects | Deferred, not committed |
| `done` | `- [x]` | Tasks & Projects | Completed |
| `canceled` | `- [-]` | Tasks & Projects | Abandoned |

Practical rule of thumb: use `canceled` when the history matters, `trash` when the task was noise/mistake, and `archive` when you're done reviewing an item in active lists.

### Task Attributes

| Field | Description |
|-------|-------------|
| `id` | ULID, auto-generated |
| `created` | Auto-set on creation |
| `state` | Task state (see above) |
| `scheduled` | When you intend to work on it |
| `deadline` | When it must be done |
| `tags` | Contexts and categories (e.g. `@computer`, `deep-work`) |
| `url` | Link to where the work happens |
| `waiting_on` | Who or what you're waiting on (nudged when state is `waiting-for`) |
| `waiting_since` | Auto-set when entering `waiting-for` state |
| notes | Free-form Markdown prose below the YAML block |

## Development

```bash
# Run tests
go test ./...

# Build
go build -o wnwn ./cmd/wnwn/

# Rebuild + replace import from bundled test data
go build -o wnwn ./cmd/wnwn/ && ./wnwn import-md --from ./testdata --replace

# Test with a throwaway data directory
WNWN_DATA_DIR=/tmp/wnwn-test ./wnwn
```

The data layer has 31 passing tests covering the parser, writer, and service layer.

## License

Licensed under the Apache License, Version 2.0. See `LICENSE`.

## Contributing

Contributions are welcome. See `CONTRIBUTING.md` for setup and DCO sign-off requirements.

## Roadmap

The core capture/organize/review workflow is functional. Planned next:

- **Views and filtering** — text-based query DSL (`state:next-action tag:@home`, `deadline:<2026-03-07`) with saved named views
- **Fuzzy search** — free-text search across task names, notes, and all content
- **Weekly Review mode** — guided flow to review projects, waiting-for items, and someday/maybe
- **Theming and advanced config** — extend config beyond behavior + keybindings (themes, default tags, saved views persistence)
- **Tickler File** — skeuomorphic 43-folder visualization (31 days + 12 months) over the agenda view

## Tech Stack

- [Bubbletea v2](https://charm.land/bubbletea) — TUI framework
- [Lipgloss v2](https://charm.land/lipgloss) — styling and layout
- [Bubbles v2](https://charm.land/bubbles) — reusable TUI components
- [oklog/ulid](https://github.com/oklog/ulid) — stable task IDs
- [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) — YAML metadata parsing
