// Package query provides a DSL for filtering tasks.
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
	OpEq Op = iota // field:value — exact or substring match (field-dependent)
	OpLt           // field:<value — less than (dates only)
	OpGt           // field:>value — greater than (dates only)
	OpLte          // field:<=value — less than or equal (dates only)
	OpGte          // field:>=value — greater than or equal (dates only)
	OpHas          // has:field — field is non-empty/non-nil
	OpText         // no field prefix — free text match on Text + Notes
)

// Clause is a single filter term.
type Clause struct {
	Field    string
	Op       Op
	Value    string
	Time     time.Time
	EndTime  time.Time
	HasRange bool
}

// Expr is a boolean query expression tree.
type Expr interface{}

type ClauseExpr struct{ Clause Clause }
type AndExpr struct{ Left, Right Expr }
type OrExpr struct{ Left, Right Expr }
type NotExpr struct{ Inner Expr }

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

var dateFields = map[string]bool{
	"deadline":  true,
	"scheduled": true,
	"created":   true,
	"modified":  true,
}

// Parse parses the query as a boolean expression tree.
func Parse(input string, now time.Time) (Expr, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}
	tokens := tokenise(input)
	p := parser{tokens: tokens, now: now}
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.hasNext() {
		return nil, fmt.Errorf("unexpected token %q", p.peek())
	}
	return expr, nil
}

// ParseClauses preserves the legacy flat implicit-AND output for callers/tests that still want clauses.
func ParseClauses(input string, now time.Time) ([]Clause, error) {
	expr, err := Parse(input, now)
	if err != nil || expr == nil {
		return nil, err
	}
	return flattenImplicitAnd(expr)
}

type parser struct {
	tokens []string
	pos    int
	now    time.Time
}

func (p *parser) parseExpr() (Expr, error) { return p.parseOr() }

func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.hasNext() && strings.EqualFold(p.peek(), "OR") {
		p.pos++
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = OrExpr{Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.hasNext() {
		if strings.EqualFold(p.peek(), "OR") || p.peek() == ")" {
			break
		}
		if strings.EqualFold(p.peek(), "AND") {
			p.pos++
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = AndExpr{Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (Expr, error) {
	if !p.hasNext() {
		return nil, fmt.Errorf("unexpected end of query")
	}
	if strings.EqualFold(p.peek(), "NOT") {
		p.pos++
		inner, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return NotExpr{Inner: inner}, nil
	}
	if p.peek() == "(" {
		p.pos++
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.hasNext() || p.peek() != ")" {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return expr, nil
	}
	c, err := parseToken(p.next(), p.now)
	if err != nil {
		return nil, err
	}
	return ClauseExpr{Clause: c}, nil
}

func (p *parser) hasNext() bool { return p.pos < len(p.tokens) }
func (p *parser) peek() string  { return p.tokens[p.pos] }
func (p *parser) next() string  { tok := p.tokens[p.pos]; p.pos++; return tok }

func tokenise(input string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	quoteChar := rune(0)
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
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
		case r == '(' || r == ')':
			flush()
			tokens = append(tokens, string(r))
		case unicode.IsSpace(r):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func parseToken(tok string, now time.Time) (Clause, error) {
	if strings.HasPrefix(tok, "has:") {
		field := strings.ToLower(tok[4:])
		if !validFields[field] && field != "notes" && field != "url" && field != "tags" {
			return Clause{}, fmt.Errorf("unknown field %q in has: clause", field)
		}
		return Clause{Field: field, Op: OpHas}, nil
	}
	if idx := strings.IndexByte(tok, ':'); idx > 0 {
		rawField := strings.ToLower(tok[:idx])
		rest := tok[idx+1:]
		if !validFields[rawField] {
			return Clause{}, fmt.Errorf("unknown field %q", rawField)
		}
		if dateFields[rawField] {
			if strings.Contains(rest, "..") {
				parts := strings.SplitN(rest, "..", 2)
				if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
					return Clause{}, fmt.Errorf("bad date range in %s:%s", rawField, rest)
				}
				start, err := resolveDate(parts[0], now)
				if err != nil {
					return Clause{}, fmt.Errorf("bad date range start in %s:%s: %w", rawField, rest, err)
				}
				end, err := resolveDate(parts[1], now)
				if err != nil {
					return Clause{}, fmt.Errorf("bad date range end in %s:%s: %w", rawField, rest, err)
				}
				if end.Before(start) {
					return Clause{}, fmt.Errorf("bad date range in %s:%s: end before start", rawField, rest)
				}
				return Clause{Field: rawField, Op: OpEq, Value: rest, Time: start, EndTime: end, HasRange: true}, nil
			}
			switch {
			case strings.HasPrefix(rest, "<="):
				t, err := resolveDate(rest[2:], now)
				if err != nil {
					return Clause{}, fmt.Errorf("bad date in %s:<=%s: %w", rawField, rest[2:], err)
				}
				return Clause{Field: rawField, Op: OpLte, Value: rest[2:], Time: t}, nil
			case strings.HasPrefix(rest, ">="):
				t, err := resolveDate(rest[2:], now)
				if err != nil {
					return Clause{}, fmt.Errorf("bad date in %s:>=%s: %w", rawField, rest[2:], err)
				}
				return Clause{Field: rawField, Op: OpGte, Value: rest[2:], Time: t}, nil
			case strings.HasPrefix(rest, "<"):
				t, err := resolveDate(rest[1:], now)
				if err != nil {
					return Clause{}, fmt.Errorf("bad date in %s:<%s: %w", rawField, rest[1:], err)
				}
				return Clause{Field: rawField, Op: OpLt, Value: rest[1:], Time: t}, nil
			case strings.HasPrefix(rest, ">"):
				t, err := resolveDate(rest[1:], now)
				if err != nil {
					return Clause{}, fmt.Errorf("bad date in %s:>%s: %w", rawField, rest[1:], err)
				}
				return Clause{Field: rawField, Op: OpGt, Value: rest[1:], Time: t}, nil
			}
			t, err := resolveDate(rest, now)
			if err != nil {
				return Clause{Field: rawField, Op: OpEq, Value: rest}, nil
			}
			return Clause{Field: rawField, Op: OpEq, Value: rest, Time: t}, nil
		}
		return Clause{Field: rawField, Op: OpEq, Value: rest}, nil
	}
	if strings.HasPrefix(tok, "@") {
		return Clause{Field: "tag", Op: OpEq, Value: tok}, nil
	}
	return Clause{Field: "", Op: OpText, Value: tok}, nil
}

func flattenImplicitAnd(expr Expr) ([]Clause, error) {
	switch e := expr.(type) {
	case ClauseExpr:
		return []Clause{e.Clause}, nil
	case AndExpr:
		left, err := flattenImplicitAnd(e.Left)
		if err != nil {
			return nil, err
		}
		right, err := flattenImplicitAnd(e.Right)
		if err != nil {
			return nil, err
		}
		return append(left, right...), nil
	default:
		return nil, fmt.Errorf("expression cannot be flattened into implicit AND clauses")
	}
}

func resolveDate(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch s {
	case "today":
		return today, nil
	case "tomorrow":
		return today.AddDate(0, 0, 1), nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err == nil && n >= 0 {
			return today.AddDate(0, 0, n), nil
		}
	}
	t, err := time.ParseInLocation("2006-01-02", s, now.Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot parse date %q (use YYYY-MM-DD, today, tomorrow, or Nd)", s)
	}
	return t, nil
}
