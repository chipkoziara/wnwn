{
  "id": "d42dcd2b",
  "title": "Add jump-to-source context navigation for tasks",
  "tags": [
    "tui",
    "navigation",
    "views",
    "projects",
    "ux"
  ],
  "status": "open",
  "created_at": "2026-03-15T19:07:00.840Z"
}

Allow users to jump from aggregated task views into the task's native context (inbox, single actions, or project detail) with the task highlighted.

## Acceptance Criteria

- From aggregated contexts such as view results and weekly review, add a command to open the task in its source list/project instead of only opening the task detail editor.
- If the task belongs to a project, open project detail with the task visible and highlighted.
- If the task belongs to inbox or single actions, return to that list with the task highlighted.
- Preserve a smooth return path so users can inspect context without losing their place unnecessarily.
- Document the keybinding and navigation behavior in STATUS.md.

## Notes

This would make aggregated views much more actionable by letting users inspect surrounding context before editing or changing state.

## Exploration note

Evaluate whether jump-to-source navigation should include a dedicated "go back" mechanism that returns the user to the originating aggregated context (for example view results or weekly review) at the same cursor position. This should be considered as part of the navigation design, not assumed automatically.
