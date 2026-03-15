{
  "id": "c5bc1cdf",
  "title": "Validate manual datetime text inputs",
  "tags": [
    "validation",
    "datetime",
    "task-detail"
  ],
  "status": "closed",
  "created_at": "2026-03-15T18:23:49.953Z"
}

Add explicit validation feedback for invalid date/time text entries in task detail editing.

## Acceptance Criteria

- Invalid datetime text no longer silently fails.
- User gets a clear status message explaining the invalid format.
- Existing datepicker-first flow remains unchanged.

## Related todos

- `TODO-a84f69a7` — clearing deadline/scheduled values.
- `TODO-af00d6d8` — stripping time while preserving date.

## Dedupe note

This item remains canonical for **manual text-entry validation** and is not a duplicate of the date-clearing/editing tasks.

## Resolution (2026-03-15)

Task detail manual date parsing now validates input with `parseDateTime` returning success/failure explicitly. Invalid text leaves edit mode active and shows a status message explaining the accepted formats (`YYYY-MM-DD` or `YYYY-MM-DD HH:MM`) instead of silently ignoring the bad value.
