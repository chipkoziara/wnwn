package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g-tuddy/g-tuddy/internal/model"
	"github.com/g-tuddy/g-tuddy/internal/store"
)

func setupTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s := store.New(dir)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAddToInbox(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	task, err := svc.AddToInbox("Buy milk")
	if err != nil {
		t.Fatal(err)
	}

	if task.Text != "Buy milk" {
		t.Errorf("Text = %q", task.Text)
	}
	if task.ID == "" {
		t.Error("ID is empty")
	}
	if task.Created.IsZero() {
		t.Error("Created is zero")
	}
	if task.State != model.StateEmpty {
		t.Errorf("State = %q, want empty", task.State)
	}

	// Verify it's persisted.
	list, err := s.ReadList(model.ListIn)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tasks) != 1 {
		t.Fatalf("inbox has %d tasks, want 1", len(list.Tasks))
	}
	if list.Tasks[0].ID != task.ID {
		t.Errorf("persisted ID = %q, want %q", list.Tasks[0].ID, task.ID)
	}
}

func TestAddToInboxWithOptions(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	deadline := time.Date(2026, 3, 15, 17, 0, 0, 0, time.UTC)
	task, err := svc.AddToInbox("Book flights",
		WithDeadline(deadline),
		WithTags([]string{"travel", "@computer"}),
		WithURL("https://flights.example.com"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if task.Deadline == nil || !task.Deadline.Equal(deadline) {
		t.Errorf("Deadline = %v, want %v", task.Deadline, deadline)
	}
	if len(task.Tags) != 2 {
		t.Errorf("Tags = %v", task.Tags)
	}
	if task.URL != "https://flights.example.com" {
		t.Errorf("URL = %q", task.URL)
	}

	// Verify round-trip through file.
	list, err := s.ReadList(model.ListIn)
	if err != nil {
		t.Fatal(err)
	}
	persisted := list.Tasks[0]
	if persisted.URL != "https://flights.example.com" {
		t.Errorf("persisted URL = %q", persisted.URL)
	}
	if len(persisted.Tags) != 2 {
		t.Errorf("persisted Tags = %v", persisted.Tags)
	}
}

func TestMoveToList(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	task, err := svc.AddToInbox("A single action")
	if err != nil {
		t.Fatal(err)
	}

	err = svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateNextAction)
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

	// Single actions should have the task.
	sa, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Tasks) != 1 {
		t.Fatalf("single-actions has %d tasks, want 1", len(sa.Tasks))
	}
	if sa.Tasks[0].State != model.StateNextAction {
		t.Errorf("State = %q, want next-action", sa.Tasks[0].State)
	}
}

func TestUpdateStateWaitingFor(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	task, err := svc.AddToInbox("Get report from Alice")
	if err != nil {
		t.Fatal(err)
	}

	// Move to single actions first.
	err = svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	// Update to waiting-for.
	err = svc.UpdateState(model.ListSingleActions, task.ID, model.StateWaitingFor)
	if err != nil {
		t.Fatal(err)
	}

	sa, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Tasks) != 1 {
		t.Fatalf("got %d tasks", len(sa.Tasks))
	}
	if sa.Tasks[0].State != model.StateWaitingFor {
		t.Errorf("State = %q", sa.Tasks[0].State)
	}
	if sa.Tasks[0].WaitingSince == nil {
		t.Error("WaitingSince is nil, should be auto-set")
	}
}

func TestUpdateStateDoneArchives(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	task, err := svc.AddToInbox("Do the thing")
	if err != nil {
		t.Fatal(err)
	}

	// Move to single actions.
	err = svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	// Mark done — should archive and remove from list.
	err = svc.UpdateState(model.ListSingleActions, task.ID, model.StateDone)
	if err != nil {
		t.Fatal(err)
	}

	// Single actions should be empty.
	sa, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Tasks) != 0 {
		t.Errorf("single-actions has %d tasks, want 0", len(sa.Tasks))
	}

	// Archive should have the task.
	archiveFile := time.Now().Format("2006-01") + ".md"
	archive, err := s.ReadArchive(archiveFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Tasks) != 1 {
		t.Fatalf("archive has %d tasks, want 1", len(archive.Tasks))
	}
	if archive.Tasks[0].State != model.StateDone {
		t.Errorf("archived state = %q", archive.Tasks[0].State)
	}
	if archive.Tasks[0].Source != "single-actions" {
		t.Errorf("archived source = %q, want single-actions", archive.Tasks[0].Source)
	}
}

func TestTrashTask(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	task, err := svc.AddToInbox("Trash me")
	if err != nil {
		t.Fatal(err)
	}

	err = svc.TrashTask(model.ListIn, task.ID)
	if err != nil {
		t.Fatal(err)
	}

	inbox, err := s.ReadList(model.ListIn)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Tasks) != 0 {
		t.Errorf("inbox has %d tasks, want 0", len(inbox.Tasks))
	}

	// Should NOT be in archive.
	archiveFile := time.Now().Format("2006-01") + ".md"
	archivePath := filepath.Join(s.Root, "archive", archiveFile)
	if _, err := os.Stat(archivePath); err == nil {
		t.Error("archive file exists, but trashed tasks should not be archived")
	}
}

func TestMoveNotFoundReturnsError(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	err := svc.MoveToList(model.ListIn, "nonexistent-id", model.ListSingleActions, model.StateNextAction)
	if err == nil {
		t.Error("expected error for nonexistent task, got nil")
	}
}

func TestMultipleAddsPreserveOrder(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	texts := []string{"First", "Second", "Third"}
	for _, text := range texts {
		if _, err := svc.AddToInbox(text); err != nil {
			t.Fatal(err)
		}
	}

	inbox, err := s.ReadList(model.ListIn)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(inbox.Tasks))
	}
	for i, text := range texts {
		if inbox.Tasks[i].Text != text {
			t.Errorf("task[%d].Text = %q, want %q", i, inbox.Tasks[i].Text, text)
		}
	}
}
