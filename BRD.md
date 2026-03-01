# GTD

We are building a TUI GTD app, with several key distinctions over other software in this space.

- Simplicity → built with a clear focus with no extra frills in the core
- Configurability → users can adjust key aspects of the software to tailor it to their workflows easily
- Quirkiness and fun → productivity software is often boring. our software should be a joy to use

# Methodology
We are using the GTD methodology as outlined in this website: https://hamberg.no/gtd

Our application will have a few distinctions to make the approach more modern and modular:
- The "state" of a given task or project (which is built on the same task primitive and has subtasks) can be empty, "next action", "waiting for", "some day/maybe", "done", or "canceled"
- All tasks can have additional optional attributes: "scheduled", "deadline", "notes", "url", and "tags"
  - "scheduled" must be a datetime, and it denotes when a user intends to work on that task
  - "deadline" must be a datetime, and it denotes when a user must complete that task
  - "notes" is a paragraph text area that can contain a description of the task that the user wants to log for future reference
  - "url" is a string that users can utilize to point to where the work needs to be done (e.g. a link to an email thread in Gmail, a Slack conversation, a website, etc.)
  - "tags" are a list of strings that users can use to add more context to a given task, and these can be searched / filtered (described later)
  - Projects are auto-assigned a tag called "project", but other tags can be added to them
- We'll limit lists to an "in" list where all new tasks will be added by default, a "single actions" list for all actions that do not have more than one step, and "projects" list. The project list contains a toplevel task and subtasks nested under them to delineate the different projects.
- Project toplevel tasks are just like other tasks in that they can have "state" or any of the optional parameters listed above
- We will introduce the concept of Agenda views which gives users the ability to flexibility define filters that show aggregated tasks from across our lists, with the ability to filter based on any/all of our attributes
- Agenda views can be pre-defined by users, or created ad hoc (and saved for later use if desired)
- Agenda views should have two main visual toggles: 1/ task lists showing tasks and any other attributes the user configures 2/ a daily/weekly/monthly view

# Architecture
We are building a TUI using Bubbletea. Recommend which additional libraries may make sense to include to make the UX more pleasant, fun, and engaging for our users.
w
We also adhere to "File Over App" where the tasks and projects should all be store in plain text files (Markdown). We may choose to augment the markdown files with a Sqlite layer to improve speed, reliability, and unlock better workflows (e.g. faster Agenda views).

# Future Enhancements
We may choose to add:
- "Areas" which are groupings of projects and may contain their own single task lists. This may be a visual distinction, a filesystem distinction, or a combination
- A way to add tasks from outside our system, straight to our "in" list
