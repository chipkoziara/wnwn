package query

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
var today = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

func TestParseEmpty(t *testing.T) {
	clauses, err := Parse("", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(clauses) != 0 {
		t.Errorf("expected 0 clauses, got %d", len(clauses))
	}
}

func TestParseStateEq(t *testing.T) {
	clauses, err := Parse("state:next-action", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(clauses))
	}
	c := clauses[0]
	if c.Field != "state" || c.Op != OpEq || c.Value != "next-action" {
		t.Errorf("unexpected clause: %+v", c)
	}
}

func TestParseTagEq(t *testing.T) {
	clauses, err := Parse("tag:@home", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if clauses[0].Field != "tag" || clauses[0].Value != "@home" {
		t.Errorf("unexpected clause: %+v", clauses[0])
	}
}

func TestParseBareAtTag(t *testing.T) {
	clauses, err := Parse("@home", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(clauses))
	}
	c := clauses[0]
	if c.Field != "tag" || c.Op != OpEq || c.Value != "@home" {
		t.Errorf("bare @tag shorthand not resolved: %+v", c)
	}
}

func TestParseDateLt(t *testing.T) {
	clauses, err := Parse("deadline:<2026-04-01", testNow)
	if err != nil {
		t.Fatal(err)
	}
	c := clauses[0]
	if c.Field != "deadline" || c.Op != OpLt {
		t.Errorf("unexpected clause: %+v", c)
	}
	want := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if !c.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", c.Time, want)
	}
}

func TestParseDateGt(t *testing.T) {
	clauses, err := Parse("scheduled:>2026-03-15", testNow)
	if err != nil {
		t.Fatal(err)
	}
	c := clauses[0]
	if c.Op != OpGt {
		t.Errorf("expected OpGt, got %v", c.Op)
	}
}

func TestParseModifiedDateGt(t *testing.T) {
	clauses, err := Parse("modified:>2026-03-15", testNow)
	if err != nil {
		t.Fatal(err)
	}
	c := clauses[0]
	if c.Field != "modified" || c.Op != OpGt {
		t.Errorf("unexpected clause: %+v", c)
	}
}

func TestParseDateEq(t *testing.T) {
	clauses, err := Parse("deadline:2026-03-10", testNow)
	if err != nil {
		t.Fatal(err)
	}
	c := clauses[0]
	if c.Op != OpEq {
		t.Errorf("expected OpEq, got %v", c.Op)
	}
	want := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	if !c.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", c.Time, want)
	}
}

func TestParseRelativeDateToday(t *testing.T) {
	clauses, err := Parse("deadline:<today", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if !clauses[0].Time.Equal(today) {
		t.Errorf("today resolved to %v, want %v", clauses[0].Time, today)
	}
}

func TestParseRelativeDateTomorrow(t *testing.T) {
	clauses, err := Parse("deadline:<tomorrow", testNow)
	if err != nil {
		t.Fatal(err)
	}
	want := today.AddDate(0, 0, 1)
	if !clauses[0].Time.Equal(want) {
		t.Errorf("tomorrow resolved to %v, want %v", clauses[0].Time, want)
	}
}

func TestParseRelativeDateNd(t *testing.T) {
	clauses, err := Parse("deadline:<7d", testNow)
	if err != nil {
		t.Fatal(err)
	}
	want := today.AddDate(0, 0, 7)
	if !clauses[0].Time.Equal(want) {
		t.Errorf("7d resolved to %v, want %v", clauses[0].Time, want)
	}
}

func TestParseHas(t *testing.T) {
	clauses, err := Parse("has:deadline", testNow)
	if err != nil {
		t.Fatal(err)
	}
	c := clauses[0]
	if c.Op != OpHas || c.Field != "deadline" {
		t.Errorf("unexpected clause: %+v", c)
	}
}

func TestParseHasNotes(t *testing.T) {
	clauses, err := Parse("has:notes", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if clauses[0].Op != OpHas || clauses[0].Field != "notes" {
		t.Errorf("unexpected clause: %+v", clauses[0])
	}
}

func TestParseBareText(t *testing.T) {
	clauses, err := Parse("buy milk", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(clauses) != 2 {
		t.Fatalf("expected 2 clauses (one per word), got %d", len(clauses))
	}
	for _, c := range clauses {
		if c.Op != OpText {
			t.Errorf("expected OpText, got %+v", c)
		}
	}
	if clauses[0].Value != "buy" || clauses[1].Value != "milk" {
		t.Errorf("unexpected values: %v %v", clauses[0].Value, clauses[1].Value)
	}
}

func TestParseMultipleClausesImplicitAnd(t *testing.T) {
	clauses, err := Parse("state:waiting-for tag:@office", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(clauses) != 2 {
		t.Fatalf("expected 2 clauses, got %d", len(clauses))
	}
	if clauses[0].Field != "state" || clauses[1].Field != "tag" {
		t.Errorf("unexpected fields: %v %v", clauses[0].Field, clauses[1].Field)
	}
}

func TestParseUnknownField(t *testing.T) {
	_, err := Parse("bogus:value", testNow)
	if err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

func TestParseUnknownHasField(t *testing.T) {
	_, err := Parse("has:bogus", testNow)
	if err == nil {
		t.Error("expected error for unknown has: field, got nil")
	}
}

func TestParseBadDate(t *testing.T) {
	_, err := Parse("deadline:<notadate", testNow)
	if err == nil {
		t.Error("expected error for bad date, got nil")
	}
}

func TestParseWaitingOn(t *testing.T) {
	clauses, err := Parse("waiting_on:marvin", testNow)
	if err != nil {
		t.Fatal(err)
	}
	c := clauses[0]
	if c.Field != "waiting_on" || c.Op != OpEq || c.Value != "marvin" {
		t.Errorf("unexpected clause: %+v", c)
	}
}

func TestParseProject(t *testing.T) {
	clauses, err := Parse("project:launch-website", testNow)
	if err != nil {
		t.Fatal(err)
	}
	c := clauses[0]
	if c.Field != "project" || c.Op != OpEq {
		t.Errorf("unexpected clause: %+v", c)
	}
}

func TestParseFieldCaseInsensitive(t *testing.T) {
	clauses, err := Parse("STATE:next-action", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if clauses[0].Field != "state" {
		t.Errorf("field not lowercased: %v", clauses[0].Field)
	}
}
