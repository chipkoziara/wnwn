# Weekly Review Fixture Pack

Purpose: provide a deterministic dataset that exercises each Weekly Review section.

Import with replace:

```bash
./wnwn import-md --from ./testdata/weekly-review --replace
```

Open weekly review from Views tab:

1. Press `4` (Views)
2. Press `W`

Expected behavior:

- **Projects Missing Next Action**: shows `Client Onboarding` and `Office Move`
- **Aging Waiting For (7+ days)**: shows tasks waiting since early 2025
- **Someday / Maybe**: shows several deferred tasks from lists/projects
- **Recently Archived (7 days)**: shows one synthetic archived item with a far-future `archived_at` timestamp (intentional for stable demos)

Notes:

- This fixture intentionally uses historical waiting dates so the aging waiting-for bucket is always populated.
- The far-future archived timestamp is a demo/testing convenience to keep "recent archived" non-empty over time.
