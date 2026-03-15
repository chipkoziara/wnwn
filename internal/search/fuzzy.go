package search

import (
	"sort"
	"strings"

	"github.com/wnwn/wnwn/internal/service"
)

type scoredTask struct {
	task  service.ViewTask
	score int
}

// Rank returns tasks matching the needle ordered by descending relevance.
// Matching is intentionally simple for v1: exact/substring matches are scored
// across task text, notes, tags, URL, waiting_on, and source provenance.
func Rank(tasks []service.ViewTask, needle string) []service.ViewTask {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return nil
	}

	scored := make([]scoredTask, 0, len(tasks))
	for _, vt := range tasks {
		score := scoreTask(vt, needle)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredTask{task: vt, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return strings.ToLower(scored[i].task.Task.Text) < strings.ToLower(scored[j].task.Task.Text)
		}
		return scored[i].score > scored[j].score
	})

	results := make([]service.ViewTask, 0, len(scored))
	for _, st := range scored {
		results = append(results, st.task)
	}
	return results
}

func scoreTask(vt service.ViewTask, needle string) int {
	score := 0
	score += scoreField(vt.Task.Text, needle, 100)
	score += scoreField(vt.Task.Notes, needle, 50)
	score += scoreField(vt.Task.WaitingOn, needle, 35)
	score += scoreField(vt.Task.URL, needle, 20)
	score += scoreField(vt.Source, needle, 20)
	for _, tag := range vt.Task.Tags {
		score += scoreField(tag, needle, 30)
	}
	return score
}

func scoreField(field, needle string, base int) int {
	if field == "" {
		return 0
	}
	field = strings.ToLower(field)
	if field == needle {
		return base + 40
	}
	if strings.HasPrefix(field, needle) {
		return base + 25
	}
	if idx := strings.Index(field, needle); idx >= 0 {
		return base + max(1, 20-idx)
	}
	if subsequence(field, needle) {
		return base / 3
	}
	return 0
}

func subsequence(field, needle string) bool {
	if needle == "" {
		return true
	}
	j := 0
	for i := 0; i < len(field) && j < len(needle); i++ {
		if field[i] == needle[j] {
			j++
		}
	}
	return j == len(needle)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
