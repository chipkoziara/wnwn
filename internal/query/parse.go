// Package query provides a simple DSL for filtering tasks.
//
// Syntax: space-separated clauses, all implicitly ANDed.
//
//	state:next-action
//	tag:@home
//	deadline:<2026-04-01
//	deadline:<7d
//	deadline:<today
//	has:deadline
//	@home              (bare @-tag shorthand, equivalent to tag:@home)
//	buy milk           (bare text, matches task text and notes)
package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Op describes how a clause matches.
type Op int

const (
	OpEq   Op = iota // field:value — exact or substring match (field-dependent)
	OpLt             // field:<value — less than (dates only)
	OpGt             // field:>value — greater than (dates only)
	OpHas            // has:field — field is non-empty/non-nil
	OpText           // no field prefix — free text match on Text + Notes
)

// Clause is a single filter term.
type Clause struct {
	Field string // "state", "tag", "deadline", "scheduled", "waiting_on",
	//            "created", "modified", "text", "project", or "" for OpText
	Op    Op
	Value string    // raw string value
	Time  time.Time // resolved date for OpLt/OpGt/OpEq on date fields; zero if unused
}

// validFields is the set of recognised field names in field:value clauses.
var validFields = map[string]bool{
	"state":      true,
	"tag":        true,
	"deadline":   true,
	"scheduled":  true,
	"waiting_on": true,
	"created":    true,
	"modified":   true,
	"text":       true,
	"project":    true,
}

// dateFields are those that support <, >, and date parsing.
var dateFields = map[string]bool{
	"deadline":  true,
	"scheduled": true,
	"created":   true,
	"modified":  true,
}

// Parse parses a query string into a slice of Clauses.
// The now parameter is used to resolve relative date tokens (today, tomorrow, Nd).
// Returns an error if an unknown field or unparseable date is encountered.
// An empty input returns nil, nil (matches everything).
func Parse(input string, now time.Time) ([]Clause, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	tokens := tokenise(input)
	clauses := make([]Clause, 0, len(tokens))

	for _, tok := range tokens {
		c, err := parseToken(tok, now)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, c)
	}
	return clauses, nil
}

// tokenise splits the input into tokens, respecting quoted strings.
func tokenise(input string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range input {
		switch {
		case inQuote:
			if r == quoteChar {
				inQuote = false
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			inQuote = true
			quoteChar = r
		case unicode.IsSpace(r):
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// parseToken parses a single token into a Clause.
func parseToken(tok string, now time.Time) (Clause, error) {
	// has:field
	if strings.HasPrefix(tok, "has:") {
		field := strings.ToLower(tok[4:])
		if !validFields[field] && field != "notes" && field != "url" {
			return Clause{}, fmt.Errorf("unknown field %q in has: clause", field)
		}
		return Clause{Field: field, Op: OpHas}, nil
	}

	// field:value / field:<value / field:>value
	if idx := strings.IndexByte(tok, ':'); idx > 0 {
		rawField := strings.ToLower(tok[:idx])
		rest := tok[idx+1:]

		if !validFields[rawField] {
			return Clause{}, fmt.Errorf("unknown field %q", rawField)
		}

		// Date comparison operators.
		if dateFields[rawField] {
			if strings.HasPrefix(rest, "<") {
				t, err := resolveDate(rest[1:], now)
				if err != nil {
					return Clause{}, fmt.Errorf("bad date in %s:<%s: %w", rawField, rest[1:], err)
				}
				return Clause{Field: rawField, Op: OpLt, Value: rest[1:], Time: t}, nil
			}
			if strings.HasPrefix(rest, ">") {
				t, err := resolveDate(rest[1:], now)
				if err != nil {
					return Clause{}, fmt.Errorf("bad date in %s:>%s: %w", rawField, rest[1:], err)
				}
				return Clause{Field: rawField, Op: OpGt, Value: rest[1:], Time: t}, nil
			}
			// Exact date: treat as a day range (same calendar day).
			t, err := resolveDate(rest, now)
			if err != nil {
				// Not a date — fall through to string eq (e.g. created:2026).
				return Clause{Field: rawField, Op: OpEq, Value: rest}, nil
			}
			return Clause{Field: rawField, Op: OpEq, Value: rest, Time: t}, nil
		}

		return Clause{Field: rawField, Op: OpEq, Value: rest}, nil
	}

	// Bare @tag shorthand: @home → tag:@home
	if strings.HasPrefix(tok, "@") {
		return Clause{Field: "tag", Op: OpEq, Value: tok}, nil
	}

	// Bare text: free-text match.
	return Clause{Field: "", Op: OpText, Value: tok}, nil
}

// resolveDate parses an absolute date (YYYY-MM-DD) or a relative token
// (today, tomorrow, Nd where N is a positive integer number of days).
// Returns midnight local time on that day.
func resolveDate(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	switch s {
	case "today":
		return today, nil
	case "tomorrow":
		return today.AddDate(0, 0, 1), nil
	}

	// Nd — relative days
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err == nil && n >= 0 {
			return today.AddDate(0, 0, n), nil
		}
	}

	// Absolute date
	t, err := time.ParseInLocation("2006-01-02", s, now.Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot parse date %q (use YYYY-MM-DD, today, tomorrow, or Nd)", s)
	}
	return t, nil
}
