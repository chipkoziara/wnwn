package core

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chipkoziara/wnwn/internal/model"
	"github.com/chipkoziara/wnwn/internal/query"
	"github.com/chipkoziara/wnwn/internal/service"
	"github.com/chipkoziara/wnwn/internal/store"
)

// Service is the top-level client-facing core boundary.
type Service interface {
	TaskService
	ProjectService
	InboxSessionService
	InboxService
	ViewService
	ReviewService
	ImportExportService
}

type TaskService interface {
	CaptureToInbox(text string, opts CaptureOpts) (model.Task, error)
	UpdateTask(taskID string, patch TaskPatch) (TaskLocation, error)
	ArchiveTask(taskID string) error
	RestoreTask(taskID string) (TaskLocation, error)
	TrashTask(taskID string) error
	RestoreTrashedTask(task model.Task, source string) (TaskLocation, error)
	MoveTaskToList(taskID string, toList model.ListType, newState model.TaskState) (TaskLocation, error)
	MoveTaskToProject(taskID string, projectID, subgroupID string, newState model.TaskState) (TaskLocation, error)
}
type ProjectService interface {
	GetProject(projectID string) (ProjectLocation, error)
	UpdateProject(projectID string, patch ProjectPatch) (ProjectLocation, error)
	QueryProjects(input QueryInput) ([]ProjectLocation, error)
	CreateSubgroup(projectID, title string) (SubgroupLocation, error)
	RenameSubgroup(projectID, subgroupID, title string) (SubgroupLocation, error)
	DeleteSubgroup(projectID, subgroupID string) error
	AddProjectTask(projectID, subgroupID, text string, opts CaptureOpts) (TaskLocation, error)
	MoveTaskToSubgroup(taskID, subgroupID string) error
	ReorderProjectTask(taskID string, delta int) error
}
type InboxSessionService interface {
	StartInboxSession() (*InboxSession, error)
	GetInboxSession(sessionID string) (*InboxSession, error)
	UpdateInboxDraft(sessionID string, patch TaskPatch) (*InboxSession, error)
	CommitInboxDecision(sessionID string, decision InboxDecision) (*InboxSession, error)
	SkipInboxItem(sessionID string) (*InboxSession, error)
	DiscardInboxSession(sessionID string) error
}

type InboxService interface{}
type ViewService interface {
	ListViews() []model.SavedView
	RunView(name string) ([]service.ViewTask, error)
	RunQuery(input QueryInput) ([]service.ViewTask, error)
}
type ReviewService interface {
	WeeklyReview(now time.Time) (service.WeeklyReviewData, error)
}
type ImportExportService interface {
	ImportMarkdown(input ImportInput) (ImportResult, error)
	ExportMarkdown(outputDir string) error
}

// CoreConfig contains behavior-affecting settings passed into the core.
type CoreConfig struct {
	AutoArchiveDone     bool
	AutoArchiveCanceled bool
	SavedViews          []model.SavedView
}

// InboxStep is the current step in a Process Inbox session.
type InboxStep string

const (
	InboxStepActionable    InboxStep = "actionable"
	InboxStepNotActionable InboxStep = "not_actionable"
	InboxStepEnrich        InboxStep = "enrich"
	InboxStepRoute         InboxStep = "route"
	InboxStepWaitingOn     InboxStep = "waiting_on"
	InboxStepNewProject    InboxStep = "new_project"
	InboxStepComplete      InboxStep = "complete"
)

// InboxSessionProgress reports current session position.
type InboxSessionProgress struct {
	Current int
	Total   int
}

// InboxSessionSummary reports completion counters.
type InboxSessionSummary struct {
	Trashed   int
	Someday   int
	Done      int
	Waiting   int
	Refiled   int
	ToProject int
	Skipped   int
}

// InboxDecisionKind is the final action chosen for the current inbox item.
type InboxDecisionKind string

const (
	InboxDecisionTrash        InboxDecisionKind = "trash"
	InboxDecisionDone         InboxDecisionKind = "done"
	InboxDecisionSomeday      InboxDecisionKind = "someday"
	InboxDecisionWaiting      InboxDecisionKind = "waiting"
	InboxDecisionSingleAction InboxDecisionKind = "single_action"
	InboxDecisionProject      InboxDecisionKind = "project"
	InboxDecisionNewProject   InboxDecisionKind = "new_project"
)

// InboxDecision is a typed final decision for a Process Inbox item.
type InboxDecision struct {
	Kind         InboxDecisionKind
	WaitingOn    string
	ProjectID    string
	ProjectTitle string
}

// InboxSessionItem is the current original item plus its draft.
type InboxSessionItem struct {
	Original model.Task
	Draft    model.Task
	Step     InboxStep
}

// InboxSession is the client-visible Process Inbox session state.
type InboxSession struct {
	ID       string
	Items    []model.Task
	Index    int
	Current  InboxSessionItem
	Progress InboxSessionProgress
	Summary  InboxSessionSummary
	Done     bool
}

// Core is the first-pass concrete implementation of the core boundary.
// It currently wraps the existing service/store/query architecture rather than
// replacing it.
type Core struct {
	store    *store.Store
	svc      *service.Service
	cfg      CoreConfig
	sessions map[string]*InboxSession
}

// New constructs a first-pass core facade over the existing service layer.
func New(st *store.Store, cfg CoreConfig) *Core {
	behavior := service.BehaviorConfig{
		AutoArchiveDone:     cfg.AutoArchiveDone,
		AutoArchiveCanceled: cfg.AutoArchiveCanceled,
	}
	return &Core{
		store:    st,
		svc:      service.NewWithBehavior(st, behavior),
		cfg:      cfg,
		sessions: map[string]*InboxSession{},
	}
}

// UnderlyingService exposes the wrapped legacy service during the extraction
// phase. This is intended as a temporary bridge while clients migrate to the
// new core boundary.
func (c *Core) UnderlyingService() *service.Service { return c.svc }

// UnderlyingStore exposes the wrapped store during the extraction phase.
func (c *Core) UnderlyingStore() *store.Store { return c.store }

// MergedSavedViews returns the core's runtime saved-view list, with client-
// supplied config views overriding built-ins by name.
func (c *Core) MergedSavedViews() []model.SavedView {
	return mergeSavedViews(model.DefaultViews(), c.cfg.SavedViews)
}

func cloneTasks(tasks []model.Task) []model.Task {
	out := make([]model.Task, len(tasks))
	copy(out, tasks)
	return out
}

func buildInboxSession(id string, items []model.Task, index int, summary InboxSessionSummary) *InboxSession {
	session := &InboxSession{
		ID:      id,
		Items:   cloneTasks(items),
		Index:   index,
		Summary: summary,
	}
	if index >= len(items) {
		session.Done = true
		session.Current = InboxSessionItem{Step: InboxStepComplete}
		session.Progress = InboxSessionProgress{Current: len(items), Total: len(items)}
		return session
	}
	session.Progress = InboxSessionProgress{Current: index + 1, Total: len(items)}
	task := items[index]
	session.Current = InboxSessionItem{
		Original: task,
		Draft:    task,
		Step:     InboxStepActionable,
	}
	return session
}

// StartInboxSession snapshots the inbox into a new ephemeral processing session.
func (c *Core) StartInboxSession() (*InboxSession, error) {
	list, err := c.store.ReadList(model.ListIn)
	if err != nil {
		return nil, err
	}
	sessionID := fmt.Sprintf("inbox-%d", time.Now().UnixNano())
	session := buildInboxSession(sessionID, list.Tasks, 0, InboxSessionSummary{})
	c.sessions[sessionID] = session
	return session, nil
}

// GetInboxSession returns the current state of an existing processing session.
func (c *Core) GetInboxSession(sessionID string) (*InboxSession, error) {
	session, ok := c.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("inbox session %q not found", sessionID)
	}
	copy := *session
	copy.Items = cloneTasks(session.Items)
	return &copy, nil
}

// UpdateInboxDraft applies a patch to the current draft in an inbox session.
func (c *Core) UpdateInboxDraft(sessionID string, patch TaskPatch) (*InboxSession, error) {
	session, ok := c.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("inbox session %q not found", sessionID)
	}
	if session.Done {
		return c.GetInboxSession(sessionID)
	}
	draft := session.Current.Draft
	applyTaskPatch(&draft, patch)
	session.Current.Draft = draft
	return c.GetInboxSession(sessionID)
}

// CommitInboxDecision persists the current draft according to a typed final decision
// and advances the session.
func (c *Core) CommitInboxDecision(sessionID string, decision InboxDecision) (*InboxSession, error) {
	session, ok := c.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("inbox session %q not found", sessionID)
	}
	if session.Done {
		return c.GetInboxSession(sessionID)
	}
	task := session.Current.Draft
	summary := session.Summary

	persistDraft := func() error {
		return c.svc.UpdateTask(model.ListIn, task)
	}

	switch decision.Kind {
	case InboxDecisionTrash:
		if err := c.svc.TrashTask(model.ListIn, task.ID); err != nil {
			return nil, err
		}
		summary.Trashed++
	case InboxDecisionDone:
		if err := persistDraft(); err != nil {
			return nil, err
		}
		if err := c.svc.UpdateState(model.ListIn, task.ID, model.StateDone); err != nil {
			return nil, err
		}
		summary.Done++
	case InboxDecisionSomeday:
		if err := persistDraft(); err != nil {
			return nil, err
		}
		if _, err := c.MoveTaskToList(task.ID, model.ListSingleActions, model.StateSomeday); err != nil {
			return nil, err
		}
		summary.Someday++
	case InboxDecisionWaiting:
		if decision.WaitingOn != "" {
			task.WaitingOn = decision.WaitingOn
		}
		if err := c.svc.UpdateTask(model.ListIn, task); err != nil {
			return nil, err
		}
		if _, err := c.MoveTaskToList(task.ID, model.ListSingleActions, model.StateWaitingFor); err != nil {
			return nil, err
		}
		summary.Waiting++
	case InboxDecisionSingleAction:
		if err := persistDraft(); err != nil {
			return nil, err
		}
		if _, err := c.MoveTaskToList(task.ID, model.ListSingleActions, model.StateNextAction); err != nil {
			return nil, err
		}
		summary.Refiled++
	case InboxDecisionProject:
		if decision.ProjectID == "" {
			return nil, fmt.Errorf("project decision requires project ID")
		}
		if err := persistDraft(); err != nil {
			return nil, err
		}
		proj, err := c.GetProject(decision.ProjectID)
		if err != nil {
			return nil, err
		}
		targetSubgroupID := ""
		if len(proj.Project.SubGroups) == 0 {
			sg, err := c.CreateSubgroup(decision.ProjectID, "Tasks")
			if err != nil {
				return nil, err
			}
			targetSubgroupID = sg.Subgroup.ID
		} else {
			targetSubgroupID = proj.Project.SubGroups[0].ID
		}
		if _, err := c.MoveTaskToProject(task.ID, decision.ProjectID, targetSubgroupID, model.StateNextAction); err != nil {
			return nil, err
		}
		summary.ToProject++
	case InboxDecisionNewProject:
		if strings.TrimSpace(decision.ProjectTitle) == "" {
			return nil, fmt.Errorf("new project decision requires project title")
		}
		if err := persistDraft(); err != nil {
			return nil, err
		}
		proj, err := c.svc.CreateProject(decision.ProjectTitle, "Tasks")
		if err != nil {
			return nil, err
		}
		if _, err := c.MoveTaskToProject(task.ID, proj.ID, proj.SubGroups[0].ID, model.StateNextAction); err != nil {
			return nil, err
		}
		summary.ToProject++
	default:
		return nil, fmt.Errorf("unsupported inbox decision %q", decision.Kind)
	}

	next := buildInboxSession(session.ID, session.Items, session.Index+1, summary)
	c.sessions[sessionID] = next
	return c.GetInboxSession(sessionID)
}

// SkipInboxItem advances the session to the next item without mutating storage.
func (c *Core) SkipInboxItem(sessionID string) (*InboxSession, error) {
	session, ok := c.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("inbox session %q not found", sessionID)
	}
	if session.Done {
		return c.GetInboxSession(sessionID)
	}
	summary := session.Summary
	summary.Skipped++
	next := buildInboxSession(session.ID, session.Items, session.Index+1, summary)
	c.sessions[sessionID] = next
	return c.GetInboxSession(sessionID)
}

// DiscardInboxSession drops an ephemeral processing session.
func (c *Core) DiscardInboxSession(sessionID string) error {
	if _, ok := c.sessions[sessionID]; !ok {
		return fmt.Errorf("inbox session %q not found", sessionID)
	}
	delete(c.sessions, sessionID)
	return nil
}

func mergeSavedViews(builtins, configured []model.SavedView) []model.SavedView {
	out := make([]model.SavedView, 0, len(builtins)+len(configured))
	byName := make(map[string]model.SavedView, len(configured))
	for _, v := range configured {
		byName[strings.ToLower(strings.TrimSpace(v.Name))] = v
	}

	seen := make(map[string]bool, len(builtins)+len(configured))
	for _, v := range builtins {
		key := strings.ToLower(strings.TrimSpace(v.Name))
		if override, ok := byName[key]; ok {
			out = append(out, override)
		} else {
			out = append(out, v)
		}
		seen[key] = true
	}
	for _, v := range configured {
		key := strings.ToLower(strings.TrimSpace(v.Name))
		if seen[key] {
			continue
		}
		out = append(out, v)
		seen[key] = true
	}
	return out
}

// TaskLocationKind describes where a task currently lives.
type TaskLocationKind string

const (
	TaskLocationInbox    TaskLocationKind = "inbox"
	TaskLocationActions  TaskLocationKind = "actions"
	TaskLocationProject  TaskLocationKind = "project"
	TaskLocationArchive  TaskLocationKind = "archive"
)

// TaskLocation resolves the current implementation-specific location for a
// task so the new core boundary can remain ID-first while delegating to legacy
// list/filename/index-based internals.
type TaskLocation struct {
	Kind       TaskLocationKind
	Task       model.Task
	ListType   model.ListType
	Source     string
	ProjectID  string
	Filename   string
	SubgroupID string
	SubgroupIx int
	Archived   bool
}

// ProjectLocation resolves a project ID to the legacy filename-based store.
type ProjectLocation struct {
	Project   model.Project
	ProjectID string
	Filename  string
}

// SubgroupLocation resolves a subgroup ID to its containing project and the
// subgroup's current index.
type SubgroupLocation struct {
	ProjectID   string
	Filename    string
	Project     model.Project
	Subgroup    model.SubGroup
	SubgroupIx  int
}

// ResolveTask locates a task by stable ID across inbox, actions, projects, and archive.
func (c *Core) ResolveTask(taskID string) (*TaskLocation, error) {
	for _, lt := range []model.ListType{model.ListIn, model.ListSingleActions} {
		list, err := c.store.ReadList(lt)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", lt, err)
		}
		for _, task := range list.Tasks {
			if task.ID != taskID {
				continue
			}
			kind := TaskLocationInbox
			if lt == model.ListSingleActions {
				kind = TaskLocationActions
			}
			return &TaskLocation{
				Kind:       kind,
				Task:       task,
				ListType:   lt,
				Source:     string(lt),
				SubgroupIx: -1,
			}, nil
		}
	}

	filenames, err := c.store.ListProjects()
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	for _, filename := range filenames {
		proj, err := c.store.ReadProject(filename)
		if err != nil {
			return nil, fmt.Errorf("reading project %s: %w", filename, err)
		}
		for sgIdx, sg := range proj.SubGroups {
			for _, task := range sg.Tasks {
				if task.ID != taskID {
					continue
				}
				return &TaskLocation{
					Kind:       TaskLocationProject,
					Task:       task,
					Source:     "projects/" + filename,
					ProjectID:  proj.ID,
					Filename:   filename,
					SubgroupID: sg.ID,
					SubgroupIx: sgIdx,
				}, nil
			}
		}
	}

	archiveFiles, err := c.store.ListArchives()
	if err != nil {
		return nil, fmt.Errorf("listing archives: %w", err)
	}
	for _, filename := range archiveFiles {
		archive, err := c.store.ReadArchive(filename)
		if err != nil {
			return nil, fmt.Errorf("reading archive %s: %w", filename, err)
		}
		for _, task := range archive.Tasks {
			if task.ID != taskID {
				continue
			}
			return &TaskLocation{
				Kind:       TaskLocationArchive,
				Task:       task,
				ListType:   model.ListArchive,
				Source:     "archive",
				Filename:   filename,
				SubgroupIx: -1,
				Archived:   true,
			}, nil
		}
	}

	return nil, fmt.Errorf("task %s not found", taskID)
}

// ResolveProject locates a project by stable project ID.
func (c *Core) ResolveProject(projectID string) (*ProjectLocation, error) {
	filenames, err := c.store.ListProjects()
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	for _, filename := range filenames {
		proj, err := c.store.ReadProject(filename)
		if err != nil {
			return nil, fmt.Errorf("reading project %s: %w", filename, err)
		}
		if proj.ID == projectID {
			return &ProjectLocation{Project: *proj, ProjectID: proj.ID, Filename: filename}, nil
		}
	}
	return nil, fmt.Errorf("project %s not found", projectID)
}

// ResolveSubgroup locates a subgroup by stable subgroup ID within a project.
func (c *Core) ResolveSubgroup(projectID, subgroupID string) (*SubgroupLocation, error) {
	pl, err := c.ResolveProject(projectID)
	if err != nil {
		return nil, err
	}
	for idx, sg := range pl.Project.SubGroups {
		if sg.ID == subgroupID {
			return &SubgroupLocation{
				ProjectID:  pl.ProjectID,
				Filename:   pl.Filename,
				Project:    pl.Project,
				Subgroup:   sg,
				SubgroupIx: idx,
			}, nil
		}
	}
	return nil, fmt.Errorf("subgroup %s not found in project %s", subgroupID, projectID)
}

// ProjectIDs returns the current project IDs in filename sort order. This is a
// small extraction helper for early core tests and callers.
func (c *Core) ProjectIDs() ([]string, error) {
	filenames, err := c.store.ListProjects()
	if err != nil {
		return nil, err
	}
	sort.Strings(filenames)
	ids := make([]string, 0, len(filenames))
	for _, filename := range filenames {
		proj, err := c.store.ReadProject(filename)
		if err != nil {
			return nil, err
		}
		ids = append(ids, proj.ID)
	}
	return ids, nil
}

// ListViews returns the merged runtime saved-view list.
func (c *Core) ListViews() []model.SavedView {
	return c.MergedSavedViews()
}

// RunView resolves a saved view by name and runs it through the shared query path.
func (c *Core) RunView(name string) ([]service.ViewTask, error) {
	for _, v := range c.ListViews() {
		if strings.EqualFold(v.Name, name) {
			return c.RunQuery(QueryInput{Query: v.Query, IncludeArchived: v.IncludeArchived})
		}
	}
	return nil, fmt.Errorf("saved view %q not found", name)
}

// RunQuery executes an ad-hoc query over active tasks plus optional archive tasks.
func (c *Core) RunQuery(input QueryInput) ([]service.ViewTask, error) {
	expr, err := query.Parse(strings.TrimSpace(input.Query), time.Now())
	if err != nil {
		return nil, err
	}

	all, err := c.svc.CollectAllTasks()
	if err != nil {
		return nil, err
	}
	if input.IncludeArchived {
		archived, err := c.svc.CollectArchiveTasks()
		if err != nil {
			return nil, err
		}
		all = append(all, archived...)
	}

	if expr == nil {
		return all, nil
	}

	results := make([]service.ViewTask, 0, len(all))
	for _, vt := range all {
		if query.MatchAll(expr, vt.Task, vt.Source) {
			results = append(results, vt)
		}
	}
	return results, nil
}

// GetProject resolves and returns a project by stable project ID.
func (c *Core) GetProject(projectID string) (ProjectLocation, error) {
	loc, err := c.ResolveProject(projectID)
	if err != nil {
		return ProjectLocation{}, err
	}
	return *loc, nil
}

// ListProjectSummaries returns the legacy project summaries for picker/list use
// while the extraction is still in progress.
func (c *Core) ListProjectSummaries() ([]service.ProjectSummary, error) {
	return c.svc.ListProjects()
}

// QueryProjects runs the shared query DSL over projects by projecting project metadata
// into a task-like filter shape, matching the current CLI semantics.
func (c *Core) QueryProjects(input QueryInput) ([]ProjectLocation, error) {
	files, err := c.store.ListProjects()
	if err != nil {
		return nil, err
	}
	expr, err := query.Parse(strings.TrimSpace(input.Query), time.Now())
	if err != nil {
		return nil, err
	}
	results := make([]ProjectLocation, 0, len(files))
	for _, filename := range files {
		proj, err := c.store.ReadProject(filename)
		if err != nil {
			return nil, err
		}
		pseudo := model.Task{ID: proj.ID, Created: time.Now(), Text: proj.Title, State: proj.State, Deadline: proj.Deadline, Tags: proj.Tags, URL: proj.URL, WaitingOn: proj.WaitingOn}
		if expr != nil && !query.MatchAll(expr, pseudo, "projects/"+filename) {
			continue
		}
		results = append(results, ProjectLocation{Project: *proj, ProjectID: proj.ID, Filename: filename})
	}
	return results, nil
}

// WeeklyReview proxies the existing weekly review aggregation through the core boundary.
func (c *Core) WeeklyReview(now time.Time) (service.WeeklyReviewData, error) {
	return c.svc.WeeklyReview(now)
}

// ExportMarkdown copies the full runtime dataset into a Markdown-backed destination store.
func (c *Core) ExportMarkdown(outputDir string) error {
	dst := store.NewMarkdown(outputDir)
	if err := dst.Init(); err != nil {
		return fmt.Errorf("initializing markdown destination: %w", err)
	}
	return copyAllData(c.store, dst)
}

// ImportMarkdown imports Markdown data into the runtime store using merge or replace semantics.
func (c *Core) ImportMarkdown(input ImportInput) (ImportResult, error) {
	var result ImportResult
	result.Mode = input.Mode
	result.DryRun = input.DryRun

	src := store.NewMarkdown(input.Dir)
	if err := src.Init(); err != nil {
		return result, fmt.Errorf("initializing markdown source: %w", err)
	}

	mode := input.Mode
	if mode == "" {
		mode = ImportModeMerge
		result.Mode = mode
	}

	switch mode {
	case ImportModeMerge:
		return c.importMerge(src, input.DryRun)
	case ImportModeReplace:
		return c.importReplace(src, input.DryRun)
	default:
		return result, fmt.Errorf("unsupported import mode %q", mode)
	}
}

func (c *Core) importMerge(src *store.Store, dryRun bool) (ImportResult, error) {
	result := ImportResult{Mode: ImportModeMerge, DryRun: dryRun}

	for _, lt := range []model.ListType{model.ListIn, model.ListSingleActions} {
		srcList, err := src.ReadList(lt)
		if err != nil {
			return result, fmt.Errorf("reading source %s: %w", lt, err)
		}
		dstList, err := c.store.ReadList(lt)
		if err != nil {
			return result, fmt.Errorf("reading destination %s: %w", lt, err)
		}
		existing := make(map[string]bool, len(dstList.Tasks))
		for _, t := range dstList.Tasks {
			existing[t.ID] = true
		}
		for _, t := range srcList.Tasks {
			if existing[t.ID] {
				continue
			}
			switch lt {
			case model.ListIn:
				result.InboxAdded++
			case model.ListSingleActions:
				result.ActionsAdded++
			}
			if !dryRun {
				dstList.Tasks = append(dstList.Tasks, t)
			}
		}
		if !dryRun {
			if err := c.store.WriteList(dstList); err != nil {
				return result, fmt.Errorf("writing destination %s: %w", lt, err)
			}
		}
	}

	projectFiles, err := src.ListProjects()
	if err != nil {
		return result, fmt.Errorf("listing source projects: %w", err)
	}
	currentProjectIDs := map[string]bool{}
	dstFiles, err := c.store.ListProjects()
	if err != nil {
		return result, fmt.Errorf("listing destination projects: %w", err)
	}
	for _, filename := range dstFiles {
		proj, err := c.store.ReadProject(filename)
		if err != nil {
			return result, fmt.Errorf("reading destination project %s: %w", filename, err)
		}
		currentProjectIDs[proj.ID] = true
	}
	for _, filename := range projectFiles {
		proj, err := src.ReadProject(filename)
		if err != nil {
			return result, fmt.Errorf("reading source project %s: %w", filename, err)
		}
		if currentProjectIDs[proj.ID] {
			continue
		}
		result.ProjectsAdded++
		if !dryRun {
			if err := c.store.WriteProject(proj); err != nil {
				return result, fmt.Errorf("writing destination project %s: %w", filename, err)
			}
		}
	}

	srcArchives, err := src.ListArchives()
	if err != nil {
		return result, fmt.Errorf("listing source archives: %w", err)
	}
	dstArchive, err := c.store.ReadArchive("archive.md")
	if err != nil {
		return result, fmt.Errorf("reading destination archive: %w", err)
	}
	existingArchived := make(map[string]bool, len(dstArchive.Tasks))
	for _, t := range dstArchive.Tasks {
		existingArchived[t.ID] = true
	}
	for _, filename := range srcArchives {
		archive, err := src.ReadArchive(filename)
		if err != nil {
			return result, fmt.Errorf("reading source archive %s: %w", filename, err)
		}
		for _, t := range archive.Tasks {
			if existingArchived[t.ID] {
				continue
			}
			result.ArchivedAdded++
			if !dryRun {
				dstArchive.Tasks = append(dstArchive.Tasks, t)
			}
		}
	}
	if !dryRun && result.ArchivedAdded > 0 {
		if err := c.store.WriteArchive("archive.md", dstArchive); err != nil {
			return result, fmt.Errorf("writing destination archive: %w", err)
		}
	}

	return result, nil
}

func (c *Core) importReplace(src *store.Store, dryRun bool) (ImportResult, error) {
	result := ImportResult{Mode: ImportModeReplace, DryRun: dryRun, Reset: true}

	for _, lt := range []model.ListType{model.ListIn, model.ListSingleActions} {
		list, err := src.ReadList(lt)
		if err != nil {
			return result, fmt.Errorf("reading source %s: %w", lt, err)
		}
		switch lt {
		case model.ListIn:
			result.InboxAdded = len(list.Tasks)
		case model.ListSingleActions:
			result.ActionsAdded = len(list.Tasks)
		}
	}
	projectFiles, err := src.ListProjects()
	if err != nil {
		return result, fmt.Errorf("listing source projects: %w", err)
	}
	result.ProjectsAdded = len(projectFiles)
	srcArchives, err := src.ListArchives()
	if err != nil {
		return result, fmt.Errorf("listing source archives: %w", err)
	}
	for _, filename := range srcArchives {
		archive, err := src.ReadArchive(filename)
		if err != nil {
			return result, fmt.Errorf("reading source archive %s: %w", filename, err)
		}
		result.ArchivedAdded += len(archive.Tasks)
	}

	if dryRun {
		return result, nil
	}
	if err := c.store.Reset(); err != nil {
		return result, fmt.Errorf("resetting destination store: %w", err)
	}
	if err := c.store.Init(); err != nil {
		return result, fmt.Errorf("reinitializing destination store: %w", err)
		}
	if err := copyAllData(src, c.store); err != nil {
		return result, err
	}
	return result, nil
}

func copyAllData(src *store.Store, dst *store.Store) error {
	for _, lt := range []model.ListType{model.ListIn, model.ListSingleActions} {
		list, err := src.ReadList(lt)
		if err != nil {
			return fmt.Errorf("reading %s: %w", lt, err)
		}
		if err := dst.WriteList(list); err != nil {
			return fmt.Errorf("writing %s: %w", lt, err)
		}
	}
	projects, err := src.ListProjects()
	if err != nil {
		return fmt.Errorf("listing projects: %w", err)
	}
	for _, filename := range projects {
		proj, err := src.ReadProject(filename)
		if err != nil {
			return fmt.Errorf("reading project %s: %w", filename, err)
		}
		if err := dst.WriteProject(proj); err != nil {
			return fmt.Errorf("writing project %s: %w", filename, err)
		}
	}
	archives, err := src.ListArchives()
	if err != nil {
		return fmt.Errorf("listing archives: %w", err)
	}
	archiveAgg := &model.TaskList{Title: "Archive", Type: model.ListArchive}
	for _, filename := range archives {
		archive, err := src.ReadArchive(filename)
		if err != nil {
			return fmt.Errorf("reading archive %s: %w", filename, err)
		}
		archiveAgg.Tasks = append(archiveAgg.Tasks, archive.Tasks...)
	}
	if len(archiveAgg.Tasks) > 0 {
		if err := dst.WriteArchive("archive.md", archiveAgg); err != nil {
			return fmt.Errorf("writing archive: %w", err)
		}
	}
	return nil
}

// UpdateTask applies a patch to a task found by stable task ID, then delegates
// to the existing location-specific service implementation.
func (c *Core) UpdateTask(taskID string, patch TaskPatch) (TaskLocation, error) {
	loc, err := c.ResolveTask(taskID)
	if err != nil {
		return TaskLocation{}, err
	}
	if loc.Archived {
		return TaskLocation{}, fmt.Errorf("task %s is archived and cannot be updated in place", taskID)
	}

	updated := loc.Task
	applyTaskPatch(&updated, patch)

	if loc.Kind == TaskLocationProject {
		if err := c.svc.UpdateProjectTask(loc.Filename, loc.SubgroupIx, updated); err != nil {
			return TaskLocation{}, err
		}
	} else {
		if err := c.svc.UpdateTask(loc.ListType, updated); err != nil {
			return TaskLocation{}, err
		}
	}

	newLoc, err := c.ResolveTask(taskID)
	if err != nil {
		return TaskLocation{}, err
	}
	return *newLoc, nil
}

// UpdateProject applies a patch to a project found by stable project ID, then
// delegates to the existing filename-based service implementation.
func (c *Core) UpdateProject(projectID string, patch ProjectPatch) (ProjectLocation, error) {
	loc, err := c.ResolveProject(projectID)
	if err != nil {
		return ProjectLocation{}, err
	}

	updated := loc.Project
	applyProjectPatch(&updated, patch)
	if patch.WaitingOn != nil || containsNormalizedField(patch.Clear, "waiting_on") {
		loc.Project.WaitingOn = updated.WaitingOn
	}

	newFilename, err := c.svc.UpdateProject(loc.Filename, updated)
	if err != nil {
		return ProjectLocation{}, err
	}
	proj, err := c.store.ReadProject(newFilename)
	if err != nil {
		return ProjectLocation{}, err
	}
	return ProjectLocation{Project: *proj, ProjectID: proj.ID, Filename: newFilename}, nil
}

func applyTaskPatch(task *model.Task, patch TaskPatch) {
	if patch.Text != nil {
		task.Text = *patch.Text
	}
	if patch.State != nil {
		task.State = *patch.State
	}
	if patch.Tags != nil {
		task.Tags = append([]string(nil), (*patch.Tags)...)
	}
	if patch.URL != nil {
		task.URL = *patch.URL
	}
	if patch.Notes != nil {
		task.Notes = *patch.Notes
	}
	if patch.WaitingOn != nil {
		task.WaitingOn = *patch.WaitingOn
	}
	if patch.Deadline != nil {
		t := *patch.Deadline
		task.Deadline = &t
	}
	if patch.Scheduled != nil {
		t := *patch.Scheduled
		task.Scheduled = &t
	}
	for _, field := range patch.Clear {
		switch normalizeField(field) {
		case "deadline":
			task.Deadline = nil
		case "scheduled":
			task.Scheduled = nil
		case "tags":
			task.Tags = nil
		case "url":
			task.URL = ""
		case "notes":
			task.Notes = ""
		case "waiting_on":
			task.WaitingOn = ""
		}
	}
}

func applyProjectPatch(proj *model.Project, patch ProjectPatch) {
	if patch.Title != nil {
		proj.Title = *patch.Title
	}
	if patch.State != nil {
		proj.State = *patch.State
	}
	if patch.Tags != nil {
		proj.Tags = append([]string(nil), (*patch.Tags)...)
	}
	if patch.URL != nil {
		proj.URL = *patch.URL
	}
	if patch.WaitingOn != nil {
		proj.WaitingOn = *patch.WaitingOn
	}
	if patch.DefinitionOfDone != nil {
		proj.DefinitionOfDone = *patch.DefinitionOfDone
	}
	if patch.Deadline != nil {
		t := *patch.Deadline
		proj.Deadline = &t
	}
	for _, field := range patch.Clear {
		switch normalizeField(field) {
		case "deadline":
			proj.Deadline = nil
		case "tags":
			proj.Tags = nil
		case "url":
			proj.URL = ""
		case "waiting_on":
			proj.WaitingOn = ""
		case "definition_of_done":
			proj.DefinitionOfDone = ""
		}
	}
}

func normalizeField(field string) string {
	field = strings.TrimSpace(strings.ToLower(field))
	field = strings.ReplaceAll(field, "-", "_")
	return field
}

func containsNormalizedField(fields []string, want string) bool {
	want = normalizeField(want)
	for _, field := range fields {
		if normalizeField(field) == want {
			return true
		}
	}
	return false
}

// CreateSubgroup adds a subgroup to a project identified by stable project ID.
func (c *Core) CreateSubgroup(projectID, title string) (SubgroupLocation, error) {
	pl, err := c.ResolveProject(projectID)
	if err != nil {
		return SubgroupLocation{}, err
	}
	sg, err := c.svc.AddSubGroup(pl.Filename, title)
	if err != nil {
		return SubgroupLocation{}, err
	}
	return SubgroupLocation{ProjectID: pl.ProjectID, Filename: pl.Filename, Project: pl.Project, Subgroup: *sg, SubgroupIx: len(pl.Project.SubGroups)}, nil
}

// RenameSubgroup renames a subgroup using stable project/subgroup IDs.
func (c *Core) RenameSubgroup(projectID, subgroupID, title string) (SubgroupLocation, error) {
	loc, err := c.ResolveSubgroup(projectID, subgroupID)
	if err != nil {
		return SubgroupLocation{}, err
	}
	if err := c.svc.RenameSubGroup(loc.Filename, loc.SubgroupIx, title); err != nil {
		return SubgroupLocation{}, err
	}
	updated, err := c.ResolveSubgroup(projectID, subgroupID)
	if err != nil {
		return SubgroupLocation{}, err
	}
	return *updated, nil
}

// DeleteSubgroup deletes an empty subgroup using stable project/subgroup IDs.
func (c *Core) DeleteSubgroup(projectID, subgroupID string) error {
	loc, err := c.ResolveSubgroup(projectID, subgroupID)
	if err != nil {
		return err
	}
	return c.svc.DeleteSubGroup(loc.Filename, loc.SubgroupIx)
}

// AddProjectTask adds a task to a subgroup using stable project/subgroup IDs.
func (c *Core) AddProjectTask(projectID, subgroupID, text string, opts CaptureOpts) (TaskLocation, error) {
	loc, err := c.ResolveSubgroup(projectID, subgroupID)
	if err != nil {
		return TaskLocation{}, err
	}
	state := model.StateNextAction
	if opts.WaitingOn != "" {
		state = model.StateWaitingFor
	}
	task, err := c.svc.AddTaskToProject(loc.Filename, loc.SubgroupIx, text, state)
	if err != nil {
		return TaskLocation{}, err
	}
	hasPatch := false
	patch := TaskPatch{}
	if opts.Deadline != nil {
		hasPatch = true
		patch.Deadline = opts.Deadline
	}
	if opts.Scheduled != nil {
		hasPatch = true
		patch.Scheduled = opts.Scheduled
	}
	if len(opts.Tags) > 0 {
		hasPatch = true
		tags := append([]string(nil), opts.Tags...)
		patch.Tags = &tags
	}
	if opts.URL != "" {
		hasPatch = true
		patch.URL = &opts.URL
	}
	if opts.Notes != "" {
		hasPatch = true
		patch.Notes = &opts.Notes
	}
	if opts.WaitingOn != "" {
		hasPatch = true
		patch.WaitingOn = &opts.WaitingOn
	}
	if hasPatch {
		if _, err := c.UpdateTask(task.ID, patch); err != nil {
			return TaskLocation{}, err
		}
	}
	resolved, err := c.ResolveTask(task.ID)
	if err != nil {
		return TaskLocation{}, err
	}
	return *resolved, nil
}

// MoveTaskToSubgroup moves a project task between subgroups using stable IDs.
func (c *Core) MoveTaskToSubgroup(taskID, subgroupID string) error {
	taskLoc, err := c.ResolveTask(taskID)
	if err != nil {
		return err
	}
	if taskLoc.Kind != TaskLocationProject {
		return fmt.Errorf("task %s is not a project task", taskID)
	}
	target, err := c.ResolveSubgroup(taskLoc.ProjectID, subgroupID)
	if err != nil {
		return err
	}
	return c.svc.MoveTaskBetweenSubGroups(taskLoc.Filename, taskLoc.SubgroupIx, taskID, target.SubgroupIx)
}

// ReorderProjectTask reorders a project task within its current subgroup using a stable task ID.
func (c *Core) ReorderProjectTask(taskID string, delta int) error {
	loc, err := c.ResolveTask(taskID)
	if err != nil {
		return err
	}
	if loc.Kind != TaskLocationProject {
		return fmt.Errorf("task %s is not a project task", taskID)
	}
	return c.svc.ReorderTaskInSubGroup(loc.Filename, loc.SubgroupIx, taskID, delta)
}

// RestoreTrashedTask restores a just-trashed raw task payload back to the given source.
// This is a transitional bridge for undo flows that still hold a pre-trash task copy.
func (c *Core) RestoreTrashedTask(task model.Task, source string) (TaskLocation, error) {
	label, err := c.svc.RestoreTask(task, source)
	if err != nil {
		return TaskLocation{}, err
	}
	_ = label
	loc, err := c.ResolveTask(task.ID)
	if err != nil {
		return TaskLocation{}, err
	}
	return *loc, nil
}

// MoveTaskToList moves a task by stable ID into a destination list with the requested state.
func (c *Core) MoveTaskToList(taskID string, toList model.ListType, newState model.TaskState) (TaskLocation, error) {
	loc, err := c.ResolveTask(taskID)
	if err != nil {
		return TaskLocation{}, err
	}
	if loc.Archived {
		return TaskLocation{}, fmt.Errorf("task %s is archived and cannot be moved", taskID)
	}
	if loc.Kind == TaskLocationProject {
		if err := c.svc.MoveTaskFromProjectToList(loc.Filename, loc.SubgroupIx, taskID, toList, newState); err != nil {
			return TaskLocation{}, err
		}
	} else {
		if loc.ListType == toList {
			updated, err := c.UpdateTask(taskID, TaskPatch{State: &newState})
			return updated, err
		}
		if err := c.svc.MoveToList(loc.ListType, taskID, toList, newState); err != nil {
			return TaskLocation{}, err
		}
	}
	resolved, err := c.ResolveTask(taskID)
	if err != nil {
		return TaskLocation{}, err
	}
	return *resolved, nil
}

// MoveTaskToProject moves a task by stable ID into a project subgroup with the requested state.
func (c *Core) MoveTaskToProject(taskID string, projectID, subgroupID string, newState model.TaskState) (TaskLocation, error) {
	loc, err := c.ResolveTask(taskID)
	if err != nil {
		return TaskLocation{}, err
	}
	if loc.Archived {
		return TaskLocation{}, fmt.Errorf("task %s is archived and cannot be moved", taskID)
	}
	target, err := c.ResolveSubgroup(projectID, subgroupID)
	if err != nil {
		return TaskLocation{}, err
	}
	if loc.Kind == TaskLocationProject {
		if loc.ProjectID == projectID && loc.SubgroupID == subgroupID {
			updated, err := c.UpdateTask(taskID, TaskPatch{State: &newState})
			return updated, err
		}
		return TaskLocation{}, fmt.Errorf("moving project tasks between projects is not implemented yet")
	} else {
		if err := c.svc.MoveToProject(loc.ListType, taskID, target.Filename, target.SubgroupIx, newState); err != nil {
			return TaskLocation{}, err
		}
	}
	resolved, err := c.ResolveTask(taskID)
	if err != nil {
		return TaskLocation{}, err
	}
	return *resolved, nil
}

// ArchiveTask archives an active task by stable ID.
func (c *Core) ArchiveTask(taskID string) error {
	loc, err := c.ResolveTask(taskID)
	if err != nil {
		return err
	}
	if loc.Archived {
		return fmt.Errorf("task %s is already archived", taskID)
	}
	if loc.Kind == TaskLocationProject {
		return c.svc.ArchiveProjectTask(loc.Filename, loc.SubgroupIx, taskID)
	}
	return c.svc.ArchiveTask(loc.ListType, taskID)
}

// RestoreTask restores an archived task by stable ID to its recorded source.
func (c *Core) RestoreTask(taskID string) (TaskLocation, error) {
	loc, err := c.ResolveTask(taskID)
	if err != nil {
		return TaskLocation{}, err
	}
	if !loc.Archived {
		return TaskLocation{}, fmt.Errorf("task %s is not archived", taskID)
	}
	if _, err := c.svc.RestoreArchivedTask(taskID); err != nil {
		return TaskLocation{}, err
	}
	newLoc, err := c.ResolveTask(taskID)
	if err != nil {
		return TaskLocation{}, err
	}
	return *newLoc, nil
}

// TrashTask permanently removes an active task by stable ID.
func (c *Core) TrashTask(taskID string) error {
	loc, err := c.ResolveTask(taskID)
	if err != nil {
		return err
	}
	if loc.Archived {
		return fmt.Errorf("task %s is archived and cannot be trashed via active-task path", taskID)
	}
	if loc.Kind == TaskLocationProject {
		return c.svc.TrashProjectTask(loc.Filename, loc.SubgroupIx, taskID)
	}
	return c.svc.TrashTask(loc.ListType, taskID)
}

// CaptureOpts is the first-pass struct form of capture inputs at the core boundary.
type CaptureOpts struct {
	Deadline  *time.Time
	Scheduled *time.Time
	Tags      []string
	URL       string
	Notes     string
	WaitingOn string
}

// CaptureToInbox adds a task to the inbox through the core boundary.
func (c *Core) CaptureToInbox(text string, opts CaptureOpts) (model.Task, error) {
	legacyOpts := make([]service.TaskOption, 0, 5)
	if opts.Deadline != nil {
		legacyOpts = append(legacyOpts, service.WithDeadline(*opts.Deadline))
	}
	if opts.Scheduled != nil {
		legacyOpts = append(legacyOpts, service.WithScheduled(*opts.Scheduled))
	}
	if len(opts.Tags) > 0 {
		legacyOpts = append(legacyOpts, service.WithTags(append([]string(nil), opts.Tags...)))
	}
	if opts.URL != "" {
		legacyOpts = append(legacyOpts, service.WithURL(opts.URL))
	}
	if opts.Notes != "" {
		legacyOpts = append(legacyOpts, service.WithNotes(opts.Notes))
	}
	if opts.WaitingOn != "" {
		legacyOpts = append(legacyOpts, service.WithWaitingOn(opts.WaitingOn))
	}
	task, err := c.svc.AddToInbox(text, legacyOpts...)
	if err != nil {
		return model.Task{}, err
	}
	return *task, nil
}

// QueryInput is the first-pass query request shape for the core boundary.
type QueryInput struct {
	Query           string
	IncludeArchived bool
}

// TaskPatch describes partial task updates at the core boundary.
type TaskPatch struct {
	Text      *string
	State     *model.TaskState
	Deadline  *time.Time
	Scheduled *time.Time
	Tags      *[]string
	URL       *string
	Notes     *string
	WaitingOn *string
	Clear     []string
}

// ProjectPatch describes partial project updates at the core boundary.
type ProjectPatch struct {
	Title            *string
	State            *model.TaskState
	Deadline         *time.Time
	Tags             *[]string
	URL              *string
	WaitingOn        *string
	DefinitionOfDone *string
	Clear            []string
}

// ImportMode controls how Markdown import writes into the runtime store.
type ImportMode string

const (
	ImportModeMerge   ImportMode = "merge"
	ImportModeReplace ImportMode = "replace"
)

// ImportInput describes a Markdown import request.
type ImportInput struct {
	Dir    string
	Mode   ImportMode
	DryRun bool
}

// ImportResult summarizes a Markdown import operation.
type ImportResult struct {
	Mode          ImportMode
	DryRun        bool
	InboxAdded    int
	ActionsAdded  int
	ProjectsAdded int
	ArchivedAdded int
	Reset         bool
}
