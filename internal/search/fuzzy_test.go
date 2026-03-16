package search

import (
	"testing"
	"time"

	"github.com/chipkoziara/wnwn/internal/model"
	"github.com/chipkoziara/wnwn/internal/service"
)

func vt(text, notes, source string, tags ...string) service.ViewTask {
	return service.ViewTask{
		Task: model.Task{
			Text:    text,
			Notes:   notes,
			Tags:    tags,
			Created: time.Now(),
		},
		Source: source,
	}
}

func TestRankPrefersTaskText(t *testing.T) {
	tasks := []service.ViewTask{
		vt("Read article on distributed systems", "", "inbox"),
		vt("Book flights", "Read that distributed systems article", "inbox"),
	}
	results := Rank(tasks, "distributed")
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Task.Text != "Read article on distributed systems" {
		t.Fatalf("top result = %q, want task text match first", results[0].Task.Text)
	}
}

func TestRankMatchesTagsAndSource(t *testing.T) {
	tasks := []service.ViewTask{
		vt("Buy milk", "", "projects/home-admin.md", "@errands"),
		vt("Plan trip", "", "projects/travel.md", "travel"),
	}
	if got := Rank(tasks, "@errands"); len(got) != 1 || got[0].Task.Text != "Buy milk" {
		t.Fatalf("tag match failed: %+v", got)
	}
	if got := Rank(tasks, "travel"); len(got) == 0 {
		t.Fatalf("expected source/tag match for travel")
	}
}

func TestRankSubsequenceFallback(t *testing.T) {
	tasks := []service.ViewTask{
		vt("Book flights to Hawaii", "", "inbox"),
	}
	results := Rank(tasks, "bfh")
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
}
