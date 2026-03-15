package query

import (
	"strings"
	"time"

	"github.com/wnwn/wnwn/internal/model"
)

// MatchAll reports whether a task satisfies the full query expression.
// A nil expression matches everything.
func MatchAll(expr Expr, task model.Task, source string) bool {
	if expr == nil {
		return true
	}
	switch e := expr.(type) {
	case ClauseExpr:
		return matchClause(e.Clause, task, source)
	case AndExpr:
		return MatchAll(e.Left, task, source) && MatchAll(e.Right, task, source)
	case OrExpr:
		return MatchAll(e.Left, task, source) || MatchAll(e.Right, task, source)
	case NotExpr:
		return !MatchAll(e.Inner, task, source)
	default:
		return false
	}
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
		return matchDateCmp(c, task, cmpLt)
	case OpGt:
		return matchDateCmp(c, task, cmpGt)
	case OpLte:
		return matchDateCmp(c, task, cmpLte)
	case OpGte:
		return matchDateCmp(c, task, cmpGte)
	}
	return false
}

func matchText(val string, task model.Task) bool {
	val = strings.ToLower(val)
	return strings.Contains(strings.ToLower(task.Text), val) ||
		strings.Contains(strings.ToLower(task.Notes), val)
}

func matchHas(field string, task model.Task) bool {
	switch field {
	case "deadline":
		return task.Deadline != nil
	case "scheduled":
		return task.Scheduled != nil
	case "modified":
		return task.ModifiedAt != nil
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
		return strings.Contains(strings.ToLower(source), val)
	case "deadline":
		if task.Deadline == nil || c.Time.IsZero() {
			return false
		}
		if c.HasRange {
			return inDateRange(*task.Deadline, c.Time, c.EndTime)
		}
		return sameDay(*task.Deadline, c.Time)
	case "scheduled":
		if task.Scheduled == nil || c.Time.IsZero() {
			return false
		}
		if c.HasRange {
			return inDateRange(*task.Scheduled, c.Time, c.EndTime)
		}
		return sameDay(*task.Scheduled, c.Time)
	case "created":
		if c.Time.IsZero() {
			return strings.Contains(task.Created.Format("2006-01-02"), val)
		}
		if c.HasRange {
			return inDateRange(task.Created, c.Time, c.EndTime)
		}
		return sameDay(task.Created, c.Time)
	case "modified":
		if task.ModifiedAt == nil {
			return false
		}
		if c.Time.IsZero() {
			return strings.Contains(task.ModifiedAt.Format("2006-01-02"), val)
		}
		if c.HasRange {
			return inDateRange(*task.ModifiedAt, c.Time, c.EndTime)
		}
		return sameDay(*task.ModifiedAt, c.Time)
	}
	return false
}

type dateCmp int

const (
	cmpLt dateCmp = iota
	cmpGt
	cmpLte
	cmpGte
)

func matchDateCmp(c Clause, task model.Task, cmp dateCmp) bool {
	var taskDate *time.Time
	switch c.Field {
	case "deadline":
		taskDate = task.Deadline
	case "scheduled":
		taskDate = task.Scheduled
	case "created":
		t := task.Created
		taskDate = &t
	case "modified":
		taskDate = task.ModifiedAt
	}
	if taskDate == nil {
		return false
	}
	td := dayOf(*taskDate)
	cd := dayOf(c.Time)
	switch cmp {
	case cmpLt:
		return td.Before(cd)
	case cmpGt:
		return td.After(cd)
	case cmpLte:
		return td.Before(cd) || td.Equal(cd)
	case cmpGte:
		return td.After(cd) || td.Equal(cd)
	default:
		return false
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func inDateRange(t, start, end time.Time) bool {
	td := dayOf(t)
	sd := dayOf(start)
	ed := dayOf(end)
	return (td.Equal(sd) || td.After(sd)) && (td.Equal(ed) || td.Before(ed))
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
