package query

import (
	"strings"
	"time"

	"github.com/g-tuddy/g-tuddy/internal/model"
)

// MatchAll reports whether a task satisfies all clauses (implicit AND).
// source is the provenance string for the task (e.g. "in", "single-actions",
// "projects/launch-website.md") — used by the "project" field clause.
// An empty clause slice matches everything.
func MatchAll(clauses []Clause, task model.Task, source string) bool {
	for _, c := range clauses {
		if !matchClause(c, task, source) {
			return false
		}
	}
	return true
}

func matchClause(c Clause, task model.Task, source string) bool {
	switch c.Op {
	case OpText:
		return matchText(c.Value, task)
	case OpHas:
		return matchHas(c.Field, task)
	case OpEq:
		return matchEq(c, task, source)
	case OpLt:
		return matchDateCmp(c, task, -1)
	case OpGt:
		return matchDateCmp(c, task, 1)
	}
	return false
}

// matchText does a case-insensitive substring search across Text and Notes.
func matchText(val string, task model.Task) bool {
	val = strings.ToLower(val)
	return strings.Contains(strings.ToLower(task.Text), val) ||
		strings.Contains(strings.ToLower(task.Notes), val)
}

// matchHas checks whether a field is non-empty/non-nil.
func matchHas(field string, task model.Task) bool {
	switch field {
	case "deadline":
		return task.Deadline != nil
	case "scheduled":
		return task.Scheduled != nil
	case "waiting_on":
		return task.WaitingOn != ""
	case "notes":
		return task.Notes != ""
	case "url":
		return task.URL != ""
	case "tags", "tag":
		return len(task.Tags) > 0
	}
	return false
}

// matchEq handles OpEq for all field types.
func matchEq(c Clause, task model.Task, source string) bool {
	val := strings.ToLower(c.Value)
	switch c.Field {
	case "state":
		return strings.ToLower(string(task.State)) == val
	case "tag":
		for _, t := range task.Tags {
			if strings.ToLower(t) == val {
				return true
			}
		}
		return false
	case "waiting_on":
		return strings.Contains(strings.ToLower(task.WaitingOn), val)
	case "text":
		return strings.Contains(strings.ToLower(task.Text), val)
	case "project":
		// Match against the source provenance string (project filename or title).
		return strings.Contains(strings.ToLower(source), val)
	case "deadline":
		// OpEq on a date field means "same calendar day".
		if task.Deadline == nil || c.Time.IsZero() {
			return false
		}
		return sameDay(*task.Deadline, c.Time)
	case "scheduled":
		if task.Scheduled == nil || c.Time.IsZero() {
			return false
		}
		return sameDay(*task.Scheduled, c.Time)
	case "created":
		if c.Time.IsZero() {
			// Non-date value: substring match on the formatted date.
			return strings.Contains(task.Created.Format("2006-01-02"), val)
		}
		return sameDay(task.Created, c.Time)
	}
	return false
}

// matchDateCmp handles OpLt and OpGt for date fields.
// dir is -1 for Lt (task date before clause date) and 1 for Gt (after).
func matchDateCmp(c Clause, task model.Task, dir int) bool {
	var taskDate *time.Time
	switch c.Field {
	case "deadline":
		taskDate = task.Deadline
	case "scheduled":
		taskDate = task.Scheduled
	case "created":
		t := task.Created
		taskDate = &t
	}
	if taskDate == nil {
		return false
	}
	// Normalise to midnight for day-level comparison.
	td := dayOf(*taskDate)
	cd := dayOf(c.Time)
	if dir < 0 {
		return td.Before(cd)
	}
	return td.After(cd)
}

// sameDay reports whether two times fall on the same calendar day.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// dayOf returns midnight on the day of t in t's location.
func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
