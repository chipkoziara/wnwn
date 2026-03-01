# g-tuddy

A GTD (Getting Things Done) TUI app built in Go. Simple, configurable, and a little quirky — because productivity software shouldn't be boring.

Data is stored as plain Markdown files ([File Over App](https://stephango.com/file-over-app) philosophy). No proprietary database lock-in.

## Features

- **Three-tab TUI** — Inbox, Single Actions, Projects
- **Full GTD workflow** — capture, process, organize, review
- **CLI quick capture** — add tasks from any terminal without opening the TUI
- **Plain Markdown storage** — human-readable files you can edit with any text editor
- **GTD contexts via tags** — `@home`, `@computer`, `@errands`, etc.
- **Project sub-groups** — organize large projects into named phases or milestones
- **Auto-archiving** — done/canceled tasks move to monthly archive files automatically

## Installation

Requires Go 1.24+ (project uses [mise](https://mise.jdx.dev/) to manage the Go version).

```bash
git clone https://github.com/yourusername/g-tuddy
cd g-tuddy
eval "$(mise activate bash)"   # if using mise
go build -o gtd ./cmd/gtd/
```

Move `gtd` somewhere on your `$PATH`.

## Usage

### Launch the TUI

```bash
gtd
```

### Quick capture from the terminal

```bash
gtd add "Buy milk"
gtd add "Book flights" --deadline 2026-03-15 --tag travel --tag "@computer" --url https://flights.example.com
gtd add "Write report" --scheduled 2026-03-10 --notes "Focus on Q1 numbers"
```

Tasks added via CLI land in your inbox for later processing.

### Configuration

Set `GTD_DATA_DIR` to choose where task files are stored (default: `~/.local/share/gtd`):

```bash
export GTD_DATA_DIR=~/Dropbox/gtd
```

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
| `r` | Refile as next action (→ Single Actions) |
| `p` | Refile to a project |
| `s` | Set someday/maybe |
| `w` | Set waiting-for |
| `d` | Mark done (archives) |
| `x` | Trash (permanent delete) |

### Single Actions

| Key | Action |
|-----|--------|
| `p` | Refile to a project |
| `s` / `w` / `d` / `x` | State changes (same as Inbox) |

### Projects

| Key | Action |
|-----|--------|
| `enter` | Open project detail |
| `a` | Create new project |

### Project Detail

| Key | Action |
|-----|--------|
| `a` | Add task to current sub-group |
| `n` | Add new sub-group |
| `d` | Mark task done |
| `ctrl+j` / `ctrl+k` | Reorder task within sub-group |
| `m` | Move task to a different sub-group |
| `esc` | Back to project list |

## File Format

Tasks are stored as Markdown checkboxes with YAML metadata blocks:

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
~/.local/share/gtd/
  in.md                  # inbox
  single-actions.md      # standalone next actions
  projects/              # one .md file per project
    launch-website.md
  archive/               # monthly archives
    2026-03.md
```

### Task States

| State | Checkbox | Meaning |
|-------|----------|---------|
| *(empty)* | `- [ ]` | Unprocessed inbox item |
| `next-action` | `- [ ]` | Committed next action |
| `waiting-for` | `- [ ]` | Delegated or blocked |
| `some-day/maybe` | `- [ ]` | Deferred, not committed |
| `done` | `- [x]` | Completed |
| `canceled` | `- [-]` | Abandoned |

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
| `delegated_to` | Who it's delegated to |
| `waiting_since` | Auto-set when entering `waiting-for` state |
| notes | Free-form Markdown prose below the YAML block |

## Development

```bash
# Run tests
go test ./...

# Build
go build -o gtd ./cmd/gtd/

# Test with a throwaway data directory
GTD_DATA_DIR=/tmp/gtd-test ./gtd
```

The data layer has 31 passing tests covering the parser, writer, and service layer.

## Roadmap

The core capture/organize workflow is fully functional. Planned next:

- **Task detail/edit view** — view and edit all task attributes from within the TUI
- **Process Inbox mode** — guided GTD decision tree for working through inbox items one at a time
- **Views and filtering** — text-based query DSL (`state:next-action tag:@home`, `deadline:<2026-03-07`) with saved named views
- **Fuzzy search** — free-text search across task names, notes, and all content
- **Weekly Review mode** — guided flow to review projects, waiting-for items, and someday/maybe
- **Config file** — `~/.config/gtd/config.toml` for keybindings, themes, default tags, saved views
- **Tickler File** — skeuomorphic 43-folder visualization (31 days + 12 months) over the agenda view

## Tech Stack

- [Bubbletea v2](https://charm.land/bubbletea) — TUI framework
- [Lipgloss v2](https://charm.land/lipgloss) — styling and layout
- [Bubbles v2](https://charm.land/bubbles) — reusable TUI components
- [oklog/ulid](https://github.com/oklog/ulid) — stable task IDs
- [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) — YAML metadata parsing
