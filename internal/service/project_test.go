package service

import (
	"testing"

	"github.com/wnwn/wnwn/internal/model"
)

func TestCreateProject(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	proj, err := svc.CreateProject("Launch Website", "Design")
	if err != nil {
		t.Fatal(err)
	}

	if proj.Title != "Launch Website" {
		t.Errorf("Title = %q", proj.Title)
	}
	if proj.ID == "" {
		t.Error("ID is empty")
	}
	if proj.State != model.StateActive {
		t.Errorf("State = %q, want %q", proj.State, model.StateActive)
	}
	if len(proj.Tags) != 1 || proj.Tags[0] != "project" {
		t.Errorf("Tags = %v", proj.Tags)
	}
	if len(proj.SubGroups) != 1 {
		t.Fatalf("SubGroups count = %d, want 1", len(proj.SubGroups))
	}
	if proj.SubGroups[0].Title != "Design" {
		t.Errorf("SubGroup title = %q", proj.SubGroups[0].Title)
	}

	// Verify persisted.
	filenames, err := s.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(filenames) != 1 {
		t.Fatalf("got %d project files, want 1", len(filenames))
	}
	if filenames[0] != "launch-website.md" {
		t.Errorf("filename = %q", filenames[0])
	}
}

func TestCreateProjectNoSubGroup(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	proj, err := svc.CreateProject("Simple Project", "")
	if err != nil {
		t.Fatal(err)
	}

	if len(proj.SubGroups) != 0 {
		t.Errorf("SubGroups count = %d, want 0", len(proj.SubGroups))
	}
}

func TestListProjects(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("Project Alpha", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateProject("Project Beta", "Setup")
	if err != nil {
		t.Fatal(err)
	}

	summaries, err := svc.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("got %d summaries, want 2", len(summaries))
	}
}

func TestAddTaskToProject(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	proj, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}

	task, err := svc.AddTaskToProject("my-project.md", 0, "Do the thing", model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}
	if task.Text != "Do the thing" {
		t.Errorf("Text = %q", task.Text)
	}
	if task.State != model.StateNextAction {
		t.Errorf("State = %q", task.State)
	}

	// Verify persisted.
	reread, err := s.ReadProject("my-project.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(reread.SubGroups[0].Tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(reread.SubGroups[0].Tasks))
	}

	_ = proj // used for creation
}

func TestAddSubGroup(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}

	sg, err := svc.AddSubGroup("my-project.md", "Phase 2")
	if err != nil {
		t.Fatal(err)
	}
	if sg.Title != "Phase 2" {
		t.Errorf("Title = %q", sg.Title)
	}

	proj, err := s.ReadProject("my-project.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.SubGroups) != 2 {
		t.Fatalf("got %d sub-groups, want 2", len(proj.SubGroups))
	}
}

func TestRenameSubGroup(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.RenameSubGroup("my-project.md", 0, "Planning"); err != nil {
		t.Fatal(err)
	}

	proj, err := s.ReadProject("my-project.md")
	if err != nil {
		t.Fatal(err)
	}
	if proj.SubGroups[0].Title != "Planning" {
		t.Fatalf("title = %q, want %q", proj.SubGroups[0].Title, "Planning")
	}
}

func TestDeleteSubGroupEmpty(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddSubGroup("my-project.md", "Phase 2")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteSubGroup("my-project.md", 1); err != nil {
		t.Fatal(err)
	}

	proj, err := s.ReadProject("my-project.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.SubGroups) != 1 {
		t.Fatalf("sub-groups = %d, want 1", len(proj.SubGroups))
	}
}

func TestDeleteSubGroupWithTasksFails(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddTaskToProject("my-project.md", 0, "Keep task", model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteSubGroup("my-project.md", 0); err == nil {
		t.Fatal("expected error when deleting non-empty sub-group")
	}
}

func TestMoveToProject(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	// Add a task to inbox.
	task, err := svc.AddToInbox("Design the homepage")
	if err != nil {
		t.Fatal(err)
	}

	// Create a project.
	_, err = svc.CreateProject("Launch Website", "Design")
	if err != nil {
		t.Fatal(err)
	}

	// Move from inbox to project.
	err = svc.MoveToProject(model.ListIn, task.ID, "launch-website.md", 0, model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	// Inbox should be empty.
	inbox, err := s.ReadList(model.ListIn)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Tasks) != 0 {
		t.Errorf("inbox has %d tasks, want 0", len(inbox.Tasks))
	}

	// Project should have the task.
	proj, err := s.ReadProject("launch-website.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.SubGroups[0].Tasks) != 1 {
		t.Fatalf("project has %d tasks, want 1", len(proj.SubGroups[0].Tasks))
	}
	if proj.SubGroups[0].Tasks[0].Text != "Design the homepage" {
		t.Errorf("Text = %q", proj.SubGroups[0].Tasks[0].Text)
	}
	if proj.SubGroups[0].Tasks[0].State != model.StateNextAction {
		t.Errorf("State = %q", proj.SubGroups[0].Tasks[0].State)
	}
}

func TestUpdateProjectTaskStateDoneStaysInProject(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}

	task, err := svc.AddTaskToProject("my-project.md", 0, "Finish docs", model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	// Mark done — should stay in project until explicitly archived.
	err = svc.UpdateProjectTaskState("my-project.md", 0, task.ID, model.StateDone)
	if err != nil {
		t.Fatal(err)
	}

	proj, err := s.ReadProject("my-project.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.SubGroups[0].Tasks) != 1 {
		t.Fatalf("project has %d tasks, want 1", len(proj.SubGroups[0].Tasks))
	}
	if proj.SubGroups[0].Tasks[0].State != model.StateDone {
		t.Errorf("state = %q, want done", proj.SubGroups[0].Tasks[0].State)
	}
}

func TestArchiveProjectTask(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}

	task, err := svc.AddTaskToProject("my-project.md", 0, "Archive this project task", model.StateDone)
	if err != nil {
		t.Fatal(err)
	}

	err = svc.ArchiveProjectTask("my-project.md", 0, task.ID)
	if err != nil {
		t.Fatal(err)
	}

	proj, err := s.ReadProject("my-project.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.SubGroups[0].Tasks) != 0 {
		t.Fatalf("project has %d tasks, want 0", len(proj.SubGroups[0].Tasks))
	}

	archiveFile := "archive.md"
	archive, err := s.ReadArchive(archiveFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Tasks) != 1 {
		t.Fatalf("archive has %d tasks, want 1", len(archive.Tasks))
	}
	if archive.Tasks[0].Source != "projects/my-project.md" {
		t.Errorf("archived source = %q", archive.Tasks[0].Source)
	}
	if archive.Tasks[0].ArchivedAt == nil {
		t.Error("ArchivedAt is nil, expected archive timestamp")
	}
}

func TestUpdateProjectTaskStateAutoArchiveWhenEnabled(t *testing.T) {
	s := setupTestStore(t)
	svc := NewWithBehavior(s, BehaviorConfig{AutoArchiveDone: true})

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}

	task, err := svc.AddTaskToProject("my-project.md", 0, "Auto archive project task", model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.UpdateProjectTaskState("my-project.md", 0, task.ID, model.StateDone); err != nil {
		t.Fatal(err)
	}

	proj, err := s.ReadProject("my-project.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.SubGroups[0].Tasks) != 0 {
		t.Fatalf("project has %d tasks, want 0", len(proj.SubGroups[0].Tasks))
	}

	archive, err := s.ReadArchive("archive.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Tasks) != 1 {
		t.Fatalf("archive has %d tasks, want 1", len(archive.Tasks))
	}
}

func TestTrashProjectTask(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}

	task, err := svc.AddTaskToProject("my-project.md", 0, "Trash this task", model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.TrashProjectTask("my-project.md", 0, task.ID); err != nil {
		t.Fatal(err)
	}

	proj, err := s.ReadProject("my-project.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.SubGroups[0].Tasks) != 0 {
		t.Fatalf("project has %d tasks, want 0", len(proj.SubGroups[0].Tasks))
	}
}

func TestListProjectsSummaryWithNextAction(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AddTaskToProject("my-project.md", 0, "First task", model.StateEmpty)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddTaskToProject("my-project.md", 0, "Next action task", model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	summaries, err := svc.ListProjects()
	if err != nil {
		t.Fatal(err)
	}

	if len(summaries) != 1 {
		t.Fatalf("got %d summaries", len(summaries))
	}
	if summaries[0].TaskCount != 2 {
		t.Errorf("TaskCount = %d, want 2", summaries[0].TaskCount)
	}
	if summaries[0].NextAction != "Next action task" {
		t.Errorf("NextAction = %q", summaries[0].NextAction)
	}
}

func TestReorderTaskInSubGroup(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}

	t1, _ := svc.AddTaskToProject("my-project.md", 0, "First", model.StateNextAction)
	t2, _ := svc.AddTaskToProject("my-project.md", 0, "Second", model.StateNextAction)
	t3, _ := svc.AddTaskToProject("my-project.md", 0, "Third", model.StateNextAction)

	// Move "Third" up one position.
	err = svc.ReorderTaskInSubGroup("my-project.md", 0, t3.ID, -1)
	if err != nil {
		t.Fatal(err)
	}

	proj, _ := s.ReadProject("my-project.md")
	tasks := proj.SubGroups[0].Tasks
	if len(tasks) != 3 {
		t.Fatalf("got %d tasks", len(tasks))
	}
	if tasks[0].ID != t1.ID || tasks[1].ID != t3.ID || tasks[2].ID != t2.ID {
		t.Errorf("order: %s, %s, %s — want %s, %s, %s",
			tasks[0].Text, tasks[1].Text, tasks[2].Text,
			"First", "Third", "Second")
	}

	// Move "First" down one position.
	err = svc.ReorderTaskInSubGroup("my-project.md", 0, t1.ID, 1)
	if err != nil {
		t.Fatal(err)
	}

	proj, _ = s.ReadProject("my-project.md")
	tasks = proj.SubGroups[0].Tasks
	if tasks[0].ID != t3.ID || tasks[1].ID != t1.ID || tasks[2].ID != t2.ID {
		t.Errorf("order: %s, %s, %s — want Third, First, Second",
			tasks[0].Text, tasks[1].Text, tasks[2].Text)
	}

	// Move at boundary (top) — should be a no-op.
	err = svc.ReorderTaskInSubGroup("my-project.md", 0, t3.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	proj, _ = s.ReadProject("my-project.md")
	if proj.SubGroups[0].Tasks[0].ID != t3.ID {
		t.Error("boundary move should be no-op")
	}
}

func TestMoveTaskBetweenSubGroups(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddSubGroup("my-project.md", "Phase 2")
	if err != nil {
		t.Fatal(err)
	}

	task, _ := svc.AddTaskToProject("my-project.md", 0, "Moveable task", model.StateNextAction)

	// Move from Phase 1 (idx 0) to Phase 2 (idx 1).
	err = svc.MoveTaskBetweenSubGroups("my-project.md", 0, task.ID, 1)
	if err != nil {
		t.Fatal(err)
	}

	proj, _ := s.ReadProject("my-project.md")
	if len(proj.SubGroups[0].Tasks) != 0 {
		t.Errorf("Phase 1 has %d tasks, want 0", len(proj.SubGroups[0].Tasks))
	}
	if len(proj.SubGroups[1].Tasks) != 1 {
		t.Fatalf("Phase 2 has %d tasks, want 1", len(proj.SubGroups[1].Tasks))
	}
	if proj.SubGroups[1].Tasks[0].Text != "Moveable task" {
		t.Errorf("Text = %q", proj.SubGroups[1].Tasks[0].Text)
	}
}

func TestMoveTaskBetweenSubGroupsSameGroup(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("My Project", "Phase 1")
	if err != nil {
		t.Fatal(err)
	}

	task, _ := svc.AddTaskToProject("my-project.md", 0, "Stay put", model.StateNextAction)

	// Move to same sub-group — should be a no-op.
	err = svc.MoveTaskBetweenSubGroups("my-project.md", 0, task.ID, 0)
	if err != nil {
		t.Fatal(err)
	}

	proj, _ := s.ReadProject("my-project.md")
	if len(proj.SubGroups[0].Tasks) != 1 {
		t.Errorf("Phase 1 has %d tasks, want 1", len(proj.SubGroups[0].Tasks))
	}
}

func TestMoveToProjectFromSingleActions(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	// Add task to inbox, refile to single-actions first.
	task, err := svc.AddToInbox("Refiled task")
	if err != nil {
		t.Fatal(err)
	}
	err = svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	// Create a project.
	_, err = svc.CreateProject("Target Project", "Tasks")
	if err != nil {
		t.Fatal(err)
	}

	// Move from single-actions to project.
	err = svc.MoveToProject(model.ListSingleActions, task.ID, "target-project.md", 0, model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	// Single-actions should be empty.
	sa, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Tasks) != 0 {
		t.Errorf("single-actions has %d tasks, want 0", len(sa.Tasks))
	}

	// Project should have the task.
	proj, err := s.ReadProject("target-project.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.SubGroups[0].Tasks) != 1 {
		t.Fatalf("project has %d tasks, want 1", len(proj.SubGroups[0].Tasks))
	}
	if proj.SubGroups[0].Tasks[0].Text != "Refiled task" {
		t.Errorf("Text = %q", proj.SubGroups[0].Tasks[0].Text)
	}
}

func TestRestoreArchivedTaskToProjectSource(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, err := svc.CreateProject("Restore Project", "Todo")
	if err != nil {
		t.Fatal(err)
	}
	task, err := svc.AddTaskToProject("restore-project.md", 0, "Archived project task", model.StateDone)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveProjectTask("restore-project.md", 0, task.ID); err != nil {
		t.Fatal(err)
	}

	dest, err := svc.RestoreArchivedTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dest != "Project: Restore Project" {
		t.Fatalf("destination = %q, want %q", dest, "Project: Restore Project")
	}

	proj, err := s.ReadProject("restore-project.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.SubGroups[0].Tasks) != 1 {
		t.Fatalf("project tasks = %d, want 1", len(proj.SubGroups[0].Tasks))
	}
	if proj.SubGroups[0].Tasks[0].ID != task.ID {
		t.Fatalf("restored task ID = %q, want %q", proj.SubGroups[0].Tasks[0].ID, task.ID)
	}
}
