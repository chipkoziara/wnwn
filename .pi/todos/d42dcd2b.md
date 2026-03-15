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

## Recommended order of operations (2026-03-15)

Suggested implementation sequence for a future session:

1. **Phase 1: jump from aggregated views into source context**
   - Start with View Results and Weekly Review only.
   - Add a dedicated jump-to-source command (separate from task detail editing).
   - Route to inbox, single-actions, or project detail based on provenance.

2. **Phase 2: highlight the destination task in context**
   - When landing in inbox or single-actions, move the list cursor to the task.
   - When landing in project detail, open the project and position/highlight the matching task row.

3. **Phase 3: evaluate return-path design**
   - Decide whether a dedicated “go back” mechanism is needed.
   - If implemented, preserve originating context (for example View Results or Weekly Review) and cursor position.
   - Keep this as an explicit design choice rather than assuming it automatically.

4. **Phase 4: docs / keybinding polish**
   - Document keybindings and navigation behavior in STATUS.md.
   - Reassess mnemonic consistency with the broader navigation/keybinding scheme.

## Recommendation

Treat Phases 1 and 2 as the minimum shippable slice. Phase 3 (go back) should be explored deliberately after the basic jump-and-highlight flow works well.
