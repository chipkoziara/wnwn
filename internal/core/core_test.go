package core

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/chipkoziara/wnwn/internal/id"
	"github.com/chipkoziara/wnwn/internal/model"
	"github.com/chipkoziara/wnwn/internal/store"
)

func newTestCore(t *testing.T) *Core {
	t.Helper()
	root := t.TempDir()
	st := store.New(root)
	if err := st.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	return New(st, CoreConfig{})
}

func TestMergedSavedViews_ConfigOverridesBuiltinByName(t *testing.T) {
	c := New(store.New(filepath.Join(t.TempDir(), "data")), CoreConfig{
		SavedViews: []model.SavedView{{Name: "Waiting For", Query: "tag:@office"}, {Name: "My View", Query: "state:next-action"}},
	})

	views := c.MergedSavedViews()
	var waiting model.SavedView
	var foundWaiting, foundCustom bool
	for _, v := range views {
		if v.Name == "Waiting For" {
			waiting = v
			foundWaiting = true
		}
		if v.Name == "My View" {
			foundCustom = true
		}
	}
	if !foundWaiting {
		t.Fatalf("expected overridden builtin view to remain present")
	}
	if waiting.Query != "tag:@office" {
		t.Fatalf("expected config override query, got %q", waiting.Query)
	}
	if !foundCustom {
		t.Fatalf("expected custom config view to be appended")
	}
}

func TestResolveTask_ListProjectAndArchive(t *testing.T) {
	c := newTestCore(t)

	inboxList, err := c.store.ReadList(model.ListIn)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	inboxTask := model.Task{ID: id.New(), Created: time.Now(), Text: "Inbox task"}
	inboxList.Tasks = append(inboxList.Tasks, inboxTask)
	if err := c.store.WriteList(inboxList); err != nil {
		t.Fatalf("write inbox: %v", err)
	}

	actionsList, err := c.store.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatalf("read actions: %v", err)
	}
	actionsTask := model.Task{ID: id.New(), Created: time.Now(), Text: "Action task", State: model.StateNextAction}
	actionsList.Tasks = append(actionsList.Tasks, actionsTask)
	if err := c.store.WriteList(actionsList); err != nil {
		t.Fatalf("write actions: %v", err)
	}

	proj := &model.Project{
		Title: "Launch Website",
		ID:    id.New(),
		State: model.StateActive,
		Tags:  []string{"project"},
		SubGroups: []model.SubGroup{{
			Title: "Phase 1",
			ID:    id.New(),
			Tasks: []model.Task{{ID: id.New(), Created: time.Now(), Text: "Project task", State: model.StateNextAction}},
		}},
	}
	if err := c.store.WriteProject(proj); err != nil {
		t.Fatalf("write project: %v", err)
	}
	projectTask := proj.SubGroups[0].Tasks[0]

	archive, err := c.store.ReadArchive("archive.md")
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	archivedAt := time.Now()
	archiveTask := model.Task{ID: id.New(), Created: time.Now(), Text: "Archived task", Source: string(model.ListSingleActions), ArchivedAt: &archivedAt}
	archive.Tasks = append(archive.Tasks, archiveTask)
	if err := c.store.WriteArchive("archive.md", archive); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	loc, err := c.ResolveTask(inboxTask.ID)
	if err != nil {
		t.Fatalf("resolve inbox task: %v", err)
	}
	if loc.Kind != TaskLocationInbox || loc.ListType != model.ListIn {
		t.Fatalf("unexpected inbox location: %+v", loc)
	}

	loc, err = c.ResolveTask(actionsTask.ID)
	if err != nil {
		t.Fatalf("resolve actions task: %v", err)
	}
	if loc.Kind != TaskLocationActions || loc.ListType != model.ListSingleActions {
		t.Fatalf("unexpected actions location: %+v", loc)
	}

	loc, err = c.ResolveTask(projectTask.ID)
	if err != nil {
		t.Fatalf("resolve project task: %v", err)
	}
	if loc.Kind != TaskLocationProject || loc.ProjectID != proj.ID || loc.SubgroupID != proj.SubGroups[0].ID || loc.SubgroupIx != 0 {
		t.Fatalf("unexpected project location: %+v", loc)
	}
	if loc.Filename == "" {
		t.Fatalf("expected project filename to be resolved")
	}

	loc, err = c.ResolveTask(archiveTask.ID)
	if err != nil {
		t.Fatalf("resolve archive task: %v", err)
	}
	if loc.Kind != TaskLocationArchive || !loc.Archived || loc.Filename != "archive.md" {
		t.Fatalf("unexpected archive location: %+v", loc)
	}
}

func TestResolveProjectAndSubgroup(t *testing.T) {
	c := newTestCore(t)
	proj := &model.Project{
		Title: "Roadmap",
		ID:    id.New(),
		State: model.StateActive,
		Tags:  []string{"project"},
		SubGroups: []model.SubGroup{
			{Title: "Now", ID: id.New()},
			{Title: "Next", ID: id.New()},
		},
	}
	if err := c.store.WriteProject(proj); err != nil {
		t.Fatalf("write project: %v", err)
	}

	pl, err := c.ResolveProject(proj.ID)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	if pl.ProjectID != proj.ID || pl.Filename == "" || pl.Project.Title != "Roadmap" {
		t.Fatalf("unexpected project location: %+v", pl)
	}

	sg, err := c.ResolveSubgroup(proj.ID, proj.SubGroups[1].ID)
	if err != nil {
		t.Fatalf("resolve subgroup: %v", err)
	}
	if sg.SubgroupIx != 1 || sg.Subgroup.Title != "Next" || sg.Filename == "" {
		t.Fatalf("unexpected subgroup location: %+v", sg)
	}
}

func TestRunViewAndQuery(t *testing.T) {
	c := newTestCore(t)
	list, err := c.store.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatalf("read actions: %v", err)
	}
	list.Tasks = append(list.Tasks,
		model.Task{ID: id.New(), Created: time.Now(), Text: "Buy stamps", State: model.StateNextAction, Tags: []string{"@errands"}},
		model.Task{ID: id.New(), Created: time.Now(), Text: "Wait for reply", State: model.StateWaitingFor},
	)
	if err := c.store.WriteList(list); err != nil {
		t.Fatalf("write actions: %v", err)
	}
	proj := &model.Project{Title: "Launch", ID: id.New(), State: model.StateActive, Tags: []string{"project", "launch"}}
	if err := c.store.WriteProject(proj); err != nil {
		t.Fatalf("write project: %v", err)
	}

	views := c.ListViews()
	if len(views) == 0 {
		t.Fatalf("expected default saved views")
	}

	res, err := c.RunView("Next Actions")
	if err != nil {
		t.Fatalf("run view: %v", err)
	}
	if len(res) != 1 || res[0].Task.Text != "Buy stamps" {
		t.Fatalf("unexpected next actions results: %+v", res)
	}

	res, err = c.RunQuery(QueryInput{Query: "state:waiting-for"})
	if err != nil {
		t.Fatalf("run query: %v", err)
	}
	if len(res) != 1 || res[0].Task.Text != "Wait for reply" {
		t.Fatalf("unexpected query results: %+v", res)
	}

	projects, err := c.QueryProjects(QueryInput{Query: "tag:launch"})
	if err != nil {
		t.Fatalf("query projects: %v", err)
	}
	if len(projects) != 1 || projects[0].Project.Title != "Launch" {
		t.Fatalf("unexpected project query results: %+v", projects)
	}
}

func TestWeeklyReviewAndImportExport(t *testing.T) {
	c := newTestCore(t)

	proj := &model.Project{
		Title: "Website",
		ID:    id.New(),
		State: model.StateActive,
		Tags:  []string{"project"},
		SubGroups: []model.SubGroup{{
			Title: "Launch",
			ID:    id.New(),
			Tasks: []model.Task{{ID: id.New(), Created: time.Now(), Text: "Waiting on copy", State: model.StateWaitingFor, WaitingSince: timePtr(time.Now().AddDate(0, 0, -10))}},
		}},
	}
	if err := c.store.WriteProject(proj); err != nil {
		t.Fatalf("write project: %v", err)
	}

	review, err := c.WeeklyReview(time.Now())
	if err != nil {
		t.Fatalf("weekly review: %v", err)
	}
	if len(review.ProjectsWithoutNextAction) != 1 {
		t.Fatalf("expected one project without next action, got %d", len(review.ProjectsWithoutNextAction))
	}
	if len(review.AgingWaitingFor) != 1 {
		t.Fatalf("expected one aging waiting-for task, got %d", len(review.AgingWaitingFor))
	}

	exportDir := t.TempDir()
	if err := c.ExportMarkdown(exportDir); err != nil {
		t.Fatalf("export markdown: %v", err)
	}

	importSrc := store.NewMarkdown(exportDir)
	if err := importSrc.Init(); err != nil {
		t.Fatalf("init import src: %v", err)
	}
	inbox, err := importSrc.ReadList(model.ListIn)
	if err != nil {
		t.Fatalf("read exported inbox: %v", err)
	}
	inbox.Tasks = append(inbox.Tasks, model.Task{ID: id.New(), Created: time.Now(), Text: "Imported inbox item"})
	if err := importSrc.WriteList(inbox); err != nil {
		t.Fatalf("write exported inbox: %v", err)
	}

	result, err := c.ImportMarkdown(ImportInput{Dir: exportDir, Mode: ImportModeMerge, DryRun: true})
	if err != nil {
		t.Fatalf("import dry-run: %v", err)
	}
	if !result.DryRun || result.InboxAdded != 1 {
		t.Fatalf("unexpected dry-run import result: %+v", result)
	}
}

func TestUpdateTaskPatch_ListAndProject(t *testing.T) {
	c := newTestCore(t)

	inbox, err := c.store.ReadList(model.ListIn)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	inboxTask := model.Task{ID: id.New(), Created: time.Now(), Text: "Inbox task", Notes: "old", URL: "https://example.com", Tags: []string{"old"}}
	inbox.Tasks = append(inbox.Tasks, inboxTask)
	if err := c.store.WriteList(inbox); err != nil {
		t.Fatalf("write inbox: %v", err)
	}

	newText := "Inbox task updated"
	newState := model.StateWaitingFor
	waitingOn := "Marvin"
	loc, err := c.UpdateTask(inboxTask.ID, TaskPatch{Text: &newText, State: &newState, WaitingOn: &waitingOn, Clear: []string{"notes", "url"}})
	if err != nil {
		t.Fatalf("update list task: %v", err)
	}
	if loc.Task.Text != newText || loc.Task.WaitingOn != waitingOn || loc.Task.Notes != "" || loc.Task.URL != "" {
		t.Fatalf("unexpected updated list task: %+v", loc.Task)
	}
	if loc.Task.WaitingSince == nil {
		t.Fatalf("expected waiting_since to be auto-set")
	}

	proj := &model.Project{
		Title: "Launch",
		ID:    id.New(),
		State: model.StateActive,
		Tags:  []string{"project"},
		SubGroups: []model.SubGroup{{
			Title: "Phase 1",
			ID:    id.New(),
			Tasks: []model.Task{{ID: id.New(), Created: time.Now(), Text: "Project task", Tags: []string{"@computer"}, URL: "https://old"}},
		}},
	}
	if err := c.store.WriteProject(proj); err != nil {
		t.Fatalf("write project: %v", err)
	}
	projectTaskID := proj.SubGroups[0].Tasks[0].ID
	newTags := []string{"deep-work", "@office"}
	newURL := "https://new.example.com"
	loc, err = c.UpdateTask(projectTaskID, TaskPatch{Tags: &newTags, URL: &newURL})
	if err != nil {
		t.Fatalf("update project task: %v", err)
	}
	if loc.Kind != TaskLocationProject || loc.Task.URL != newURL || len(loc.Task.Tags) != 2 {
		t.Fatalf("unexpected updated project task: %+v", loc)
	}
}

func TestUpdateProjectPatchAndClear(t *testing.T) {
	c := newTestCore(t)
	deadline := time.Now().Add(24 * time.Hour).Truncate(time.Minute)
	proj := &model.Project{
		Title:            "Launch Website",
		ID:               id.New(),
		State:            model.StateActive,
		Deadline:         &deadline,
		Tags:             []string{"project", "launch"},
		URL:              "https://old.example.com",
		WaitingOn:        "Sarah",
		DefinitionOfDone: "Ship it",
		SubGroups:        []model.SubGroup{{Title: "Now", ID: id.New()}},
	}
	if err := c.store.WriteProject(proj); err != nil {
		t.Fatalf("write project: %v", err)
	}

	newTitle := "Launch New Website"
	newState := model.StateWaitingFor
	newTags := []string{"project", "marketing"}
	updated, err := c.UpdateProject(proj.ID, ProjectPatch{Title: &newTitle, State: &newState, Tags: &newTags, Clear: []string{"deadline", "url", "waiting_on", "definition_of_done"}})
	if err != nil {
		t.Fatalf("update project: %v", err)
	}
	if updated.Project.Title != newTitle || updated.Project.State != newState {
		t.Fatalf("unexpected updated project: %+v", updated.Project)
	}
	if updated.Project.Deadline != nil || updated.Project.URL != "" || updated.Project.DefinitionOfDone != "" {
		t.Fatalf("expected cleared project fields, got %+v", updated.Project)
	}
	// Note: the legacy service.UpdateProject path does not yet persist WaitingOn.
	if updated.Project.WaitingOn != "Sarah" {
		t.Fatalf("expected legacy waiting_on behavior to remain unchanged for now, got %+v", updated.Project)
	}
	if updated.Filename == "" {
		t.Fatalf("expected resolved project filename after update")
	}
}

func TestArchiveRestoreAndTrashByTaskID(t *testing.T) {
	c := newTestCore(t)

	inbox, err := c.store.ReadList(model.ListIn)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	inboxTask := model.Task{ID: id.New(), Created: time.Now(), Text: "Inbox task"}
	inbox.Tasks = append(inbox.Tasks, inboxTask)
	if err := c.store.WriteList(inbox); err != nil {
		t.Fatalf("write inbox: %v", err)
	}

	actions, err := c.store.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatalf("read actions: %v", err)
	}
	actionTask := model.Task{ID: id.New(), Created: time.Now(), Text: "Action task", State: model.StateNextAction}
	actions.Tasks = append(actions.Tasks, actionTask)
	if err := c.store.WriteList(actions); err != nil {
		t.Fatalf("write actions: %v", err)
	}

	proj := &model.Project{
		Title: "Project",
		ID:    id.New(),
		State: model.StateActive,
		Tags:  []string{"project"},
		SubGroups: []model.SubGroup{{
			Title: "Now",
			ID:    id.New(),
			Tasks: []model.Task{{ID: id.New(), Created: time.Now(), Text: "Project task", State: model.StateNextAction}},
		}},
	}
	if err := c.store.WriteProject(proj); err != nil {
		t.Fatalf("write project: %v", err)
	}
	projectTaskID := proj.SubGroups[0].Tasks[0].ID

	if err := c.ArchiveTask(inboxTask.ID); err != nil {
		t.Fatalf("archive inbox task: %v", err)
	}
	loc, err := c.ResolveTask(inboxTask.ID)
	if err != nil {
		t.Fatalf("resolve archived inbox task: %v", err)
	}
	if !loc.Archived || loc.Kind != TaskLocationArchive {
		t.Fatalf("expected archived inbox task location, got %+v", loc)
	}

	restored, err := c.RestoreTask(inboxTask.ID)
	if err != nil {
		t.Fatalf("restore inbox task: %v", err)
	}
	if restored.Kind != TaskLocationInbox || restored.Archived {
		t.Fatalf("expected restored inbox task in inbox, got %+v", restored)
	}

	if err := c.TrashTask(actionTask.ID); err != nil {
		t.Fatalf("trash action task: %v", err)
	}
	if _, err := c.ResolveTask(actionTask.ID); err == nil {
		t.Fatalf("expected trashed action task to be gone")
	}

	if err := c.ArchiveTask(projectTaskID); err != nil {
		t.Fatalf("archive project task: %v", err)
	}
	loc, err = c.ResolveTask(projectTaskID)
	if err != nil {
		t.Fatalf("resolve archived project task: %v", err)
	}
	if !loc.Archived || loc.Kind != TaskLocationArchive {
		t.Fatalf("expected archived project task location, got %+v", loc)
	}
	if _, err := c.RestoreTask(projectTaskID); err != nil {
		t.Fatalf("restore project task: %v", err)
	}
	loc, err = c.ResolveTask(projectTaskID)
	if err != nil {
		t.Fatalf("resolve restored project task: %v", err)
	}
	if loc.Kind != TaskLocationProject || loc.Archived {
		t.Fatalf("expected restored project task location, got %+v", loc)
	}
}

func TestProjectAndSubgroupCoreAPIs(t *testing.T) {
	c := newTestCore(t)
	proj := &model.Project{
		Title: "Roadmap",
		ID:    id.New(),
		State: model.StateActive,
		Tags:  []string{"project"},
		SubGroups: []model.SubGroup{
			{Title: "Now", ID: id.New(), Tasks: []model.Task{{ID: id.New(), Created: time.Now(), Text: "Task A", State: model.StateNextAction}}},
			{Title: "Later", ID: id.New()},
		},
	}
	if err := c.store.WriteProject(proj); err != nil {
		t.Fatalf("write project: %v", err)
	}

	got, err := c.GetProject(proj.ID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if got.Project.Title != "Roadmap" || got.Filename == "" {
		t.Fatalf("unexpected get project result: %+v", got)
	}

	sg, err := c.CreateSubgroup(proj.ID, "Maybe")
	if err != nil {
		t.Fatalf("create subgroup: %v", err)
	}
	if sg.Subgroup.Title != "Maybe" {
		t.Fatalf("unexpected created subgroup: %+v", sg)
	}

	renamed, err := c.RenameSubgroup(proj.ID, proj.SubGroups[1].ID, "Next")
	if err != nil {
		t.Fatalf("rename subgroup: %v", err)
	}
	if renamed.Subgroup.Title != "Next" {
		t.Fatalf("unexpected renamed subgroup: %+v", renamed)
	}

	added, err := c.AddProjectTask(proj.ID, proj.SubGroups[1].ID, "Task B", CaptureOpts{Notes: "note", URL: "https://example.com", Tags: []string{"@computer"}})
	if err != nil {
		t.Fatalf("add project task: %v", err)
	}
	if added.Kind != TaskLocationProject || added.Task.URL != "https://example.com" || added.Task.Notes != "note" {
		t.Fatalf("unexpected added project task: %+v", added)
	}

	if err := c.MoveTaskToSubgroup(proj.SubGroups[0].Tasks[0].ID, proj.SubGroups[1].ID); err != nil {
		t.Fatalf("move task to subgroup: %v", err)
	}
	moved, err := c.ResolveTask(proj.SubGroups[0].Tasks[0].ID)
	if err != nil {
		t.Fatalf("resolve moved task: %v", err)
	}
	if moved.SubgroupID != proj.SubGroups[1].ID {
		t.Fatalf("expected moved task in target subgroup, got %+v", moved)
	}

	if err := c.DeleteSubgroup(proj.ID, sg.Subgroup.ID); err != nil {
		t.Fatalf("delete empty subgroup: %v", err)
	}
	if _, err := c.ResolveSubgroup(proj.ID, sg.Subgroup.ID); err == nil {
		t.Fatalf("expected deleted subgroup to be gone")
	}
}

func TestMoveTaskCoreAPIs(t *testing.T) {
	c := newTestCore(t)
	inbox, err := c.store.ReadList(model.ListIn)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	inboxTask := model.Task{ID: id.New(), Created: time.Now(), Text: "Inbox task"}
	inbox.Tasks = append(inbox.Tasks, inboxTask)
	if err := c.store.WriteList(inbox); err != nil {
		t.Fatalf("write inbox: %v", err)
	}

	actions, err := c.store.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatalf("read actions: %v", err)
	}
	actionTask := model.Task{ID: id.New(), Created: time.Now(), Text: "Action task", State: model.StateNextAction}
	actions.Tasks = append(actions.Tasks, actionTask)
	if err := c.store.WriteList(actions); err != nil {
		t.Fatalf("write actions: %v", err)
	}

	proj := &model.Project{
		Title: "Launch",
		ID:    id.New(),
		State: model.StateActive,
		Tags:  []string{"project"},
		SubGroups: []model.SubGroup{{Title: "Tasks", ID: id.New()}},
	}
	if err := c.store.WriteProject(proj); err != nil {
		t.Fatalf("write project: %v", err)
	}

	movedToActions, err := c.MoveTaskToList(inboxTask.ID, model.ListSingleActions, model.StateNextAction)
	if err != nil {
		t.Fatalf("move inbox to actions: %v", err)
	}
	if movedToActions.Kind != TaskLocationActions || movedToActions.ListType != model.ListSingleActions {
		t.Fatalf("unexpected inbox->actions move result: %+v", movedToActions)
	}

	movedToProject, err := c.MoveTaskToProject(actionTask.ID, proj.ID, proj.SubGroups[0].ID, model.StateNextAction)
	if err != nil {
		t.Fatalf("move actions to project: %v", err)
	}
	if movedToProject.Kind != TaskLocationProject || movedToProject.ProjectID != proj.ID || movedToProject.SubgroupID != proj.SubGroups[0].ID {
		t.Fatalf("unexpected actions->project move result: %+v", movedToProject)
	}

	movedInboxToProject, err := c.MoveTaskToProject(inboxTask.ID, proj.ID, proj.SubGroups[0].ID, model.StateSomeday)
	if err != nil {
		t.Fatalf("move actions->project after first move: %v", err)
	}
	if movedInboxToProject.Kind != TaskLocationProject || movedInboxToProject.Task.State != model.StateSomeday {
		t.Fatalf("unexpected move-to-project state result: %+v", movedInboxToProject)
	}
}

func TestInboxSessionLifecycle(t *testing.T) {
	c := newTestCore(t)
	inbox, err := c.store.ReadList(model.ListIn)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	inbox.Tasks = append(inbox.Tasks,
		model.Task{ID: id.New(), Created: time.Now(), Text: "First"},
		model.Task{ID: id.New(), Created: time.Now(), Text: "Second"},
	)
	if err := c.store.WriteList(inbox); err != nil {
		t.Fatalf("write inbox: %v", err)
	}

	session, err := c.StartInboxSession()
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected session ID")
	}
	if session.Done {
		t.Fatalf("expected active session")
	}
	if session.Progress.Current != 1 || session.Progress.Total != 2 {
		t.Fatalf("unexpected progress: %+v", session.Progress)
	}
	if session.Current.Step != InboxStepActionable || session.Current.Original.Text != "First" || session.Current.Draft.Text != "First" {
		t.Fatalf("unexpected current item: %+v", session.Current)
	}

	next, err := c.SkipInboxItem(session.ID)
	if err != nil {
		t.Fatalf("skip item: %v", err)
	}
	if next.Progress.Current != 2 || next.Progress.Total != 2 {
		t.Fatalf("unexpected progress after skip: %+v", next.Progress)
	}
	if next.Summary.Skipped != 1 {
		t.Fatalf("expected skipped count to increment, got %+v", next.Summary)
	}
	if next.Current.Original.Text != "Second" {
		t.Fatalf("expected second item after skip, got %+v", next.Current)
	}

	doneSession, err := c.SkipInboxItem(session.ID)
	if err != nil {
		t.Fatalf("skip final item: %v", err)
	}
	if !doneSession.Done || doneSession.Current.Step != InboxStepComplete {
		t.Fatalf("expected completed session, got %+v", doneSession)
	}
	if doneSession.Summary.Skipped != 2 {
		t.Fatalf("expected both items skipped, got %+v", doneSession.Summary)
	}

	if err := c.DiscardInboxSession(session.ID); err != nil {
		t.Fatalf("discard session: %v", err)
	}
	if _, err := c.GetInboxSession(session.ID); err == nil {
		t.Fatalf("expected discarded session lookup to fail")
	}
}

func timePtr(t time.Time) *time.Time { return &t }
