package query

import (
	"testing"
	"time"

	"github.com/wnwn/wnwn/internal/model"
)

func tp(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

func makeTask() model.Task {
	return model.Task{
		ID:        "01TEST",
		Created:   time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC),
		Text:      "Book flights to Hawaii",
		State:     model.StateNextAction,
		Tags:      []string{"travel", "@computer"},
		Deadline:  tp("2026-03-15"),
		URL:       "https://example.com",
		Notes:     "Check Southwest and United.",
		WaitingOn: "",
	}
}

// helper: parse + match
func match(t *testing.T, query string, task model.Task, source string) bool {
	t.Helper()
	clauses, err := Parse(query, testNow)
	if err != nil {
		t.Fatalf("Parse(%q): %v", query, err)
	}
	return MatchAll(clauses, task, source)
}

func TestMatchEmptyQueryMatchesEverything(t *testing.T) {
	task := makeTask()
	if !match(t, "", task, "single-actions") {
		t.Error("empty query should match everything")
	}
}

func TestMatchStateEq(t *testing.T) {
	task := makeTask()
	if !match(t, "state:next-action", task, "") {
		t.Error("state:next-action should match")
	}
	if match(t, "state:waiting-for", task, "") {
		t.Error("state:waiting-for should not match")
	}
}

func TestMatchStateWaitingFor(t *testing.T) {
	task := makeTask()
	task.State = model.StateWaitingFor
	if !match(t, "state:waiting-for", task, "") {
		t.Error("should match waiting-for")
	}
}

func TestMatchTagEq(t *testing.T) {
	task := makeTask()
	if !match(t, "tag:travel", task, "") {
		t.Error("tag:travel should match")
	}
	if !match(t, "tag:@computer", task, "") {
		t.Error("tag:@computer should match")
	}
	if match(t, "tag:@home", task, "") {
		t.Error("tag:@home should not match")
	}
}

func TestMatchBareAtTag(t *testing.T) {
	task := makeTask()
	if !match(t, "@computer", task, "") {
		t.Error("bare @computer should match")
	}
}

func TestMatchTagCaseInsensitive(t *testing.T) {
	task := makeTask()
	if !match(t, "tag:Travel", task, "") {
		t.Error("tag match should be case-insensitive")
	}
}

func TestMatchDeadlineLt(t *testing.T) {
	task := makeTask() // deadline 2026-03-15
	if !match(t, "deadline:<2026-04-01", task, "") {
		t.Error("deadline:<2026-04-01 should match 2026-03-15")
	}
	if match(t, "deadline:<2026-03-01", task, "") {
		t.Error("deadline:<2026-03-01 should not match 2026-03-15")
	}
}

func TestMatchDeadlineGt(t *testing.T) {
	task := makeTask() // deadline 2026-03-15
	if !match(t, "deadline:>2026-03-01", task, "") {
		t.Error("deadline:>2026-03-01 should match 2026-03-15")
	}
	if match(t, "deadline:>2026-04-01", task, "") {
		t.Error("deadline:>2026-04-01 should not match 2026-03-15")
	}
}

func TestMatchDeadlineEq(t *testing.T) {
	task := makeTask() // deadline 2026-03-15
	if !match(t, "deadline:2026-03-15", task, "") {
		t.Error("deadline:2026-03-15 should match")
	}
	if match(t, "deadline:2026-03-16", task, "") {
		t.Error("deadline:2026-03-16 should not match")
	}
}

func TestMatchDeadlineNil(t *testing.T) {
	task := makeTask()
	task.Deadline = nil
	if match(t, "deadline:<2026-04-01", task, "") {
		t.Error("no deadline should not match deadline:<...")
	}
}

func TestMatchScheduled(t *testing.T) {
	task := makeTask()
	sched := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	task.Scheduled = &sched
	if !match(t, "scheduled:<2026-03-15", task, "") {
		t.Error("scheduled:<2026-03-15 should match 2026-03-10")
	}
}

func TestMatchRelativeDateToday(t *testing.T) {
	task := makeTask()
	// deadline is 2026-03-15, testNow is 2026-03-01
	if !match(t, "deadline:>today", task, "") {
		t.Error("deadline:>today should match 2026-03-15 when today is 2026-03-01")
	}
}

func TestMatchRelativeDate7d(t *testing.T) {
	task := makeTask()
	// deadline 2026-03-15, today+7d = 2026-03-08; 2026-03-15 is not before 2026-03-08
	if match(t, "deadline:<7d", task, "") {
		t.Error("2026-03-15 should not be before today+7d (2026-03-08)")
	}
	// deadline 2026-03-05 would be before today+7d
	task.Deadline = tp("2026-03-05")
	if !match(t, "deadline:<7d", task, "") {
		t.Error("2026-03-05 should be before today+7d (2026-03-08)")
	}
}

func TestMatchHasDeadline(t *testing.T) {
	task := makeTask()
	if !match(t, "has:deadline", task, "") {
		t.Error("should match has:deadline when deadline is set")
	}
	task.Deadline = nil
	if match(t, "has:deadline", task, "") {
		t.Error("should not match has:deadline when deadline is nil")
	}
}

func TestMatchHasNotes(t *testing.T) {
	task := makeTask()
	if !match(t, "has:notes", task, "") {
		t.Error("should match has:notes")
	}
	task.Notes = ""
	if match(t, "has:notes", task, "") {
		t.Error("should not match has:notes when empty")
	}
}

func TestMatchHasUrl(t *testing.T) {
	task := makeTask()
	if !match(t, "has:url", task, "") {
		t.Error("should match has:url")
	}
}

func TestMatchHasWaitingOn(t *testing.T) {
	task := makeTask()
	task.WaitingOn = "Marvin"
	if !match(t, "has:waiting_on", task, "") {
		t.Error("should match has:waiting_on")
	}
	task.WaitingOn = ""
	if match(t, "has:waiting_on", task, "") {
		t.Error("should not match has:waiting_on when empty")
	}
}

func TestMatchWaitingOnSubstring(t *testing.T) {
	task := makeTask()
	task.WaitingOn = "Design Team"
	if !match(t, "waiting_on:design", task, "") {
		t.Error("waiting_on:design should substring-match 'Design Team'")
	}
	if match(t, "waiting_on:sarah", task, "") {
		t.Error("waiting_on:sarah should not match 'Design Team'")
	}
}

func TestMatchBareText(t *testing.T) {
	task := makeTask() // Text: "Book flights to Hawaii"
	if !match(t, "flights", task, "") {
		t.Error("bare text 'flights' should match task text")
	}
	if !match(t, "southwest", task, "") {
		t.Error("bare text 'southwest' should match task notes")
	}
	if match(t, "dentist", task, "") {
		t.Error("bare text 'dentist' should not match")
	}
}

func TestMatchTextCaseInsensitive(t *testing.T) {
	task := makeTask()
	if !match(t, "HAWAII", task, "") {
		t.Error("text match should be case-insensitive")
	}
}

func TestMatchProject(t *testing.T) {
	task := makeTask()
	if !match(t, "project:launch", task, "projects/launch-website.md") {
		t.Error("project:launch should match source 'projects/launch-website.md'")
	}
	if match(t, "project:launch", task, "in") {
		t.Error("project:launch should not match source 'in'")
	}
}

func TestMatchMultipleClausesImplicitAnd(t *testing.T) {
	task := makeTask()
	// state:next-action AND tag:travel — both true
	if !match(t, "state:next-action tag:travel", task, "") {
		t.Error("both clauses true should match")
	}
	// state:next-action AND tag:@home — second false
	if match(t, "state:next-action tag:@home", task, "") {
		t.Error("one false clause should cause overall false")
	}
}

func TestMatchTextFieldEq(t *testing.T) {
	task := makeTask()
	if !match(t, "text:hawaii", task, "") {
		t.Error("text:hawaii should substring-match task text")
	}
}

func TestMatchCreatedEq(t *testing.T) {
	task := makeTask() // Created: 2026-02-01
	if !match(t, "created:2026-02-01", task, "") {
		t.Error("created:2026-02-01 should match")
	}
	if match(t, "created:2026-03-01", task, "") {
		t.Error("created:2026-03-01 should not match")
	}
}

func TestMatchCreatedLt(t *testing.T) {
	task := makeTask() // Created: 2026-02-01
	if !match(t, "created:<2026-03-01", task, "") {
		t.Error("created:<2026-03-01 should match 2026-02-01")
	}
}
