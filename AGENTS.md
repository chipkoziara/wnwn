# wnwn

A GTD TUI app in Go. Spec in `BRD.md`. Current build status and architecture in `STATUS.md`. **Read both before starting work.**

## Project Structure

- `cmd/wnwn/` — entry point (TUI launch + CLI quick capture)
- `internal/model/` — Task, TaskList, Project, SubGroup types
- `internal/parser/` — Markdown → model
- `internal/writer/` — model → Markdown
- `internal/store/` — file I/O abstraction
- `internal/service/` — GTD business logic (service.go = lists, project.go = projects)
- `internal/id/` — ULID generation
- `internal/tui/` — Bubbletea TUI (app.go + styles.go)
- `internal/tui/datepicker/` — custom calendar date picker component

## Tech Stack

- **Go 1.25** via mise (`eval "$(mise activate bash)"`)
- **Bubbletea v2** — import path `charm.land/bubbletea/v2`, NOT `github.com/charmbracelet/bubbletea`. Uses `tea.KeyPressMsg` (not `tea.KeyMsg`) and `tea.View` struct return from `View()` (not string).
- **Lipgloss v2** — `charm.land/lipgloss/v2`
- **Bubbles v2** — `charm.land/bubbles/v2`
- No third-party date picker is compatible with the v2 import paths — the custom datepicker package is the solution.

## Workflow After Every Feature

Once a feature is complete and `go test ./...` passes:

1. **Update `STATUS.md`** — move completed items out of "What's NOT Built Yet", document new keybindings, service methods, architecture patterns, and any important implementation notes for future sessions.
2. **Update `BRD.md` if needed** — only if the implementation meaningfully diverges from or extends the spec. BRD is a stable spec document; don't update it just to match implementation details.
3. **Commit with a detailed message** — include what was built, key design decisions, and any non-obvious implementation notes (e.g. why a third-party library wasn't used, patterns introduced). Multi-paragraph commit messages are appropriate for significant features.

## Code Patterns

- **Service layer is stateless** — every mutation is read → modify → write. No in-memory cache in the service.
- **TUI uses working copies** — when opening a detail/edit view, copy the data into model fields and mutate locally. Only write to disk on explicit save (`s` key). `esc` discards.
- **Bubbletea sub-models** — reusable components (e.g. datepicker) follow the pattern: `Open()` to activate, forward all `tea.Msg` from parent `Update()` while active, read result via `Result()` after each update cycle.
- **Cursor patterns** — `m.cursor` is general-purpose (lists, pickers); `m.projCursor` is exclusively for the flat project detail view. `loadProjectDetail()` resets cursor; `reloadProjectDetail()` preserves it.
- **Flat index projection** — project detail flattens `[]SubGroup{[]Task}` into `[]flatItem` via `flattenProject()`. All rendering and key handlers operate on this flat slice.
- **Status messages** — set `m.statusMsg` synchronously, clear via `tea.Tick(3s)` returning `clearStatusMsg{}`. Any keypress also clears it immediately.

## Running

```bash
eval "$(mise activate bash)"
go test ./...
go build -o wnwn ./cmd/wnwn/
WNWN_DATA_DIR=/tmp/wnwn-test ./wnwn
```
