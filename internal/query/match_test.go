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
		ID:         "01TEST",
		Created:    time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC),
		ModifiedAt: tp("2026-03-03"),
		Text:       "Book flights to Hawaii",
		State:      model.StateNextAction,
		Tags:       []string{"travel", "@computer"},
		Deadline:   tp("2026-03-15"),
		URL:        "https://example.com",
		Notes:      "Check Southwest and United.",
		WaitingOn:  "",
	}
}

func match(t *testing.T, q string, task model.Task, source string) bool {
	t.Helper()
	expr, err := Parse(q, testNow)
	if err != nil {
		t.Fatalf("Parse(%q): %v", q, err)
	}
	return MatchAll(expr, task, source)
}

func TestMatchEmptyQueryMatchesEverything(t *testing.T) {
	if !match(t, "", makeTask(), "single-actions") {
		t.Error("empty query should match everything")
	}
}

func TestMatchBasicFields(t *testing.T) {
	task := makeTask()
	if !match(t, "state:next-action", task, "") {
		t.Error("state:next-action should match")
	}
	if !match(t, "tag:travel", task, "") {
		t.Error("tag:travel should match")
	}
	if !match(t, "@computer", task, "") {
		t.Error("bare @computer should match")
	}
	if !match(t, "flights", task, "") {
		t.Error("bare text should match task text")
	}
}

func TestMatchDateComparators(t *testing.T) {
	task := makeTask()
	if !match(t, "deadline:<2026-04-01", task, "") {
		t.Error("deadline:< should match")
	}
	if !match(t, "deadline:<=2026-03-15", task, "") {
		t.Error("deadline:<= should match equal day")
	}
	if !match(t, "deadline:>=2026-03-15", task, "") {
		t.Error("deadline:>= should match equal day")
	}
	if match(t, "deadline:<=2026-03-01", task, "") {
		t.Error("deadline:<=2026-03-01 should not match 2026-03-15")
	}
}

func TestMatchBooleanAndOrNot(t *testing.T) {
	task := makeTask()
	if !match(t, "state:next-action AND tag:travel", task, "") {
		t.Error("AND should match when both are true")
	}
	if !match(t, "state:waiting-for OR tag:travel", task, "") {
		t.Error("OR should match when one side is true")
	}
	if !match(t, "NOT state:done", task, "") {
		t.Error("NOT should invert clause match")
	}
	if match(t, "NOT tag:travel", task, "") {
		t.Error("NOT tag:travel should not match")
	}
}

func TestMatchParenthesizedExpressions(t *testing.T) {
	task := makeTask()
	if !match(t, "state:next-action AND (tag:@home OR tag:travel)", task, "") {
		t.Error("parenthesized OR should match")
	}
	if match(t, "(state:waiting-for OR tag:@home) AND text:dentist", task, "") {
		t.Error("parenthesized expression should not match")
	}
}

func TestMatchProjectAndWaitingOn(t *testing.T) {
	task := makeTask()
	task.WaitingOn = "Design Team"
	if !match(t, "project:launch", task, "projects/launch-website.md") {
		t.Error("project:launch should match source")
	}
	if !match(t, "waiting_on:design", task, "") {
		t.Error("waiting_on substring should match")
	}
}
