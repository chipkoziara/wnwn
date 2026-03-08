package service

import (
	"testing"
	"time"

	"github.com/wnwn/wnwn/internal/model"
	"github.com/wnwn/wnwn/internal/store"
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
	if task.ModifiedAt == nil {
		t.Fatal("ModifiedAt is nil")
	}
	if !task.ModifiedAt.Equal(task.Created) {
		t.Errorf("ModifiedAt = %v, want %v", task.ModifiedAt, task.Created)
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

func TestUpdateStateDoneStaysInList(t *testing.T) {
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

	// Mark done — should stay in list until explicitly archived.
	err = svc.UpdateState(model.ListSingleActions, task.ID, model.StateDone)
	if err != nil {
		t.Fatal(err)
	}

	// Single actions should still contain the task.
	sa, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Tasks) != 1 {
		t.Fatalf("single-actions has %d tasks, want 1", len(sa.Tasks))
	}
	if sa.Tasks[0].State != model.StateDone {
		t.Errorf("state = %q, want done", sa.Tasks[0].State)
	}
}

func TestUpdateStateTouchesModifiedAt(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	task, err := svc.AddToInbox("Touch modified")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateNextAction); err != nil {
		t.Fatal(err)
	}

	sa, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Tasks) != 1 || sa.Tasks[0].ModifiedAt == nil {
		t.Fatal("expected one task with ModifiedAt set")
	}
	before := *sa.Tasks[0].ModifiedAt

	time.Sleep(1100 * time.Millisecond)
	if err := svc.UpdateState(model.ListSingleActions, task.ID, model.StateDone); err != nil {
		t.Fatal(err)
	}

	sa, err = s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	after := sa.Tasks[0].ModifiedAt
	if after == nil {
		t.Fatal("ModifiedAt is nil after update")
	}
	if !after.After(before) {
		t.Errorf("ModifiedAt did not advance: before=%v after=%v", before, *after)
	}
}

func TestArchiveTask(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	task, err := svc.AddToInbox("Archive me")
	if err != nil {
		t.Fatal(err)
	}
	err = svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateDone)
	if err != nil {
		t.Fatal(err)
	}

	err = svc.ArchiveTask(model.ListSingleActions, task.ID)
	if err != nil {
		t.Fatal(err)
	}

	sa, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Tasks) != 0 {
		t.Fatalf("single-actions has %d tasks, want 0", len(sa.Tasks))
	}

	archiveFile := "archive.md"
	archive, err := s.ReadArchive(archiveFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Tasks) != 1 {
		t.Fatalf("archive has %d tasks, want 1", len(archive.Tasks))
	}
	if archive.Tasks[0].ID != task.ID {
		t.Errorf("archived ID = %q, want %q", archive.Tasks[0].ID, task.ID)
	}
	if archive.Tasks[0].Source != "single-actions" {
		t.Errorf("archived source = %q, want single-actions", archive.Tasks[0].Source)
	}
	if archive.Tasks[0].ArchivedAt == nil {
		t.Error("ArchivedAt is nil, expected archive timestamp")
	}
}

func TestUpdateStateDoneAutoArchiveWhenEnabled(t *testing.T) {
	s := setupTestStore(t)
	svc := NewWithBehavior(s, BehaviorConfig{AutoArchiveDone: true})

	task, err := svc.AddToInbox("Auto archive done")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateNextAction); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateState(model.ListSingleActions, task.ID, model.StateDone); err != nil {
		t.Fatal(err)
	}

	sa, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Tasks) != 0 {
		t.Fatalf("single-actions has %d tasks, want 0", len(sa.Tasks))
	}

	archive, err := s.ReadArchive("archive.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Tasks) != 1 {
		t.Fatalf("archive has %d tasks, want 1", len(archive.Tasks))
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
	archive, err := s.ReadArchive("archive.md")
	if err == nil && len(archive.Tasks) > 0 {
		t.Errorf("archive has %d tasks, want 0", len(archive.Tasks))
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
