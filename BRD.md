# wnwn: A GTD TUI App

## 1. Vision & Principles

We are building a TUI GTD app, with several key distinctions over other software in this space.

- Simplicity → built with a clear focus with no extra frills in the core
- Configurability → users can adjust key aspects of the software to tailor it to their workflows easily
- Quirkiness and fun → productivity software is often boring. our software should be a joy to use

## 2. Methodology / Domain Model

We are using the GTD methodology as outlined in this website: https://hamberg.no/gtd

Our application will have a few distinctions to make the approach more modern and modular.

### Task Attributes
- The task as written is itself an attribute (attribute: "task", e.g. "book flights to/from Hawaii")
- Task states: empty (unprocessed), "next-action", "waiting-for", "some-day/maybe", "done", "canceled"
- Project states: "active" (being pursued), "waiting-for", "some-day/maybe", "done", "canceled". Projects use "active" rather than "next-action" — a project doesn't have a next action, it *contains* them
- All tasks can have additional optional attributes: "scheduled", "deadline", "notes", "url", "tags", and "waiting_on"
  - "scheduled" must be a datetime, and it denotes when a user intends to work on that task
  - "deadline" must be a datetime, and it denotes when a user must complete that task
  - "notes" is a paragraph text area that can contain a description of the task that the user wants to log for future reference
  - "url" is a string that users can utilize to point to where the work needs to be done (e.g. a link to an email thread in Gmail, a Slack conversation, a website, etc.)
  - "tags" are a list of strings that users can use to add more context to a given task, and these can be searched / filtered (described later)
  - "waiting_on" is an optional string capturing who or what a task or project is blocked on; used when state is "waiting-for" (can be a person, system, event, or external decision)
  - When a task transitions to "waiting for" state, the date of that transition is automatically recorded so users can track how long they've been waiting
  - Projects are auto-assigned a tag called "project", but other tags can be added to them
  - Projects also support "url", "waiting_on", and "definition_of_done" fields in their frontmatter

### Tags
- Tags serve double duty as GTD "contexts" — users can create tags like "@home", "@computer", "@office", "@errands" to filter next actions by location or available tools
- Default tags seeded for new users: "read/review", "reply", "deep work", "@computer", "@home", "@office", "@errands"
- Users should be able to add/remove tags (except for special "projects" tag)
- Tags prefixed with "@" are treated as contexts and may receive special filtering/display treatment in views

### Lists
- We'll limit lists to an "in" list where all new tasks will be added by default, a "single actions" list for all actions that do not have more than one step, and "projects" list
- Users only see the "in" list, and would navigate "single actions" and "projects" list view Agenda views outlined below

### Projects
- The project list contains a toplevel task and subtasks nested under them to delineate the different projects
- Project toplevel tasks are just like other tasks in that they can have "state" or any of the optional parameters listed above
- Nesting supports up to two levels: Project → Sub-groups → Actions. This allows organizing large projects (e.g. "Launch Website" → "Design" → "Create mockups") without full tree complexity
- Sub-groups can exist with metadata but no actions, serving as milestones or placeholders for future planning. The weekly review will flag empty sub-groups to prompt the user to define next actions
- Each project is stored as its own Markdown file in a `projects/` directory, using a slugified title as the filename (e.g. `projects/launch-website.md`). If a collision occurs, a numeric suffix is appended (e.g. `launch-website-2.md`)

### Views
- We will introduce the concept of Views which gives users the ability to flexibly define filters that show aggregated tasks from across our lists, with the ability to filter based on any/all of our attributes
- Views can be pre-defined by users, or created ad hoc (and saved for later use if desired)
- Views should have two main visual toggles: 1/ task list views showing tasks and any other attributes the user configures (e.g. by tag) 2/ a daily/weekly "agenda" view showing scheduled and deadline information for each relevant task
- Tasks should be able to be updated within any view (any attribute or the text of the task itself)

#### Search
- Two search modes are available:
  1. **Structured query DSL** — filter by attribute (state, tag, deadline, waiting_on, etc.) as described in View Filtering
  2. **Fuzzy free-text search** — search across task names, notes, and all text content for quick lookups
- Both modes can be combined (e.g. fuzzy search within a filtered view)

#### View Filtering
- Views are defined using a text-based query DSL that users type inline (e.g. `state:next-action tag:@home`, `deadline:<2026-03-07`, `waiting_on:marvin`)
- The DSL supports attribute matching, boolean operators (AND/OR/NOT), and date comparisons
- Ad-hoc filters can be applied on the fly and optionally saved as named views

#### Saved Views
- Named views are persisted in the `~/.config/wnwn` config file
- Some default views ship out of the box (e.g. "Next Actions", "Waiting For", "This Week's Agenda", "Someday/Maybe")
- Users can create, edit, and delete saved views

### Tickler File
- The Tickler File is a special visual skin on top of the agenda view — it operates on the same underlying scheduled/deadline data, presented as a skeuomorphic 43-folder visualization (31 days + 12 months)
- Manila folder icons indicate which "folders" have items in them, providing an at-a-glance overview of upcoming work
- Filters (using the same query DSL as Views) apply to the tickler, so users can drill down into specific types of tasks
- Opening a "folder" shows the tasks for that day/month in a standard task list format

## 3. UX & Workflows

The TUI should be minimal but fun to engage with. Keyboard shortcuts should be used to navigate the app quickly.

We start the app with a view on the "in" list and an easy way to add tasks quickly (and also add deadline/scheduled/notes/url/tags) and "refile" them to either the "single actions" list or "projects" list.

### Process Inbox Mode
- A dedicated guided flow activated by a keyboard shortcut that walks through each "in" list item one at a time
- For each item, the user is presented with the GTD decision tree:
  1. Is it actionable?
     - **No** → Trash it (non-actionable items are not deferred; someday/maybe is for actionable-but-not-now)
     - **Yes** → Enrich the task (text, tags, deadline, schedule, notes), then route:
       - Takes < 2 minutes? → Do it now, then mark done
       - Delegate or blocked? → Set state to "waiting for", capture "waiting_on", refile
       - Defer? → Set state to "next action", assign to "single actions" or a project
       - Multi-step? → Create or assign to a project, define first next action
- Items are presented in FIFO order (oldest first, as GTD prescribes)
- The flow should feel quick and lightweight — minimal keystrokes to process each item

### Weekly Review Mode
- A dedicated guided flow activated by keyboard shortcut that walks the user through the GTD weekly review:
  1. Review each project — ensure every project has at least one "next action". Flag projects with no next actions
  2. Review "waiting for" items — surface how long each has been waiting, prompt to follow up or escalate
  3. Review "some day/maybe" items — prompt to activate (move to next actions/projects) or remove
  4. Review completed/canceled items — acknowledge progress, archive or clean up
  5. Trigger list — optionally present a configurable trigger list to help capture any open loops the user may have forgotten
- The review should track the last review date and surface a configurable reminder (e.g. "It's been 8 days since your last review")

### CLI Quick Capture
- Users can add tasks to the inbox from any terminal without launching the TUI: `wnwn add "buy milk"`
- Supports optional inline attributes: `wnwn add "buy milk" --deadline 2026-03-07 --tag @errands`
- Tasks added via CLI go directly to the "in" list for later processing

## 4. Data Format Specification

Runtime storage uses SQLite for speed and reliability, with Markdown import/export as a first-class interoperability format.

### Markdown Interchange Layout
- `in.md` — the inbox list
- `single-actions.md` — standalone next actions
- `projects/` — a directory containing one `.md` file per project
- `archive/archive.md` — archived tasks in a single export file
- Each task has a stable ULID (Universally Unique Lexicographically Sortable Identifier) embedded in its metadata, used for SQLite indexing and cross-references

### Archiving
- Marking a task `done` or `canceled` does **not** auto-archive by default; it remains in its current list/project until explicitly archived
- Users archive tasks via an explicit archive action in the UI/CLI
- Future configuration may allow enabling auto-archive behavior for users who prefer it
- Archived tasks include `archived_at` metadata (timestamp of explicit archive action) and are not grouped by month in runtime storage.
- Each archived task includes a `source` field in its metadata indicating where it came from (e.g. `source: single-actions`, `source: projects/launch-website`)
- Archived tasks are still searchable and visible in views via the query DSL
- The Weekly Review flow surfaces recently completed/canceled items from the archive for acknowledgment

### Markdown Format

#### Task Format
Tasks are represented as Markdown checkbox list items. Metadata is stored in a fenced YAML block indented under the task. Optional notes follow as indented Markdown prose.

**Checkbox state mapping:**
- `- [ ]` — open task (state is determined by the `state` field in metadata)
- `- [x]` — done
- `- [-]` — canceled

**Source of truth:** The `state` metadata field is the single source of truth. The checkbox is derived from the state and kept in sync by the application (`done` → `[x]`, `canceled` → `[-]`, all other states → `[ ]`). If a user hand-edits a file and the checkbox and state disagree, the `state` field wins and the checkbox will be corrected on next save.

**YAML quoting:** Tags prefixed with `@` (context tags) must be quoted in YAML because `@` is a reserved character. The application handles this automatically when writing files. Example: `tags: ["@computer", deep-work]`.

**Full task example:**
```markdown
- [ ] Book flights to/from Hawaii
  ```yaml
  id: 01JNQX7K9MZYZ3VDEHGR8N0P6F
  created: 2026-02-28T10:30
  state: next-action
  scheduled: 2026-03-10T09:00
  deadline: 2026-03-15T17:00
  url: https://flights.example.com
  tags: [travel, "@computer"]
  ```

  Check Southwest and United for direct flights from SFO.
  Compare prices for flexible dates around March 20-27.
```

**Minimal task (inbox item, no metadata yet):**
```markdown
- [ ] Buy milk
  ```yaml
  id: 01JNQX8B2KFGH4RSVW1MPAT9XY
  created: 2026-03-01T08:15
  ```
```

**Waiting-for task:**
```markdown
- [ ] Get quarterly report from Marvin
  ```yaml
  id: 01JNQXCF5NPQR7TWYZ2BDEV8MN
  created: 2026-02-18T14:00
  state: waiting-for
  waiting_on: Marvin
  waiting_since: 2026-02-20
  tags: ["@office", reports]
  ```
```

**Completed task:**
```markdown
- [x] Submit expense report
  ```yaml
  id: 01JNQXDJ8QRST0VWXY3CEFG1PQ
  created: 2026-02-15T09:00
  state: done
  tags: ["@office"]
  ```
```

#### List Files (in.md, single-actions.md)
List files have YAML frontmatter for file-level metadata, followed by tasks.

**Example `in.md`:**
```markdown
---
title: Inbox
type: in
---

- [ ] Look into new project management tools
  ```yaml
  id: 01JNQX7K9MZYZ3VDEHGR8N0P6F
  ```

- [ ] Schedule dentist appointment
  ```yaml
  id: 01JNQX8B2KFGH4RSVW1MPAT9XY
  tags: ["@errands"]
  ```

- [ ] Read article on distributed systems
  ```yaml
  id: 01JNQXCF5NPQR7TWYZ2BDEV8MN
  tags: [read/review]
  ```
```

**Example `single-actions.md`:**
```markdown
---
title: Single Actions
type: single-actions
---

- [ ] Buy new running shoes
  ```yaml
  id: 01JNQXHM4STUV6WXYZ7AFGH2RS
  state: next-action
  tags: ["@errands"]
  ```

- [ ] Book flights to/from Hawaii
  ```yaml
  id: 01JNQX7K9MZYZ3VDEHGR8N0P6F
  state: next-action
  deadline: 2026-03-15T17:00
  tags: [travel, "@computer"]
  ```

  Check Southwest and United for direct flights from SFO.
```

#### Project Files (projects/*.md)
Each project is its own file. Project-level metadata lives in YAML frontmatter. Sub-groups are `##` headings, each with an optional YAML block for sub-group metadata. Actions are task items under each sub-group.

**Example `projects/launch-website.md`:**
```markdown
---
title: Launch Website
id: 01JNQXKP7VWXY9ZAB0CDEF3GHI
state: active
deadline: 2026-06-01T00:00
tags: [project, launch, "@office"]
---

# Launch Website

## Design
```yaml
id: 01JNQXLQ8WXYZ0ABC1DEFG4HIJ
state: next-action
deadline: 2026-04-01T00:00
```

- [ ] Create wireframe mockups
  ```yaml
  id: 01JNQXMR9XYZA1BCD2EFGH5IJK
  state: next-action
  tags: ["@computer"]
  ```

- [ ] Review mockups with stakeholders
  ```yaml
  id: 01JNQXNS0YZAB2CDE3FGHI6JKL
  state: waiting-for
  waiting_on: Design Team
  waiting_since: 2026-03-01
  ```

## Development
```yaml
id: 01JNQXPT1ZABC3DEF4GHIJ7KLM
state: some-day/maybe
```

- [ ] Set up CI/CD pipeline
  ```yaml
  id: 01JNQXQU2ABCD4EFG5HIJK8LMN
  tags: ["@computer", deep-work]
  ```

- [ ] Implement responsive homepage
  ```yaml
  id: 01JNQXRV3BCDE5FGH6IJKL9MNO
  created: 2026-03-01T10:15
  tags: ["@computer", deep-work]
  ```

  Must support mobile, tablet, and desktop breakpoints.
  Reference the brand guidelines doc for color palette.

## Launch
```yaml
id: 01JNQXSW4CDEF6GHI7JKLM0NOP
deadline: 2026-06-01T00:00
```

- [ ] Write launch announcement
  ```yaml
  id: 01JNQXTX5DEFG7HIJ8KLMN1OPQ
  tags: ["@computer"]
  ```

- [ ] Coordinate with marketing on social posts
  ```yaml
  id: 01JNQXUY6EFGH8IJK9LMNO2PQR
  state: waiting-for
  waiting_on: Sarah
  waiting_since: 2026-02-25
  tags: ["@office"]
  ```
```

#### Metadata Field Reference

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | ULID string | Yes | Unique task identifier, auto-generated |
| `created` | datetime | Yes | Auto-set when task is created |
| `state` | enum string | No | Tasks: `next-action`, `waiting-for`, `some-day/maybe`, `done`, `canceled` (empty = unprocessed). Projects: `active`, `waiting-for`, `some-day/maybe`, `done`, `canceled` |
| `scheduled` | datetime | No | When the user intends to work on the task |
| `deadline` | datetime | No | When the task must be completed |
| `url` | string | No | Link to where the work happens |
| `tags` | list of strings | No | Context and category tags |
| `waiting_on` | string | No | Who or what the task/project is waiting on (person, system, event, or decision); used with `waiting-for` state |
| `waiting_since` | date | No | Auto-set when task enters `waiting-for` state |
| `source` | string | No | Auto-set when a task is archived; records origin (e.g. `single-actions`, `projects/launch-website`) |
| `notes` | — | No | Not a YAML field; written as indented Markdown prose below the YAML block |

## 5. Architecture & Tech Stack

We are building a cross-platform TUI (prioritize Mac/Linux) using [Bubbletea v2](https://charm.land/bubbletea/v2) and the following companion libraries from the Charm ecosystem:
- **[Lipgloss v2](https://charm.land/lipgloss/v2)** — styling, layout, and theming (powers the theme/color configuration)
- **[Bubbles v2](https://charm.land/bubbles/v2)** — reusable TUI components (text inputs, lists, viewports, spinners, tables)

Note: Bubbletea v2 uses `charm.land/*` import paths (not `github.com/charmbracelet/*`). The v2 API has breaking changes: `View()` returns `tea.View` (not string), key messages are `tea.KeyPressMsg` (not `tea.KeyMsg`). No third-party form/wizard library compatible with v2 exists yet — interactive flows (process inbox, weekly review) are implemented directly using Bubbletea state machines.

**Potential future additions (pending v2 compatibility):**
- **[Huh](https://github.com/charmbracelet/huh)** — form and wizard framework; would simplify task creation flows and the weekly review guided steps if a v2-compatible release ships
- **[Glamour](https://github.com/charmbracelet/glamour)** — terminal Markdown rendering; would enable rich display of task notes inline rather than plain text

## 6. Configuration

Users should be able to edit configuration via an app-specific `~/.config/wnwn/config.toml` file.

### Configuration Scope
The following are configurable in v1:
- **Keybindings** — remap keyboard shortcuts to user preference
- **Default tags** — configure which tags are seeded for new setups (contexts, action types, etc.)
- **Theme/colors** — customize the color scheme and visual style of the TUI
- **Data directory location** — choose where task files are stored (default: `~/.local/share/wnwn` or similar)
- **Saved views** — named view definitions with their query DSL filters
- **Review reminder settings** — configure weekly review reminder day and frequency

## 7. Future Enhancements

We may choose to add:
- "Areas" which are groupings of projects and may contain their own single task lists. This may be a visual distinction, a filesystem distinction, or a combination
- A mobile app, which syncs to desktop via Dropbox (not a top priority right now)
