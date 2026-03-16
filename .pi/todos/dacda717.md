{
  "id": "dacda717",
  "title": "Prepare wnwn for a public release",
  "tags": [
    "release",
    "docs",
    "packaging",
    "polish"
  ],
  "status": "open",
  "created_at": "2026-03-15T19:49:35.177Z",
  "assigned_to_session": "254cefb0-f1ad-4a8e-ac80-99ee2392965f"
}

Plan and execute the work needed to make wnwn ready for a public release.

## Acceptance Criteria

- Define the minimum release checklist (stability, docs, onboarding, packaging, licensing, contribution guidance, and known limitations).
- Review CLI/TUI ergonomics and ensure the README reflects the current feature set accurately.
- Identify any blockers for first public release (for example discoverability, module path/repo alignment, install story, demo data/screenshots, or release process).
- Decide what version boundary constitutes a sensible first public release.
- Document the release plan and outstanding blockers clearly.

## Notes

This is intentionally broader than any single feature. It should help decide when the app is ready to move from active prototyping into a public-facing release posture.

## Progress

Created `docs/release-plan.md` with:
- proposed first public version boundary (`v0.1.0`)
- release readiness checklist
- blockers and non-blockers
- concrete next actions

Key blockers identified so far:
- `README.md` is stale relative to the current feature set (Views tab, fuzzy search, weekly review, roadmap, test count, install URL)
- `RELEASE_NOTES_v0.1.0.md` still claims fuzzy search is missing
- install/distribution story needs an explicit decision for public release

## Release tracking subtasks created
- TODO-08d184f5 — Refresh README for public release accuracy
- TODO-c3c92dfa — Add release-focused install, quick-start, and limitations docs
- TODO-7a2f7a5a — Refresh v0.1.0 release notes
- TODO-b2af336a — Decide v0.1.0 distribution model
- TODO-77cf1f4f — Document reproducible release build steps
- TODO-3284364e — Run release hardening checks
- TODO-be2c95dd — Create public demo asset for wnwn
- TODO-9c6c854c — Add sample/demo data or reproducible demo flow
- TODO-16b74b8d — Confirm repo/module/install metadata for public release
- TODO-04af1bda — Define v0.1.0 blockers vs acceptable rough edges
