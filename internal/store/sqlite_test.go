package store

import (
	"testing"
	"time"

	"github.com/wnwn/wnwn/internal/model"
)

func TestSQLiteInitAndDefaultLists(t *testing.T) {
	s := NewWithBackend(t.TempDir(), BackendSQLite)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	inbox, err := s.ReadList(model.ListIn)
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Title != "Inbox" {
		t.Fatalf("inbox title = %q", inbox.Title)
	}
	if len(inbox.Tasks) != 0 {
		t.Fatalf("inbox tasks = %d, want 0", len(inbox.Tasks))
	}
}

func TestSQLiteProjectRoundTrip(t *testing.T) {
	s := NewWithBackend(t.TempDir(), BackendSQLite)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	d := time.Date(2026, 3, 20, 9, 0, 0, 0, time.UTC)
	p := &model.Project{
		Title: "Launch Website",
		ID:    "P1",
		State: model.StateActive,
		Tags:  []string{"project", "@office"},
		SubGroups: []model.SubGroup{
			{
				Title: "Design",
				ID:    "SG1",
				Tasks: []model.Task{
					{
						ID:       "T1",
						Created:  d,
						Text:     "Create mockups",
						State:    model.StateNextAction,
						Deadline: &d,
						Tags:     []string{"@computer"},
					},
				},
			},
		},
	}

	if err := s.WriteProject(p); err != nil {
		t.Fatal(err)
	}

	r, err := s.ReadProject("launch-website.md")
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != p.Title || r.ID != p.ID {
		t.Fatalf("project mismatch: got %q/%q", r.Title, r.ID)
	}
	if len(r.SubGroups) != 1 || len(r.SubGroups[0].Tasks) != 1 {
		t.Fatalf("unexpected subgroup/task counts: %+v", r.SubGroups)
	}
	if r.SubGroups[0].Tasks[0].Text != "Create mockups" {
		t.Fatalf("task text = %q", r.SubGroups[0].Tasks[0].Text)
	}
}
