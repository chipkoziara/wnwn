# wnwn

A Getting Things Done (GTD) inspired TUI personal productivity app built in Go. Simple, configurable, and a little quirky — because productivity software shouldn't be boring.

Runtime data lives in SQLite for fast local access, with first-class Markdown import/export for portability, backup, and interoperability.

## Status
This is an early release, not a finished product. Expect core workflow iteration and intentionally deferred features.

The core GTD workflow is already usable end-to-end:
- capture into inbox
- process and refile
- manage single actions and projects
- review work through saved views and weekly review
- query and update tasks/projects from the CLI

## Features

- **Four top-level TUI views** — Inbox, Actions, Projects, and Views
- **Full GTD workflow** — capture, process, organize, review
- **CLI quick capture** — add tasks from any terminal without opening the TUI
- **CLI query + update** — inspect and mutate tasks/projects by stable ID from scripts or local agents
- **SQLite runtime + Markdown interchange** — fast local storage with portable plain-text import/export
- **Saved views + query DSL** — filter across inbox, actions, projects, and archive
- **Fuzzy search in Views** — quick free-text search across task content and provenance
- **Process Inbox mode** — guided GTD decision tree to work through inbox items one at a time
- **Weekly Review mode** — guided multi-section sweep for stale projects, aging waiting-for, someday/maybe, and recent archives
- **Project sub-groups** — organize large projects into named phases or milestones
- **Project editing** — rename projects, edit metadata, and preserve file-slug behavior automatically
- **Explicit archiving** — mark done/canceled without auto-removal; archive manually when ready
- **Configurable keybindings and saved views** — adapt navigation and view layout to your workflow

## Task lifecycle semantics

- `done` / `canceled` are **state changes** (what happened to the task)
- `archive` is a **location change** (move completed/abandoned tasks out of active working lists)
- `trash` is a **hard delete** (remove permanently, not archived)

## Installation

For **v0.1.0**, `wnwn` is distributed as a **source-build install**.

That means:
- the supported install path is cloning the repository and building with Go
- prebuilt binaries are not part of the initial public release
- [mise](https://mise.jdx.dev/) is optional convenience for contributors and users who want the project-pinned Go toolchain

### Requirements

- Go 1.25+
- Optional: [mise](https://mise.jdx.dev/) for matching the project Go toolchain

### Build from source

```bash
git clone https://github.com/chipkoziara/wnwn.git
cd wnwn

# optional, if you use mise
# eval "$(mise activate bash)"

go build -o wnwn ./cmd/wnwn/
```

Move `wnwn` somewhere on your `$PATH`, or run it from the repo root.

Prebuilt binaries may come in a later release, but they are not part of `v0.1.0`.


## Quick start

### First run in 60 seconds

```bash
git clone https://github.com/chipkoziara/wnwn.git
cd wnwn
go build -o wnwn ./cmd/wnwn/
WNWN_DATA_DIR=/tmp/wnwn-demo ./wnwn add "Buy milk"
WNWN_DATA_DIR=/tmp/wnwn-demo ./wnwn
```

That gives you:
- a local build of the app
- a throwaway demo data directory
- one captured inbox item to process when the TUI opens

### Reproducible demo flow

If you want a richer demo dataset instead of starting nearly empty:

```bash
# from the repo root
go build -o wnwn ./cmd/wnwn/
rm -rf /tmp/wnwn-demo
WNWN_DATA_DIR=/tmp/wnwn-demo ./wnwn import-md --from ./testdata --mode replace
WNWN_DATA_DIR=/tmp/wnwn-demo ./wnwn
```

For a Weekly Review-focused demo:

```bash
go build -o wnwn ./cmd/wnwn/
rm -rf /tmp/wnwn-review-demo
WNWN_DATA_DIR=/tmp/wnwn-review-demo ./wnwn import-md --from ./testdata/weekly-review --mode replace
WNWN_DATA_DIR=/tmp/wnwn-review-demo ./wnwn
```

Suggested demo path:
- use `./testdata` for a general product walkthrough
- use `./testdata/weekly-review` when you specifically want to show Weekly Review behavior
- use `demo-script.txt` as a lightweight walkthrough aid for demos and onboarding

### Launch the TUI

```bash
wnwn
```

### Capture tasks from the terminal

```bash
wnwn add "Buy milk"
wnwn add "Book flights" --deadline 2026-03-15 --tag travel --tag "@computer" --url https://flights.example.com
wnwn add "Write report" --scheduled 2026-03-10 --notes "Focus on Q1 numbers"
```

Tasks added via CLI land in your inbox for later processing.

### A simple first workflow to try

1. Launch `wnwn`
2. Press `a` to add another inbox item, or use `wnwn add "..."` from another terminal
3. Press `P` to enter Process Inbox mode
4. Route one task to **Actions** and one task to a **Project**
5. Press `Tab` or `4` to open **Views**
6. Open **Next Actions** or run `/ state:next-action`
7. Press `W` to preview Weekly Review mode

That path exercises the app's main loop: capture → clarify → organize → review.

### Query and update from the terminal

```bash
# Query tasks using the same DSL as the Views tab
wnwn query --tasks --query "deadline:today..7d"
wnwn query --tasks --query "state:waiting-for AND tag:@office"
wnwn query --projects --query "state:active"

# Update a task by stable ID
wnwn update --task-id 01ABCDEF --state done
wnwn update --task-id 01ABCDEF --deadline 2026-03-20 --tags "@computer,deep-work"
wnwn update --task-id 01ABCDEF --clear deadline,scheduled

# Update a project by stable ID
wnwn update --project-id 01PROJECT --title "Launch Website" --state active
wnwn update --project-id 01PROJECT --deadline 2026-06-01 --clear deadline
```

## Query DSL quick reference

The query DSL powers saved views, ad-hoc `/` search in the Views tab, and `wnwn query`.

Examples:

```text
state:next-action
state:waiting-for AND tag:@office
NOT state:done
project:launch AND (deadline:today..7d OR scheduled:today..7d)
deadline:<today
scheduled:today
deadline:today..7d
has:url
@computer
```

Supported concepts:
- implicit AND via spaces (`state:next-action tag:@home`)
- explicit `AND`, `OR`, `NOT`
- parentheses for grouping
- date comparisons with `<`, `<=`, `>`, `>=`
- inclusive date ranges with `start..end`
- relative date tokens like `today`, `tomorrow`, `7d`
- `has:field` checks
- bare `@tag` shorthand
- bare text matching across task text and notes

## Fuzzy search

From the **Views** tab:
- press `/` for an ad-hoc DSL query
- press `?` for fuzzy free-text search

Fuzzy search ranks matches across:
- task text
- notes
- tags
- URL
- `waiting_on`
- source / project provenance

Current limitation: fuzzy search is its own mode in the Views tab rather than a combined DSL+fuzzy pipeline.

## Markdown import/export

```bash
# Export current SQLite data to Markdown files
wnwn export-md --out /tmp/wnwn-export

# Preview import plan without writing
wnwn import-md --from /tmp/wnwn-export --dry-run

# Merge import (adds only missing items)
wnwn import-md --from /tmp/wnwn-export --mode merge

# Replace import (reset destination store, then import everything)
wnwn import-md --from /tmp/wnwn-export --mode replace
```

## Configuration

Set `WNWN_DATA_DIR` to choose where app data is stored.
Defaults follow XDG data conventions: `$XDG_DATA_HOME/wnwn` when set, otherwise `~/.local/share/wnwn`.

```bash
export WNWN_DATA_DIR=~/my-wnwn-data
```

Config file resolution order:

1. `WNWN_CONFIG_FILE` (explicit override)
2. `$XDG_CONFIG_HOME/wnwn/config.toml` (or `~/.config/wnwn/config.toml`)
3. Legacy fallback: `$WNWN_DATA_DIR/config.toml`

Example:

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

## Keyboard shortcuts

### Global

| Key | Action |
|-----|--------|
| `Tab` / `1` / `2` / `3` / `4` | Switch between top-level tabs/views |
| `j` / `k` / arrows | Navigate list |
| `g` / `G` | Jump to top / bottom |

> Default tab order includes **Inbox**, **Actions**, **Projects**, and **Views**, but tabs can be reordered in config.

### Inbox

| Key | Action |
|-----|--------|
| `a` | Add task inline |
| `P` | Process inbox (guided GTD decision tree) |
| `e` | Open task detail / edit view |
| `p` | Refile to a project |
| `r` then `a` | Route to Single Actions |
| `r` then `p` | Route to a project |
| `s` then `m/d/c/w` | State prefix actions |
| `t` then `d/s` | Edit deadline / scheduled |
| `m` / `w` / `d` / `c` | Quick state changes |
| `A` | Archive selected task |
| `x` | Trash selected task |

### Actions

| Key | Action |
|-----|--------|
| `e` | Open task detail / edit view |
| `p` | Refile to a project |
| `s` then `m/d/c/w` | State prefix actions |
| `m` / `w` / `d` / `c` | Quick state changes |
| `t` then `d/s` | Edit deadline / scheduled |
| `A` | Archive selected task |
| `x` | Trash selected task |

### Projects list

| Key | Action |
|-----|--------|
| `enter` | Open project detail |
| `a` | Create new project |
| `E` | Edit project metadata |

### Project detail

| Key | Action |
|-----|--------|
| `e` | Open task detail / edit view |
| `o` | Open selected task URL |
| `a` | Add task to current sub-group |
| `n` | Add new sub-group |
| `R` | Rename selected sub-group |
| `X` | Delete selected empty sub-group |
| `s` then `m/d/c/w` | State prefix actions |
| `t` then `d/s` | Edit deadline / scheduled |
| `d` / `c` | Mark task done / canceled |
| `A` | Archive selected task |
| `x` | Trash selected task |
| `E` | Edit project metadata |
| `ctrl+j` / `ctrl+k` | Reorder task within sub-group |
| `m` | Move task to another sub-group |
| `esc` | Back to project list |

### Views tab

| Key | Action |
|-----|--------|
| `enter` | Open selected saved view |
| `/` | Run ad-hoc DSL query |
| `?` | Run fuzzy search |
| `W` | Start guided Weekly Review mode |
| `j` / `k` / `g` / `G` | Navigate the saved view list |

### View results

| Key | Action |
|-----|--------|
| `e` | Open task detail |
| `o` | Open selected task URL |
| `s` then `m/d/c/w` | State prefix actions |
| `m` / `w` / `d` / `c` | Quick state changes |
| `t` then `d/s` | Edit deadline / scheduled |
| `A` | Archive selected task |
| `U` | Restore selected archived task |
| `x` | Trash selected task |
| `R` | Refresh results |
| `esc` | Back to Views |

### Weekly Review

In Weekly Review mode:
- `h` / `l` or arrows move between sections
- `j` / `k` / `g` / `G` navigate items in the active section
- `enter` opens the selected item
- `o` opens the selected task URL
- `d` / `c` / `m` / `w`, `A`, `x` work on active tasks
- archived items are read-only

## Data locations and backup guidance

Default data directory:

```text
~/.local/share/wnwn/
```

Typical layout:

```text
~/.local/share/wnwn/
  wnwn.db                # SQLite runtime store
```

Markdown export/import layout:

```text
export-dir/
  in.md
  single-actions.md
  projects/
    launch-website.md
  archive/
    archive.md
```

For backup and portability in `v0.1.0`, the recommended approach is:
- keep your live data in the SQLite store under your data directory
- periodically run `wnwn export-md --out <dir>` to create a readable Markdown snapshot
- treat Markdown export as the current user-facing backup/interchange format

## Markdown export format

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

## Known limitations for v0.1.0

These are known gaps or intentionally deferred items for the first public release:


- No recurring tasks
- No sync story (local-only)
- Fuzzy search is separate from the DSL rather than composable with it
- Process Inbox undo steering is still an improvement area
- No tickler-file visualization
- `v0.1.0` is source-build only; prebuilt binaries are not included yet

## Development

```bash
# Optional: activate Go via mise
eval "$(mise activate bash)"

# Run tests
go test ./...

# Build
go build -o wnwn ./cmd/wnwn/

# Example import into a local test store
./wnwn import-md --from ./testdata --mode replace

# Test with a throwaway data directory
WNWN_DATA_DIR=/tmp/wnwn-test ./wnwn
```

## License

Licensed under the Apache License, Version 2.0. See `LICENSE`.
