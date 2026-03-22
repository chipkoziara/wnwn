# wnwn Architecture Evolution Plan

## Overview

This document describes the plan to evolve wnwn from a monolithic TUI application into a **core + client architecture**, enabling multiple clients (TUI, web, iOS) to share a single well-defined domain core. The inspiration is Mitchell Hashimoto's libghostty approach in Ghostty: build a clean core library, make the existing app a consumer of it, then open the door for other clients.

The repository is at `github.com/chipkoziara/wnwn`. The current codebase is Go, uses SQLite for storage, Bubble Tea for the TUI, ULID-based IDs, a custom query DSL, and a TOML config file.

This is an **incremental extraction plan**, not a rewrite. The goal is to introduce a hard service boundary over the existing implementation, migrate the TUI and CLI to depend on it, and only then refine internal package boundaries where the new seams prove useful.

---

## Architecture Layers

### Core Layer ("libwnwn")

The core is the hard boundary. Everything below this line is client-agnostic. The core should be testable and drivable entirely without any UI.

**Service Interface** — the single logical entry point for all clients. It should be exposed as one top-level `Service`, but internally composed from smaller sub-interfaces.

```go
type Service interface {
    TaskService
    ProjectService
    InboxService
    ViewService
    ReviewService
    ImportExportService
}
```

The service boundary should be **ID-first**. Tasks, projects, and any sub-groups exposed through the service contract should be addressed by stable IDs. Filenames, slugs, list indexes, subgroup indexes, and cursor positions are implementation details that stay inside the store layer or client layer. Project sub-groups should ultimately be exposed as stable-ID entities at the boundary, but the exact service shape for subgroup operations remains an explicit design item during extraction.

**Inbox / Process Workflow**

Process Inbox is a first-class workflow owned by the core, not by the client. It should not be modeled as a thin `next + decide` API. Instead, the core should expose a session/state-machine contract created and managed through the `Service` boundary:

- the core owns the current workflow stage
- the core owns the in-memory draft/working copy during the session
- the core exposes the current step and allowed actions
- the client submits explicit typed actions back to the core
- only final workflow actions commit to persistent storage

Process Inbox sessions are intentionally **ephemeral and non-durable**:

- draft edits are held only in memory
- mid-session changes are not persisted automatically
- if the client exits or disconnects mid-process, the draft is discarded
- the underlying inbox item remains unchanged until a final workflow action commits atomically

This makes Process Inbox the one intentionally stateful workflow in the core, while preserving the current mental model of "working copy until explicit commit."

**Domain Logic** — client-agnostic, opinionated about GTD semantics:

- **GTD State Machine**: Enforces valid transitions and task/project semantics. No client should hardcode these rules.
- **Query DSL**: Parser, evaluator, date range resolution, boolean logic, and field matching. Used by saved views, ad-hoc search, CLI queries, and any future client.
- **Process Inbox Engine**: Workflow stages, valid actions, validation rules, next-item selection, and atomic final commits.
- **Weekly Review Engine**: Section generation, stale project detection, aging waiting-for, someday/maybe sweep, and recent archive summary.

**Interchange / Import-Export Logic**

Markdown import/export remains part of the service boundary, but it is not treated as pure domain logic. Internally it should be considered **interchange/application-layer logic**:

- Markdown export serialization
- Markdown import parsing
- merge vs. replace logic
- dry-run support
- ID preservation and source mapping

**Storage Layer**

- **Store Interface**: Abstract CRUD operations, transaction support, and migration runner. The core depends on this interface, not on SQLite directly.
- **SQLite Implementation**: Schema, migrations, ULID generation, and any future FTS/search support. Implements the Store interface.

**Core Config**

- `CoreConfig` is a struct passed in at initialization. The core never loads config from disk — that is the client's job.
- It contains behavior-affecting settings such as `auto_archive_done`, `auto_archive_canceled`, and future workflow preferences.
- Saved views are a **core concept**, but not all saved views are persisted by the core. The core owns their semantics and merged runtime list; clients may supply additional config-defined views at initialization.

### Adapter Layer

Transport between clients and the core.

- **Direct (in-process)**: TUI and CLI call the `Service` interface directly via Go function calls. Zero overhead.
- **HTTP API (localhost)**: JSON over REST, wraps the `Service` interface. Enables non-Go clients (web, iOS, AI agents). Built later, but the `Service` interface should make it straightforward to implement.

### Client Layer

Each client owns its own config and presentation. Clients receive a `Service` (or talk to one over HTTP) and never contain GTD workflow logic.

- **TUI Client (Bubble Tea)**: Keybindings, view models, theme, Bubble Tea-specific state. Owns `TUIConfig` (keybindings, default view, disable lists, key prefixes). Owns config loading (TOML parsing, XDG resolution, env var overrides). Extracts `CoreConfig` from the TOML file and passes it to the `Service` at init.
- **CLI Client**: Flag parsing, output formatting. Thin wrapper around `Service` calls.
- **Future Web Client**: Browser UI (React, Svelte, etc.) talking to the HTTP adapter.
- **Future iOS Client**: Swift UI talking to the HTTP adapter (or via gomobile if offline support is needed later).

---

## Config Architecture

Split the current TOML config into two concerns:

**CoreConfig** (passed into Service at init):
```toml
# These affect domain behavior regardless of client
[archive]
auto_archive_done = false
auto_archive_canceled = false
```

**TUIConfig** (owned by the TUI client):
```toml
[ui]
default_view = "inbox"

[keys.list]
done = "d"
cancel = "c"
# ... all keybinding config

[keys.disable]
list = ["done", "cancel", "waiting", "someday", "refile_project"]
```

The TOML file can keep its current shape. The change is internal: who parses what and who receives which parts. The core should never import a config-loading package that knows about file paths or TOML.

### Saved Views

Saved views use a **hybrid source model**:

1. **Built-in/system views** defined by the core
2. **Config-defined views** supplied by the client at initialization
3. **Future persisted user-created views** are possible later, but are out of scope for Phase 1

The core owns:
- what a saved view is
- how its query runs
- how the final runtime view list is merged and exposed

The client owns:
- parsing config-defined views from TOML
- any client-specific display metadata or presentation choices

For Phase 1, saved-view APIs should remain **read-oriented**:
- list views
- run a saved view
- run an ad-hoc query

Mutable saved-view APIs (`create`, `update`, `delete`) are deferred until persistence ownership is intentionally revisited. Phase 1 does not change the current config-backed custom-view persistence model. If config-defined views collide with built-in views, **config-defined views override built-ins by name**.

---

## SQLite-over-Dropbox (Best-Effort Single-User Sync)

For careful personal multi-device use with Dropbox (or iCloud Drive), a SQLite database in a synced folder can be workable as an **advanced best-effort setup**, but it is not equivalent to a true concurrent sync system.

Recommended guardrails:

- Use `PRAGMA journal_mode=DELETE` (avoids multi-file WAL consistency issues), OR use WAL mode with `PRAGMA wal_checkpoint(TRUNCATE)` on clean shutdown.
- On startup, run `PRAGMA quick_check` (or `PRAGMA integrity_check`) to verify the database is intact.
- Use a lockfile next to the database. On startup, check for the lockfile and warn if present (with a "force open anyway" override for stale locks). On clean shutdown, remove the lockfile.
- Markdown export serves as the recovery fallback: if the database fails integrity check, rebuild from the last known-good export.
- Never open the database on two machines simultaneously. The lockfile is advisory only; it is not a distributed consensus mechanism.

This mode assumes exactly one active writer/device at a time. Unexpected shutdowns, delayed sync, or user error can still cause conflicts. Markdown export remains the resilience mechanism and recovery path.

---

## Mobile Capture via Obsidian

Before building any mobile client, solve mobile capture using existing tools:

1. Create a dedicated `wnwn-inbox.md` file in an Obsidian vault that syncs via iCloud or Dropbox.
2. On the phone: open Obsidian, open the note, type one task per line, close. Zero friction.
3. On the main machine: a shell script watches the file (via cron every 2–5 minutes) and pipes each line into `wnwn add`, then clears the file.

Example watcher script:

```bash
#!/bin/bash
INBOX="$HOME/path/to/vault/wnwn-inbox.md"
if [ -s "$INBOX" ]; then
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        wnwn add "$line"
    done < "$INBOX"
    > "$INBOX"
fi
```

This gets mobile capture working immediately with no new code in wnwn itself. Processing and review still happen in the TUI on the main machine, which aligns with GTD's separation of capture from processing.

---

## Recommended Sequencing

### Phase 1: Core Extraction

1. **Map the dependency graph.** Identify where TUI concerns (Bubble Tea models, view rendering) are entangled with domain logic (state transitions, query parsing) and storage (SQLite operations). This is a few hours of analysis, not days.

2. **Define the `Service` boundary first.** Introduce the new `Service` interface in a `core` or `engine` package as a facade over existing capabilities. Use the current CLI and TUI flows as the guide: if a new client from scratch could drive capture, process, query, review, and import/export through this boundary alone, the contract is right.

3. **Migrate the TUI and CLI to depend on the `Service`.** This validates the boundary against real usage before deeper package churn. Process Inbox is the main hard case here because it should move to the session/state-machine model described above.

4. **Split config ownership.** Separate `CoreConfig` from `TUIConfig`. The TUI loader reads TOML and passes `CoreConfig` plus config-defined saved views into the Service at init.

5. **Write integration tests against the `Service` interface directly.** Open an in-memory SQLite database, create a Service, and exercise the full GTD workflow (capture, process, query, weekly review, import/export) with no TUI and no CLI parsing. This is the proof the separation is real.

6. **Refine internal package boundaries only after the service seam is in use.** Once the new boundary is proven, move logic behind clearer sub-packages where it helps:
   - `store/` — SQLite implementation, Store interface, migrations
   - `domain/` — GTD state machine, query DSL, Process Inbox engine, Weekly Review engine
   - `interchange/` or `markdown/` — Markdown import/export
   - `core/` or `engine/` — Service interface, CoreConfig, orchestration

**Validation test:** Can you write a comprehensive test suite against the core/service that never imports anything from the TUI package? If yes, the separation is real.

### Phase 2: Mobile Capture (Obsidian Bridge)

7. Set up the `wnwn-inbox.md` file in an Obsidian vault with iCloud/Dropbox sync.
8. Write the watcher script (cron or fswatch) on the main machine.
9. Use this daily and iterate on the capture format if needed (e.g. supporting `#tag` or `!deadline` syntax in the watcher script).

This phase requires zero changes to wnwn core — it's purely external tooling leveraging the existing `wnwn add` CLI.

### Phase 3: HTTP Adapter

10. Build a lightweight HTTP server that wraps the `Service` interface. JSON over REST on localhost. The HTTP layer should expose the service capabilities over a stable local API.
11. Optionally refactor the CLI commands to become thin wrappers around the HTTP API (this exercises the API constantly and ensures parity).

### Phase 4: Web Client

12. Build a mobile-friendly web client (React, Svelte, or similar) that talks to the HTTP adapter on localhost.
13. This validates the adapter with a real second client and gives you a GUI for daily use.

### Phase 5: Remote Access via Tailscale

14. Install Tailscale on the machine running the HTTP adapter.
15. Access the web client from your phone over Tailscale's encrypted network — secure remote access with zero cloud deployment, no public server, no auth layer to build.
16. This gives you full mobile access (capture, review, processing) from anywhere.

### Phase 6: Native iOS Client (Later)

17. Native Swift UI app that talks to the HTTP adapter (same as the web client, but native).
18. Evaluate whether gomobile FFI is needed for offline support, or whether the HTTP approach is sufficient.

---

## Key Architectural Principles

- **The Service interface is the hard boundary.** No GTD workflow logic in clients. No client-specific concerns in the core.
- **The service contract is ID-first.** Stable IDs cross the boundary; filenames, slugs, indexes, and cursors do not.
- **The core is a library.** It doesn't read files from disk, load config, or know about network transport. Callers do that and pass results in.
- **Process Inbox is a core-owned workflow.** The core exposes the current step and valid actions, owns an in-memory draft during the session, and only final actions commit atomically.
- **Process Inbox sessions are ephemeral.** They are not durable or resumable in Phase 1; client exit/disconnect discards the draft.
- **CoreConfig is passed in, not loaded.** The core accepts a `CoreConfig` struct at init time. It doesn't care whether it came from TOML, a database row, environment variables, or hardcoded test defaults.
- **The HTTP adapter pattern is the pragmatic path for non-Go clients.** Avoids FFI/gomobile complexity while keeping the core in Go. A native iOS app, a web UI, and a local AI agent can all speak HTTP to the same Service.
- **Markdown export is both a feature and a resilience mechanism.** It's an interchange format, backup format, and recovery path if SQLite gets corrupted.
- **Service calls define mutation atomicity where feasible.** Clients may hold working copies locally, but persisted mutations should happen only on explicit save/submit actions. Dry-run import remains non-mutating, while destructive import modes such as replace should be treated as atomic where feasible.
- **Go is the right language for the core** given the current codebase, the Bubble Tea TUI, and the HTTP adapter strategy. The Service interface is the abstraction that matters — if a language change ever became necessary, the interface contract tells you exactly what to reimplement.

---

## Anti-Goals for Phase 1

- Do not rewrite wnwn from scratch.
- Do not require a package reorganization before the new service boundary is in active use.
- Do not block all user-facing work on completion of the refactor.
- Do not require HTTP, web, or iOS clients before the architecture work pays off.
- Do not introduce mutable saved-view persistence yet.
- Do not make Process Inbox sessions durable or resumable in the initial extraction.
- Do not treat Dropbox/shared-folder SQLite usage as a fully supported concurrent sync system.
- Do not treat package movement/renaming as a goal in itself.

---

## Deployment Modes (Same Core, Different Wiring)

- **Local**: SQLite file on disk. TUI + CLI call Service directly in-process.
- **Dropbox/Shared FS**: SQLite in synced folder as an advanced single-user setup, with single-writer discipline, advisory lockfile convention, and Markdown export as recovery fallback.
- **Web Server** (future): SQLite per user tenant. HTTP adapter. Auth + session layer. Litestream or similar for backup.

All three modes use the same core and the same Service interface. The difference is how the Service is instantiated and how clients connect to it.
