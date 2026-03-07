package service

import (
	"testing"
	"time"

	"github.com/wnwn/wnwn/internal/model"
)

func TestWeeklyReviewGroups(t *testing.T) {
	s := setupTestStore(t)
	svc := New(s)

	// Waiting-for older than 7 days.
	waitTask, err := svc.AddToInbox("Waiting on contractor")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveToList(model.ListIn, waitTask.ID, model.ListSingleActions, model.StateWaitingFor); err != nil {
		t.Fatal(err)
	}
	actions, err := s.ReadList(model.ListSingleActions)
	if err != nil {
		t.Fatal(err)
	}
	for i := range actions.Tasks {
		if actions.Tasks[i].ID == waitTask.ID {
			ts := time.Now().AddDate(0, 0, -10)
			actions.Tasks[i].WaitingSince = &ts
		}
	}
	if err := s.WriteList(actions); err != nil {
		t.Fatal(err)
	}

	// Someday task.
	somedayTask, err := svc.AddToInbox("Maybe learn zig")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveToList(model.ListIn, somedayTask.ID, model.ListSingleActions, model.StateSomeday); err != nil {
		t.Fatal(err)
	}

	// Project with no next-action.
	_, err = svc.CreateProject("Website Refresh", "Plan")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTaskToProject("website-refresh.md", 0, "Collect inspiration", model.StateWaitingFor); err != nil {
		t.Fatal(err)
	}

	// Recently archived task.
	archiveTask, err := svc.AddToInbox("Done thing")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveToList(model.ListIn, archiveTask.ID, model.ListSingleActions, model.StateDone); err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveTask(model.ListSingleActions, archiveTask.ID); err != nil {
		t.Fatal(err)
	}

	review, err := svc.WeeklyReview(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(review.ProjectsWithoutNextAction) != 1 {
		t.Fatalf("projects without next action = %d, want 1", len(review.ProjectsWithoutNextAction))
	}
	if len(review.AgingWaitingFor) == 0 {
		t.Fatal("expected at least one aging waiting-for task")
	}
	if len(review.SomedayMaybe) == 0 {
		t.Fatal("expected at least one someday/maybe task")
	}
	if len(review.RecentArchived) == 0 {
		t.Fatal("expected at least one recent archived task")
	}
}
