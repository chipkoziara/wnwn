package service

import (
	"testing"

	"github.com/wnwn/wnwn/internal/model"
	"github.com/wnwn/wnwn/internal/store"
)

func TestCollectAllTasks_Empty(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	results, err := svc.CollectAllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty store, got %d", len(results))
	}
}

func TestCollectAllTasks_ListsAndProjects(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	// Add to inbox.
	_, err := svc.AddToInbox("Inbox task")
	if err != nil {
		t.Fatal(err)
	}

	// Add to single-actions.
	inbox, err := svc.Store.ReadList(model.ListIn)
	if err != nil {
		t.Fatal(err)
	}
	inboxTask := inbox.Tasks[0]
	if err := svc.MoveToList(model.ListIn, inboxTask.ID, model.ListSingleActions, model.StateNextAction); err != nil {
		t.Fatal(err)
	}

	// Add another inbox task.
	_, err = svc.AddToInbox("Second inbox task")
	if err != nil {
		t.Fatal(err)
	}

	// Create a project with a task.
	_, err = svc.CreateProject("My Project", "Planning")
	if err != nil {
		t.Fatal(err)
	}
	projFilename := store.Slugify("My Project") + ".md"
	_, err = svc.AddTaskToProject(projFilename, 0, "Project task", model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	results, err := svc.CollectAllTasks()
	if err != nil {
		t.Fatal(err)
	}

	// Expect: 1 inbox task + 1 single-actions task + 1 project task = 3
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Check provenance is set correctly.
	sourceSet := map[string]int{}
	for _, vt := range results {
		sourceSet[vt.Source]++
	}

	if sourceSet["in"] != 1 {
		t.Errorf("expected 1 inbox task, got %d", sourceSet["in"])
	}
	if sourceSet["single-actions"] != 1 {
		t.Errorf("expected 1 single-actions task, got %d", sourceSet["single-actions"])
	}

	var projSource string
	for src := range sourceSet {
		if src != "in" && src != "single-actions" {
			projSource = src
		}
	}
	if projSource == "" {
		t.Error("no project source found")
	}
	if sourceSet[projSource] != 1 {
		t.Errorf("expected 1 project task, got %d", sourceSet[projSource])
	}

	// Verify IsProject flag.
	for _, vt := range results {
		if vt.IsProject && vt.Filename == "" {
			t.Error("project task should have non-empty Filename")
		}
		if !vt.IsProject && vt.SgIdx != -1 {
			t.Errorf("list task should have SgIdx=-1, got %d", vt.SgIdx)
		}
	}
}

func TestCollectAllTasks_MultipleProjects(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	// Create two projects with tasks.
	svc.CreateProject("Alpha Project", "Work")
	svc.CreateProject("Beta Project", "Work")
	f1 := store.Slugify("Alpha Project") + ".md"
	f2 := store.Slugify("Beta Project") + ".md"
	svc.AddTaskToProject(f1, 0, "Alpha task 1", model.StateNextAction)
	svc.AddTaskToProject(f1, 0, "Alpha task 2", model.StateNextAction)
	svc.AddTaskToProject(f2, 0, "Beta task", model.StateWaitingFor)

	results, err := svc.CollectAllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 total tasks, got %d", len(results))
	}
}

func TestCollectAllTasks_SourceFormat(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	_, _ = svc.AddToInbox("In task")
	svc.CreateProject("Launch Website", "Design")
	lwFilename := store.Slugify("Launch Website") + ".md"
	svc.AddTaskToProject(lwFilename, 0, "Design task", model.StateNextAction)

	results, err := svc.CollectAllTasks()
	if err != nil {
		t.Fatal(err)
	}

	for _, vt := range results {
		if !vt.IsProject && vt.Source != "in" && vt.Source != "single-actions" {
			t.Errorf("unexpected list source: %q", vt.Source)
		}
		if vt.IsProject {
			expected := "projects/" + lwFilename
			if vt.Source != expected {
				t.Errorf("project source = %q, want %q", vt.Source, expected)
			}
		}
	}
}
