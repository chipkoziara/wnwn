# GTD

We are building a TUI GTD app, with several key distinctions over other software in this space.

- Simplicity → built with a clear focus with no extra frills in the core
- Configurability → users can adjust key aspects of the software to tailor it to their workflows easily
- Quirkiness and fun → productivity software is often boring. our software should be a joy to use

# Methodology
We are using the GTD methodology as outlined in this website: https://hamberg.no/gtd

Our application will have a few distinctions to make the approach more modern and modular.

## Task Attributes
- The task as written is itself an attribute (attribute: "task", e.g. "book flights to/from Hawaii")
- The "state" of a given task or project (which is built on the same task primitive and has subtasks) can be empty, "next action", "waiting for", "some day/maybe", "read/review", "done", or "canceled"
- All tasks can have additional optional attributes: "scheduled", "deadline", "notes", "url", "tags", and "delegated_to"
  - "scheduled" must be a datetime, and it denotes when a user intends to work on that task
  - "deadline" must be a datetime, and it denotes when a user must complete that task
  - "notes" is a paragraph text area that can contain a description of the task that the user wants to log for future reference
  - "url" is a string that users can utilize to point to where the work needs to be done (e.g. a link to an email thread in Gmail, a Slack conversation, a website, etc.)
  - "tags" are a list of strings that users can use to add more context to a given task, and these can be searched / filtered (described later)
  - "delegated_to" is an optional string capturing who a task has been delegated to; primarily used when state is "waiting for"
  - When a task transitions to "waiting for" state, the date of that transition is automatically recorded so users can track how long they've been waiting
  - Projects are auto-assigned a tag called "project", but other tags can be added to them

### Tags
- Tags serve double duty as GTD "contexts" — users can create tags like "@home", "@computer", "@office", "@errands" to filter next actions by location or available tools
- Default tags seeded for new users: "read/review", "reply", "deep work", "@computer", "@home", "@office", "@errands"
- Users should be able to add/remove tags (except for special "projects" tag)
- Tags prefixed with "@" are treated as contexts and may receive special filtering/display treatment in views

## Lists
- We'll limit lists to an "in" list where all new tasks will be added by default, a "single actions" list for all actions that do not have more than one step, and "projects" list
- Users only see the "in" list, and would navigate "single actions" and "projects" list view Agenda views outlined below

## Projects
- The project list contains a toplevel task and subtasks nested under them to delineate the different projects
- Project toplevel tasks are just like other tasks in that they can have "state" or any of the optional parameters listed above
- Nesting supports up to two levels: Project → Sub-groups → Actions. This allows organizing large projects (e.g. "Launch Website" → "Design" → "Create mockups") without full tree complexity
- Each project is stored as its own Markdown file in a `projects/` directory

## Views
- We will introduce the concept of Views which gives users the ability to flexibly define filters that show aggregated tasks from across our lists, with the ability to filter based on any/all of our attributes
- Views can be pre-defined by users, or created ad hoc (and saved for later use if desired)
- Views should have two main visual toggles: 1/ task list views showing tasks and any other attributes the user configures (e.g. by tag) 2/ a daily/weekly "agenda" view showing scheduled and deadline information for each relevant task
- Tasks should be able to be updated within any view (any attribute or the text of the task itself)

### Search
- Two search modes are available:
  1. **Structured query DSL** — filter by attribute (state, tag, deadline, delegated_to, etc.) as described in View Filtering
  2. **Fuzzy free-text search** — search across task names, notes, and all text content for quick lookups
- Both modes can be combined (e.g. fuzzy search within a filtered view)

### View Filtering
- Views are defined using a text-based query DSL that users type inline (e.g. `state:next-action tag:@home`, `deadline:<2026-03-07`, `delegated_to:marvin`)
- The DSL supports attribute matching, boolean operators (AND/OR/NOT), and date comparisons
- Ad-hoc filters can be applied on the fly and optionally saved as named views

### Saved Views
- Named views are persisted in the `~/.config/gtd` config file
- Some default views ship out of the box (e.g. "Next Actions", "Waiting For", "This Week's Agenda", "Someday/Maybe")
- Users can create, edit, and delete saved views

## Tickler File
- The Tickler File is a special visual skin on top of the agenda view — it operates on the same underlying scheduled/deadline data, presented as a skeuomorphic 43-folder visualization (31 days + 12 months)
- Manila folder icons indicate which "folders" have items in them, providing an at-a-glance overview of upcoming work
- Filters (using the same query DSL as Views) apply to the tickler, so users can drill down into specific types of tasks
- Opening a "folder" shows the tasks for that day/month in a standard task list format

# Architecture Guidance
We are building a cross-platform TUI (prioritize Mac/Linux) using [Bubbletea](https://github.com/charmbracelet/bubbletea) and the following companion libraries from the Charm ecosystem:
- **[Lipgloss](https://github.com/charmbracelet/lipgloss)** — styling, layout, and theming (powers the theme/color configuration)
- **[Bubbles](https://github.com/charmbracelet/bubbles)** — reusable TUI components (text inputs, lists, viewports, spinners, tables)
- **[Huh](https://github.com/charmbracelet/huh)** — form and wizard framework (powers task creation, process inbox mode, and weekly review flows)
- **[Glamour](https://github.com/charmbracelet/glamour)** — terminal markdown rendering (for displaying task notes inline)

We also adhere to "File Over App" where the tasks and projects should all be store in plain text files (Markdown). We may choose to augment the markdown files with a Sqlite layer to improve speed, reliability, and unlock better workflows (e.g. faster views).

### File Layout
- `in.md` — the inbox list
- `single-actions.md` — standalone next actions
- `projects/` — a directory containing one `.md` file per project
- Each task needs a stable unique identifier (e.g. a short hash or ULID) embedded in the Markdown, used for SQLite indexing and cross-references

Users should be able to edit configuration via an app-specific ~/.config/gtd config file (format TBD).

### Configuration Scope
The following are configurable in v1:
- **Keybindings** — remap keyboard shortcuts to user preference
- **Default tags** — configure which tags are seeded for new setups (contexts, action types, etc.)
- **Theme/colors** — customize the color scheme and visual style of the TUI
- **Data directory location** — choose where task files are stored (default: `~/.local/share/gtd` or similar)
- **Saved views** — named view definitions with their query DSL filters
- **Review reminder settings** — configure weekly review reminder day and frequency

The TUI should be minimal but fun to engage with. Keyboard shortcuts should be used to navigate the app quickly.

We start the app with a view on the "in" list and an easy way to add tasks quickly (and also add deadline/scheduled/notes/url/tags) and "refile" them to either the "single actions" list or "projects" list.

## Process Inbox Mode
- A dedicated guided flow activated by a keyboard shortcut that walks through each "in" list item one at a time
- For each item, the user is presented with the GTD decision tree:
  1. Is it actionable?
     - **No** → Trash it, save as reference (notes), or move to "some day/maybe"
     - **Yes** → What's the next action?
       - Takes < 2 minutes? → Do it now, then mark done
       - Delegate? → Set state to "waiting for", capture "delegated_to", refile
       - Defer? → Set state to "next action", assign to "single actions" or a project
       - Multi-step? → Create or assign to a project, define first next action
- Items are presented in FIFO order (oldest first, as GTD prescribes)
- The flow should feel quick and lightweight — minimal keystrokes to process each item

## Weekly Review Mode
- A dedicated guided flow activated by keyboard shortcut that walks the user through the GTD weekly review:
  1. Review each project — ensure every project has at least one "next action". Flag projects with no next actions
  2. Review "waiting for" items — surface how long each has been waiting, prompt to follow up or escalate
  3. Review "some day/maybe" items — prompt to activate (move to next actions/projects) or remove
  4. Review completed/canceled items — acknowledge progress, archive or clean up
  5. Trigger list — optionally present a configurable trigger list to help capture any open loops the user may have forgotten
- The review should track the last review date and surface a configurable reminder (e.g. "It's been 8 days since your last review")

## CLI Quick Capture
- Users can add tasks to the inbox from any terminal without launching the TUI: `gtd add "buy milk"`
- Supports optional inline attributes: `gtd add "buy milk" --deadline 2026-03-07 --tag @errands`
- Tasks added via CLI go directly to the "in" list for later processing

# Future Enhancements
We may choose to add:
- "Areas" which are groupings of projects and may contain their own single task lists. This may be a visual distinction, a filesystem distinction, or a combination
- A way to add tasks from outside our system, straight to our "in" list
- A mobile app, which syncs to desktop via Dropbox (not a top priority right now)
