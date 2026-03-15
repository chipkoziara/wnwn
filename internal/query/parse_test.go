package query

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
var today = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

func mustClauseExpr(t *testing.T, input string) ClauseExpr {
	t.Helper()
	expr, err := Parse(input, testNow)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := expr.(ClauseExpr)
	if !ok {
		t.Fatalf("expected ClauseExpr, got %T", expr)
	}
	return c
}

func TestParseEmpty(t *testing.T) {
	expr, err := Parse("", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if expr != nil {
		t.Fatalf("expected nil expr, got %#v", expr)
	}
}

func TestParseStateEq(t *testing.T) {
	c := mustClauseExpr(t, "state:next-action").Clause
	if c.Field != "state" || c.Op != OpEq || c.Value != "next-action" {
		t.Errorf("unexpected clause: %+v", c)
	}
}

func TestParseTagEq(t *testing.T) {
	c := mustClauseExpr(t, "tag:@home").Clause
	if c.Field != "tag" || c.Value != "@home" {
		t.Errorf("unexpected clause: %+v", c)
	}
}

func TestParseBareAtTag(t *testing.T) {
	c := mustClauseExpr(t, "@home").Clause
	if c.Field != "tag" || c.Op != OpEq || c.Value != "@home" {
		t.Errorf("bare @tag shorthand not resolved: %+v", c)
	}
}

func TestParseDateComparators(t *testing.T) {
	if c := mustClauseExpr(t, "deadline:<2026-04-01").Clause; c.Op != OpLt {
		t.Fatalf("expected OpLt, got %v", c.Op)
	}
	if c := mustClauseExpr(t, "deadline:<=2026-04-01").Clause; c.Op != OpLte {
		t.Fatalf("expected OpLte, got %v", c.Op)
	}
	if c := mustClauseExpr(t, "deadline:>2026-04-01").Clause; c.Op != OpGt {
		t.Fatalf("expected OpGt, got %v", c.Op)
	}
	if c := mustClauseExpr(t, "deadline:>=2026-04-01").Clause; c.Op != OpGte {
		t.Fatalf("expected OpGte, got %v", c.Op)
	}
}

func TestParseDateEq(t *testing.T) {
	c := mustClauseExpr(t, "deadline:2026-03-10").Clause
	if c.Op != OpEq {
		t.Errorf("expected OpEq, got %v", c.Op)
	}
	want := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	if !c.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", c.Time, want)
	}
}

func TestParseDateRange(t *testing.T) {
	c := mustClauseExpr(t, "deadline:today..7d").Clause
	if !c.HasRange {
		t.Fatalf("expected HasRange true")
	}
	if !c.Time.Equal(today) {
		t.Fatalf("start = %v, want %v", c.Time, today)
	}
	wantEnd := today.AddDate(0, 0, 7)
	if !c.EndTime.Equal(wantEnd) {
		t.Fatalf("end = %v, want %v", c.EndTime, wantEnd)
	}
}

func TestParseRelativeDates(t *testing.T) {
	if c := mustClauseExpr(t, "deadline:<today").Clause; !c.Time.Equal(today) {
		t.Errorf("today resolved to %v, want %v", c.Time, today)
	}
	wantTomorrow := today.AddDate(0, 0, 1)
	if c := mustClauseExpr(t, "deadline:<tomorrow").Clause; !c.Time.Equal(wantTomorrow) {
		t.Errorf("tomorrow resolved to %v, want %v", c.Time, wantTomorrow)
	}
	want7d := today.AddDate(0, 0, 7)
	if c := mustClauseExpr(t, "deadline:<7d").Clause; !c.Time.Equal(want7d) {
		t.Errorf("7d resolved to %v, want %v", c.Time, want7d)
	}
}

func TestParseHas(t *testing.T) {
	c := mustClauseExpr(t, "has:deadline").Clause
	if c.Op != OpHas || c.Field != "deadline" {
		t.Errorf("unexpected clause: %+v", c)
	}
}

func TestParseBareText(t *testing.T) {
	expr, err := Parse("buy milk", testNow)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := expr.(AndExpr)
	if !ok {
		t.Fatalf("expected implicit AND expression, got %T", expr)
	}
}

func TestParseBooleanOperators(t *testing.T) {
	expr, err := Parse("state:next-action OR tag:@home", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := expr.(OrExpr); !ok {
		t.Fatalf("expected OrExpr, got %T", expr)
	}

	expr, err = Parse("NOT state:done", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := expr.(NotExpr); !ok {
		t.Fatalf("expected NotExpr, got %T", expr)
	}
}

func TestParseParentheses(t *testing.T) {
	expr, err := Parse("state:next-action AND (tag:@home OR tag:@office)", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := expr.(AndExpr); !ok {
		t.Fatalf("expected AndExpr, got %T", expr)
	}
}

func TestParseUnknownField(t *testing.T) {
	_, err := Parse("bogus:value", testNow)
	if err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

func TestParseBadDate(t *testing.T) {
	_, err := Parse("deadline:<notadate", testNow)
	if err == nil {
		t.Error("expected error for bad date, got nil")
	}
}

func TestParseBadDateRange(t *testing.T) {
	_, err := Parse("deadline:7d..today", testNow)
	if err == nil {
		t.Error("expected error for reverse date range, got nil")
	}
}
