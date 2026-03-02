package service

import (
	"testing"

	"github.com/wnwn/wnwn/internal/model"
)

func TestRefileInboxToSingleActions(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	// Add a task to inbox.
	task, err := svc.AddToInbox("Buy new running shoes")
	if err != nil {
		t.Fatal(err)
	}

	// Refile to single-actions as next-action.
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

	// Single actions should have the task with next-action state.
	sa, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Tasks) != 1 {
		t.Fatalf("single-actions has %d tasks, want 1", len(sa.Tasks))
	}
	if sa.Tasks[0].Text != "Buy new running shoes" {
		t.Errorf("Text = %q", sa.Tasks[0].Text)
	}
	if sa.Tasks[0].State != model.StateNextAction {
		t.Errorf("State = %q, want next-action", sa.Tasks[0].State)
	}
}

func TestRefileInboxToSomeday(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	task, err := svc.AddToInbox("Learn piano")
	if err != nil {
		t.Fatal(err)
	}

	// Refile from inbox to single-actions as someday/maybe.
	err = svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateSomeday)
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
	if sa.Tasks[0].State != model.StateSomeday {
		t.Errorf("State = %q, want some-day/maybe", sa.Tasks[0].State)
	}
}

func TestUpdateStateInPlace(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	// Add and refile a task.
	task, err := svc.AddToInbox("Get report")
	if err != nil {
		t.Fatal(err)
	}
	err = svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateNextAction)
	if err != nil {
		t.Fatal(err)
	}

	// Change state to someday in-place on single-actions.
	err = svc.UpdateState(model.ListSingleActions, task.ID, model.StateSomeday)
	if err != nil {
		t.Fatal(err)
	}

	sa, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	if sa.Tasks[0].State != model.StateSomeday {
		t.Errorf("State = %q", sa.Tasks[0].State)
	}
}
