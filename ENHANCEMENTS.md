# Process Inbox

## Fixed

- ~~During the "Enrich" phase, when I add a note, I can't see what I'm typing (it works when I hit ENTER after typing--I just can't see anything)~~ — Fixed in commit `a17d773`. Enrich step now renders a proper field list with inline input display when editing, same pattern as task detail view.
- ~~During the enrich phase, after I submit a note, when I add another note it just appends. I should be editing the entered text.~~ — Fixed in `a17d773`. Input is pre-populated with the existing value so subsequent edits replace rather than append.
- ~~During the enrich phase, when I click `t` to edit task text, the newly entered text is also not visible, and it's appended similar to the notes feedback~~ — Fixed in `a17d773`. Same fix as the notes issue above.
- ~~If I choose a date from deadline or schedule WITHOUT a time, it shouldn't be a datetime. It should just be a date. Now it looks like I want to do things at midnight :)~~ — Fixed in `a17d773`. Date picker now tracks `hasTime` (true only when the user explicitly toggles time input). Date-only selections display without a time component. Midnight can still be set explicitly by toggling time and entering 00:00.
- ~~Shift Scheduled above Deadline when in Process Inbox (or any views)~~ — Fixed in `a17d773`. Scheduled now appears before Deadline in both the task detail view and the process inbox enrich step.
- ~~We should add someday/maybe as another option during the enrich / routing flow.~~ — Fixed in `a17d773`. Someday/maybe `[s]` is now a first-class option in the route step. Not-actionable step is now trash-only (GTD-correct: non-actionables get trashed, not deferred; someday/maybe is for actionable-but-not-now items).

## Deferred (tracked in STATUS.md)

- If I errantly mark something as "done" or refile to an incorrect location, how should we introduce better steering? — Deferred. Recommendation: add a "Recently Modified" saved view once the Views/Query DSL feature is built. See STATUS.md item #5.
- Should I append timezones to my datetimes? — Decision: No. Naive local time is correct for a personal GTD app. See STATUS.md item #6.
- I can type invalid datetimes... not a huge issue — Deferred, low priority. Date picker prevents most invalid input. See STATUS.md item #7.
- Creating new project workflow needs more work (rename, definition of done, URL field) — Deferred as a dedicated "Project Editing" enhancement pass. See STATUS.md item #8.

# Task Edits
- ~~When setting a task as "waiting-for" can we light up the "delegated to" field visually so users know to enter something there? Don't make it required~~ — Fixed. When state is `waiting-for` and `delegated_to` is empty, the entire row renders in amber (`waitingNudgeStyle`) with the prompt "— who is this waiting on?". Disappears once a value is entered or the state changes.
- Consider the order of task editing / which fields appear where (e.g. should notes be higher / lower)

# General Edits
- Review default keybindings and see if there are inconsistencies and ways I can improve with better, more consistent mnemonics
- Add setting keybindings by config.toml (set defaults in the file?)
- Improve UX generally with more fun injected :)

# Config file
- Update where text files are stored (set the directory)
