# Architecture Milestone Summary — 2026-03-22

This note captures the architecture milestone reached during the `internal/core` extraction work on 2026-03-22.

## Summary

`internal/core` has now become the primary **write-path boundary** for wnwn.

The codebase started this phase with a first-pass core facade and a TUI still heavily dependent on the legacy location-aware service methods. By the end of this milestone, the CLI and nearly all meaningful TUI mutation flows now route through stable-ID core APIs instead of calling filename/list/subgroup-index-based mutation methods directly.

This was a successful **strangler-style extraction milestone**, not just incremental cleanup.

## What the Core Boundary Now Covers

The core now exposes working APIs for:

- inbox capture
- task update / archive / trash / restore
- task move / refile between lists and projects
- project lookup / update / create
- subgroup create / rename / delete
- project task add / move / reorder
- Process Inbox session lifecycle
- Process Inbox draft mutation
- Process Inbox typed final decisions
- transitional trash-undo restore bridge

## What Moved Out of Direct TUI → Legacy Mutation Calls

The TUI now routes through `internal/core` for nearly all meaningful mutation surfaces, including:

- inbox/actions/list task mutations
- task detail save
- project-detail structure operations
- project-detail task mutations
- project task detail save
- project metadata edit save
- refile / move flows
- view-results and weekly-review mutation handlers
- project reorder
- inbox add
- project creation
- Process Inbox movement semantics
- Process Inbox session lifecycle
- Process Inbox draft edits
- Process Inbox final typed decisions

## Process Inbox Status

Process Inbox is now core-backed for:

- session lifecycle / progress
- draft truth
- final decision semantics
- summary / progress ownership

The TUI still owns:

- rendering
- input / focus behavior
- step-navigation UI mechanics

This is considered a good practical stopping point for the workflow extraction. The core now owns the actual workflow truth and mutations, while Bubble Tea remains responsible for presentation.

## What Still Remains

The remaining TUI direct use of the legacy service is now mostly **read-side** or **presentation-side**, such as:

- project list loading
- weekly review loading
- project detail / project edit loading
- aggregate task collection for views
- Process Inbox step-navigation / presentation ownership

These remaining areas are no longer the high-risk mutation-coupling problem that motivated the extraction.

## Why This Matters

The architectural shift is now real:

- the **write path** is mostly behind an ID-first core boundary
- the TUI is far less coupled to filenames, subgroup indexes, and location-aware mutation routing
- future clients have a much more credible core/service boundary to target
- remaining work is largely consistency cleanup, not foundational rescue

## Outcome

This milestone should be treated as the point where the `internal/core` extraction became operationally successful for write paths.

From here, future work can be chosen intentionally:

- continue with read-side consistency cleanup
- further simplify Process Inbox presentation state
- or pause architecture work and let normal product work proceed against the new boundary

## Related Files

- `docs/wnwn-architecture-plan.md`
- `docs/process-inbox-extraction-prep.md`
- `STATUS.md`
- `internal/core/core.go`
- `internal/core/core_test.go`
