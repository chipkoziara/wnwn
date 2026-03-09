{
  "id": "b7e4d8aa",
  "title": "Validate manual datetime text inputs",
  "tags": ["validation", "datetime", "task-detail"],
  "status": "open",
  "created_at": "2026-03-09T00:00:00.000Z",
  "assigned_to_session": null
}

Add explicit validation feedback for invalid date/time text entries in task detail editing.

## Acceptance Criteria

- Invalid datetime text no longer silently fails.
- User gets a clear status message explaining the invalid format.
- Existing datepicker-first flow remains unchanged.
