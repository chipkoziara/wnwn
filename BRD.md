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
- All tasks can have additional optional attributes: "scheduled", "deadline", "notes", "url", and "tags"
  - "scheduled" must be a datetime, and it denotes when a user intends to work on that task
  - "deadline" must be a datetime, and it denotes when a user must complete that task
  - "notes" is a paragraph text area that can contain a description of the task that the user wants to log for future reference
  - "url" is a string that users can utilize to point to where the work needs to be done (e.g. a link to an email thread in Gmail, a Slack conversation, a website, etc.)
  - "tags" are a list of strings that users can use to add more context to a given task, and these can be searched / filtered (described later)
  - Projects are auto-assigned a tag called "project", but other tags can be added to them

### Tags
- Additional tags that should be available for users to configure by default include "read/review", "reply", and "deep work"
- Users should be able to add/remove tags (except for special "projects" tag)

## Lists
- We'll limit lists to an "in" list where all new tasks will be added by default, a "single actions" list for all actions that do not have more than one step, and "projects" list
- Users only see the "in" list, and would navigate "single actions" and "projects" list view Agenda views outlined below

## Projects
- The project list contains a toplevel task and subtasks nested under them to delineate the different projects
- Project toplevel tasks are just like other tasks in that they can have "state" or any of the optional parameters listed above

## Views
- We will introduce the concept of Views which gives users the ability to flexibility define filters that show aggregated tasks from across our lists, with the ability to filter based on any/all of our attributes
- Views can be pre-defined by users, or created ad hoc (and saved for later use if desired)
- Views should have two main visual toggles: 1/ task list views showing tasks and any other attributes the user configures (e.g. by tag) 2/ a daily/weekly "agenda" view showing scheduled a deadline information for each relevant task
- Tasks should be able to be updated within any Agenda view (any attribute or the text of the task itself)

## Tickler File
This is an area we can continue to develop, but I want this to be a special agenda view that visualizes a skeumorphic 43 folders view that shows scheduled/deadline items across this format with the manilla folders visualization and an indicator for which virtual "folders" have items in them
- Filters should apply to this view, too, so users can drill down into specific types of views

# Architecture Guidance
We are building a cross-platform TUI (prioritize Mac/Linux) using [Bubbletea](https://github.com/charmbracelet/bubbletea). Recommend which additional libraries may make sense to include to make the UX more pleasant, fun, and engaging for our users.

We also adhere to "File Over App" where the tasks and projects should all be store in plain text files (Markdown). We may choose to augment the markdown files with a Sqlite layer to improve speed, reliability, and unlock better workflows (e.g. faster views).

Users should be able to edit configuration via an app-specific ~/.config/gtd config file (format TBD).

The TUI should be minimal but fun to engage with. Keyboard shortcuts should be used to navigate the app quickly.

We start the app with a view on the "in" list and an easy way to add tasks quickly (and also add deadline/scheduled/notes/url/tags) and "refile" them to either the "single actions" list or "projects" list.

# Future Enhancements
We may choose to add:
- "Areas" which are groupings of projects and may contain their own single task lists. This may be a visual distinction, a filesystem distinction, or a combination
- A way to add tasks from outside our system, straight to our "in" list
- A mobile app, which syncs to desktop via Dropbox (not a top priority right now)
