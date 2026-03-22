# Process Inbox Extraction Prep

Date: 2026-03-22

This note scopes the next architecture step after the recent `internal/core` move/refile migration slices. It does **not** implement the Process Inbox extraction yet. Its purpose is to inventory what the TUI currently owns, identify what belongs at the core boundary, and propose the smallest viable core-owned session API.

## Current State

Process Inbox still lives primarily in `internal/tui/app.go` as a TUI-owned workflow with a local working copy:

- TUI loads a snapshot of inbox tasks into `processItems`
- TUI tracks progress via `processIdx`, `processStep`, and `processStats`
- TUI holds the mutable current draft in `processTask`
- TUI owns all decision-tree navigation and field editing interactions
- Final actions persist changes by calling service/core methods from the TUI

Recent migration progress changed **movement semantics**, but not workflow ownership:

- Process Inbox route actions that move tasks now use core move APIs after persisting the working copy back to inbox
- Process Inbox itself is still not a core-owned session

## Inventory of Current TUI-Owned Responsibilities

### 1. Session lifecycle / progress

Currently TUI-owned state:

- `processItems []model.Task` — snapshot of inbox tasks at activation
- `processIdx int` — current item index
- `processStep processStep` — current decision-tree step
- `processStats processStats` — completion summary counts
- `advanceProcessInbox()` — next-item / completion transition
- `loadInboxForProcessing()` — snapshot load from inbox

Assessment:
- **Core-worthy**: inbox snapshot ownership, current item selection, skip/advance semantics, completion/progress accounting
- **TUI-worthy**: rendering the progress tab label, focus placement after transitions

### 2. Draft / working-copy ownership

Currently TUI-owned state:

- `processTask model.Task` — mutable current draft
- `processTags []string` — temporary tag editing buffer
- date picker and field-edit writeback into `processTask`
- escape/back behavior that reverts to the original item snapshot

Assessment:
- **Core-worthy**: current draft state as the source of truth for the workflow session
- **TUI-worthy**: text input widgets, tag entry UX, date picker controls, cursor/focus behavior

Important note:
- The architecture plan already decided that Process Inbox drafts should be **ephemeral and core-owned**, not persisted until final action commit.
- That means the draft should move out of TUI model fields and into a session object managed by core.

### 3. Decision tree / workflow transitions

Currently TUI-owned step handlers:

- `updateProcessStepActionable`
- `updateProcessStepNotActionable`
- `updateProcessStepEnrich`
- `updateProcessStepEnrichTags`
- `updateProcessStepRoute`
- `updateProcessStepWaitingOn`
- `updateProcessStepNewProject`

Assessment:
- **Core-worthy**:
  - which step is active
  - which actions are valid at each step
  - whether a transition is allowed
  - whether required fields are present before a final commit action
  - skip/advance/completion semantics
- **TUI-worthy**:
  - mapping keys to actions
  - inline edit mode transitions
  - input focus behavior
  - screen layout for each step

### 4. Final commit behavior

Currently TUI-owned orchestration:

- persist draft to inbox first via `svc.UpdateTask(model.ListIn, task)`
- then perform final routing/mutation:
  - done via `svc.UpdateState(...)`
  - trash via `svc.TrashTask(...)`
  - move to single-actions/project via core move APIs
  - create project, then move into it
- then emit `processAdvancedMsg` for bookkeeping

Assessment:
- **Core-worthy**:
  - final commit orchestration should become one atomic-ish session action at the boundary
  - core should own the mapping from typed decision to mutations
  - core should own project-creation-plus-refile behavior for Process Inbox
- **TUI-worthy**:
  - showing success/error messages
  - rendering completion summary

### 5. Completion summary

Currently TUI-owned:

- `processStats` counters
- completion rendering from those counters

Assessment:
- **Core-worthy**: session summary/progress counts
- **TUI-worthy**: presentation of the summary

## Recommended Boundary Split

## Keep in the TUI

These should remain presentation concerns:

- keybindings and key-to-action mapping
- which Bubble Tea mode/input widget is active
- text input, tag-entry UX, date picker interactions
- cursor position, focus, and visual back/escape behavior
- rendering of the decision tree and completion screen

## Move into core

These should become session/domain concerns:

- loading the inbox snapshot for processing
- owning the current draft for the current item
- deciding the current workflow step
- validating allowed transitions/actions
- applying draft field updates to the session draft
- skipping the current item
- committing the current item via typed final actions
- creating new projects as part of a Process Inbox route
- maintaining progress / completion counters
- exposing completion state and summary

## Smallest Viable Core Session API

The goal is not a giant workflow engine. The goal is the smallest API that lets the TUI become a dumb presenter over a core-owned Process Inbox session.

Proposed shape:

```go
type InboxSessionService interface {
    StartInboxSession() (*InboxSession, error)
}

type InboxSession struct {
    ID       string
    Current  InboxSessionItem
    Progress InboxSessionProgress
    Summary  InboxSessionSummary
    Done     bool
}

type InboxSessionItem struct {
    Original model.Task
    Draft    model.Task
    Step     InboxStep
}

type InboxStep string

const (
    InboxStepActionable InboxStep = "actionable"
    InboxStepNotActionable InboxStep = "not_actionable"
    InboxStepEnrich InboxStep = "enrich"
    InboxStepRoute InboxStep = "route"
    InboxStepWaitingOn InboxStep = "waiting_on"
    InboxStepNewProject InboxStep = "new_project"
    InboxStepComplete InboxStep = "complete"
)
```

Then typed session methods, conceptually:

```go
GetInboxSession(sessionID string) (*InboxSession, error)
UpdateInboxDraft(sessionID string, patch TaskPatch) (*InboxSession, error)
SetInboxStep(sessionID string, action InboxStepAction) (*InboxSession, error)
SkipInboxItem(sessionID string) (*InboxSession, error)
CommitInboxDecision(sessionID string, decision InboxDecision) (*InboxSession, error)
DiscardInboxSession(sessionID string) error
```

Where `InboxDecision` is explicit and typed, for example:

```go
type InboxDecision struct {
    Kind InboxDecisionKind

    // Optional payload depending on Kind.
    WaitingOn  string
    ProjectID  string
    ProjectTitle string
}

type InboxDecisionKind string

const (
    InboxDecisionTrash InboxDecisionKind = "trash"
    InboxDecisionDone InboxDecisionKind = "done"
    InboxDecisionSomeday InboxDecisionKind = "someday"
    InboxDecisionWaiting InboxDecisionKind = "waiting"
    InboxDecisionSingleAction InboxDecisionKind = "single_action"
    InboxDecisionProject InboxDecisionKind = "project"
    InboxDecisionNewProject InboxDecisionKind = "new_project"
)
```

This does **not** require the TUI to stop owning keybindings or field widgets. It only changes who owns workflow truth.

## Recommended First Implementation Slice

Do **not** implement the full session engine in one jump.

Instead, take the smallest first slice:

### Slice A: define the contract and in-memory session scaffolding

1. Add Process Inbox session types to `internal/core`
2. Add a `StartInboxSession()` API
3. Add an in-memory session registry owned by core
4. Load inbox snapshot into the session
5. Expose current item, step, progress, and summary
6. Add `SkipInboxItem()` and `DiscardInboxSession()`

This slice proves session ownership without taking on draft mutation or final commits yet.

### Slice B: move draft ownership to core

1. Add `UpdateInboxDraft(sessionID, TaskPatch)`
2. Move `processTask` truth into core session state
3. TUI becomes a renderer/editor over the session draft
4. Keep final commit actions temporary if needed

### Slice C: move final typed decisions into core

1. Add `CommitInboxDecision(...)`
2. Move done/trash/someday/waiting/refile/new-project routing into core-owned typed actions
3. Remove Process Inbox-specific movement/update orchestration from the TUI

## Acceptance Criteria for Prep Completion

This prep is complete if the next implementation session can answer these clearly:

1. What Process Inbox state stays TUI-only?
2. What Process Inbox state moves to core?
3. What is the minimum core API to start the extraction?
4. What is the first implementation slice that reduces coupling without requiring a rewrite?

This document answers those questions with the recommendation:

- TUI keeps presentation and widget mechanics
- core takes session truth, draft truth, transitions, and final commit orchestration
- first implementation slice should be **session lifecycle scaffolding**, not the whole workflow engine at once
