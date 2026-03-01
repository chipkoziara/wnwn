package parser

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/g-tuddy/g-tuddy/internal/model"
	"github.com/g-tuddy/g-tuddy/internal/writer"
)

func mustTime(s string) time.Time {
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	panic("cannot parse time: " + s)
}

func timePtr(t time.Time) *time.Time { return &t }

// --- TaskList parsing tests ---

func TestParseInboxFile(t *testing.T) {
	f, err := os.Open("../../testdata/in.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	list, err := ParseTaskList(f)
	if err != nil {
		t.Fatal(err)
	}

	if list.Title != "Inbox" {
		t.Errorf("title = %q, want %q", list.Title, "Inbox")
	}
	if list.Type != model.ListIn {
		t.Errorf("type = %q, want %q", list.Type, model.ListIn)
	}
	if len(list.Tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(list.Tasks))
	}

	// First task: minimal inbox item.
	task := list.Tasks[0]
	if task.Text != "Look into new project management tools" {
		t.Errorf("task[0].Text = %q", task.Text)
	}
	if task.ID != "01JNQX7K9MZYZ3VDEHGR8N0P6F" {
		t.Errorf("task[0].ID = %q", task.ID)
	}
	if task.State != model.StateEmpty {
		t.Errorf("task[0].State = %q, want empty", task.State)
	}

	// Second task: has tags.
	task = list.Tasks[1]
	if task.Text != "Schedule dentist appointment" {
		t.Errorf("task[1].Text = %q", task.Text)
	}
	if len(task.Tags) != 1 || task.Tags[0] != "@errands" {
		t.Errorf("task[1].Tags = %v", task.Tags)
	}

	// Third task.
	task = list.Tasks[2]
	if len(task.Tags) != 1 || task.Tags[0] != "read/review" {
		t.Errorf("task[2].Tags = %v", task.Tags)
	}
}

func TestParseSingleActionsFile(t *testing.T) {
	f, err := os.Open("../../testdata/single-actions.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	list, err := ParseTaskList(f)
	if err != nil {
		t.Fatal(err)
	}

	if list.Title != "Single Actions" {
		t.Errorf("title = %q", list.Title)
	}
	if list.Type != model.ListSingleActions {
		t.Errorf("type = %q", list.Type)
	}
	if len(list.Tasks) != 4 {
		t.Fatalf("got %d tasks, want 4", len(list.Tasks))
	}

	// Task with state and deadline.
	task := list.Tasks[1]
	if task.State != model.StateNextAction {
		t.Errorf("task[1].State = %q", task.State)
	}
	if task.Deadline == nil {
		t.Fatal("task[1].Deadline is nil")
	}
	wantDeadline := mustTime("2026-03-15T17:00")
	if !task.Deadline.Equal(wantDeadline) {
		t.Errorf("task[1].Deadline = %v, want %v", task.Deadline, wantDeadline)
	}

	// Task with notes.
	if task.Notes != "Check Southwest and United for direct flights from SFO." {
		t.Errorf("task[1].Notes = %q", task.Notes)
	}

	// Done task.
	task = list.Tasks[2]
	if task.State != model.StateDone {
		t.Errorf("task[2].State = %q, want done", task.State)
	}

	// Canceled task.
	task = list.Tasks[3]
	if task.State != model.StateCanceled {
		t.Errorf("task[3].State = %q, want canceled", task.State)
	}
}

// --- Project parsing tests ---

func TestParseProjectFile(t *testing.T) {
	f, err := os.Open("../../testdata/projects/launch-website.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	proj, err := ParseProject(f)
	if err != nil {
		t.Fatal(err)
	}

	if proj.Title != "Launch Website" {
		t.Errorf("title = %q", proj.Title)
	}
	if proj.ID != "01JNQXKP7VWXY9ZAB0CDEF3GHI" {
		t.Errorf("id = %q", proj.ID)
	}
	if proj.State != model.StateNextAction {
		t.Errorf("state = %q", proj.State)
	}
	if proj.Deadline == nil {
		t.Fatal("deadline is nil")
	}

	wantTags := []string{"project", "launch", "@office"}
	if len(proj.Tags) != len(wantTags) {
		t.Fatalf("tags = %v, want %v", proj.Tags, wantTags)
	}
	for i, tag := range wantTags {
		if proj.Tags[i] != tag {
			t.Errorf("tags[%d] = %q, want %q", i, proj.Tags[i], tag)
		}
	}

	// Sub-groups.
	if len(proj.SubGroups) != 3 {
		t.Fatalf("got %d sub-groups, want 3", len(proj.SubGroups))
	}

	// Design sub-group.
	sg := proj.SubGroups[0]
	if sg.Title != "Design" {
		t.Errorf("sg[0].Title = %q", sg.Title)
	}
	if sg.ID != "01JNQXLQ8WXYZ0ABC1DEFG4HIJ" {
		t.Errorf("sg[0].ID = %q", sg.ID)
	}
	if sg.State != model.StateNextAction {
		t.Errorf("sg[0].State = %q", sg.State)
	}
	if len(sg.Tasks) != 2 {
		t.Fatalf("sg[0] has %d tasks, want 2", len(sg.Tasks))
	}

	// Waiting-for task with delegation.
	task := sg.Tasks[1]
	if task.State != model.StateWaitingFor {
		t.Errorf("task.State = %q", task.State)
	}
	if task.DelegatedTo != "Design Team" {
		t.Errorf("task.DelegatedTo = %q", task.DelegatedTo)
	}
	if task.WaitingSince == nil {
		t.Fatal("task.WaitingSince is nil")
	}

	// Development sub-group.
	sg = proj.SubGroups[1]
	if sg.State != model.StateSomeday {
		t.Errorf("sg[1].State = %q", sg.State)
	}
	if len(sg.Tasks) != 2 {
		t.Fatalf("sg[1] has %d tasks, want 2", len(sg.Tasks))
	}

	// Task with notes.
	task = sg.Tasks[1]
	if task.Text != "Implement responsive homepage" {
		t.Errorf("task.Text = %q", task.Text)
	}
	wantNotes := "Must support mobile, tablet, and desktop breakpoints.\nReference the brand guidelines doc for color palette."
	if task.Notes != wantNotes {
		t.Errorf("task.Notes = %q, want %q", task.Notes, wantNotes)
	}

	// Launch sub-group — has deadline but no state.
	sg = proj.SubGroups[2]
	if sg.Title != "Launch" {
		t.Errorf("sg[2].Title = %q", sg.Title)
	}
	if sg.Deadline == nil {
		t.Fatal("sg[2].Deadline is nil")
	}
	if sg.State != model.StateEmpty {
		t.Errorf("sg[2].State = %q, want empty", sg.State)
	}
}

// --- Round-trip tests ---

func TestTaskListRoundTrip(t *testing.T) {
	f, err := os.Open("../../testdata/in.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	list, err := ParseTaskList(f)
	if err != nil {
		t.Fatal(err)
	}

	// Write back to string.
	var buf strings.Builder
	if err := writer.WriteTaskList(&buf, list); err != nil {
		t.Fatal(err)
	}

	// Parse the written output again.
	list2, err := ParseTaskList(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse failed: %v\n\nOutput:\n%s", err, buf.String())
	}

	// Verify structural equivalence.
	if list.Title != list2.Title {
		t.Errorf("title: %q != %q", list.Title, list2.Title)
	}
	if list.Type != list2.Type {
		t.Errorf("type: %q != %q", list.Type, list2.Type)
	}
	if len(list.Tasks) != len(list2.Tasks) {
		t.Fatalf("task count: %d != %d", len(list.Tasks), len(list2.Tasks))
	}
	for i, task := range list.Tasks {
		task2 := list2.Tasks[i]
		if task.ID != task2.ID {
			t.Errorf("task[%d].ID: %q != %q", i, task.ID, task2.ID)
		}
		if task.Text != task2.Text {
			t.Errorf("task[%d].Text: %q != %q", i, task.Text, task2.Text)
		}
		if task.State != task2.State {
			t.Errorf("task[%d].State: %q != %q", i, task.State, task2.State)
		}
		if len(task.Tags) != len(task2.Tags) {
			t.Errorf("task[%d].Tags: %v != %v", i, task.Tags, task2.Tags)
		}
	}
}

func TestProjectRoundTrip(t *testing.T) {
	f, err := os.Open("../../testdata/projects/launch-website.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	proj, err := ParseProject(f)
	if err != nil {
		t.Fatal(err)
	}

	// Write back to string.
	var buf strings.Builder
	if err := writer.WriteProject(&buf, proj); err != nil {
		t.Fatal(err)
	}

	// Parse the written output again.
	proj2, err := ParseProject(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse failed: %v\n\nOutput:\n%s", err, buf.String())
	}

	// Verify structural equivalence.
	if proj.Title != proj2.Title {
		t.Errorf("title: %q != %q", proj.Title, proj2.Title)
	}
	if proj.ID != proj2.ID {
		t.Errorf("id: %q != %q", proj.ID, proj2.ID)
	}
	if proj.State != proj2.State {
		t.Errorf("state: %q != %q", proj.State, proj2.State)
	}
	if len(proj.SubGroups) != len(proj2.SubGroups) {
		t.Fatalf("sub-group count: %d != %d", len(proj.SubGroups), len(proj2.SubGroups))
	}
	for i, sg := range proj.SubGroups {
		sg2 := proj2.SubGroups[i]
		if sg.Title != sg2.Title {
			t.Errorf("sg[%d].Title: %q != %q", i, sg.Title, sg2.Title)
		}
		if sg.ID != sg2.ID {
			t.Errorf("sg[%d].ID: %q != %q", i, sg.ID, sg2.ID)
		}
		if sg.State != sg2.State {
			t.Errorf("sg[%d].State: %q != %q", i, sg.State, sg2.State)
		}
		if len(sg.Tasks) != len(sg2.Tasks) {
			t.Errorf("sg[%d] task count: %d != %d", i, len(sg.Tasks), len(sg2.Tasks))
			continue
		}
		for j, task := range sg.Tasks {
			task2 := sg2.Tasks[j]
			if task.ID != task2.ID {
				t.Errorf("sg[%d].task[%d].ID: %q != %q", i, j, task.ID, task2.ID)
			}
			if task.Text != task2.Text {
				t.Errorf("sg[%d].task[%d].Text: %q != %q", i, j, task.Text, task2.Text)
			}
			if task.State != task2.State {
				t.Errorf("sg[%d].task[%d].State: %q != %q", i, j, task.State, task2.State)
			}
			if task.DelegatedTo != task2.DelegatedTo {
				t.Errorf("sg[%d].task[%d].DelegatedTo: %q != %q", i, j, task.DelegatedTo, task2.DelegatedTo)
			}
			if task.Notes != task2.Notes {
				t.Errorf("sg[%d].task[%d].Notes: %q != %q", i, j, task.Notes, task2.Notes)
			}
		}
	}
}

// --- Edge case tests ---

func TestParseEmptyFile(t *testing.T) {
	list, err := ParseTaskList(strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tasks) != 0 {
		t.Errorf("got %d tasks, want 0", len(list.Tasks))
	}
}

func TestParseFrontmatterOnly(t *testing.T) {
	input := "---\ntitle: Test\ntype: in\n---\n"
	list, err := ParseTaskList(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if list.Title != "Test" {
		t.Errorf("title = %q", list.Title)
	}
	if len(list.Tasks) != 0 {
		t.Errorf("got %d tasks, want 0", len(list.Tasks))
	}
}

func TestParseTaskWithAllFields(t *testing.T) {
	input := `---
title: Test
type: single-actions
---

- [ ] Full task example
  ` + "```yaml" + `
  id: 01TEST000000000000000000001
  created: 2026-03-01T10:00
  state: waiting-for
  scheduled: 2026-03-05T09:00
  deadline: 2026-03-10T17:00
  url: https://example.com
  tags: ["@office", deep-work]
  delegated_to: Alice
  waiting_since: 2026-03-01
  source: projects/test-project
  ` + "```" + `

  These are some detailed notes.
  They span multiple lines.
`

	list, err := ParseTaskList(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(list.Tasks))
	}

	task := list.Tasks[0]
	if task.ID != "01TEST000000000000000000001" {
		t.Errorf("ID = %q", task.ID)
	}
	if task.State != model.StateWaitingFor {
		t.Errorf("State = %q", task.State)
	}
	if task.Scheduled == nil {
		t.Error("Scheduled is nil")
	}
	if task.Deadline == nil {
		t.Error("Deadline is nil")
	}
	if task.URL != "https://example.com" {
		t.Errorf("URL = %q", task.URL)
	}
	if len(task.Tags) != 2 {
		t.Errorf("Tags = %v", task.Tags)
	}
	if task.DelegatedTo != "Alice" {
		t.Errorf("DelegatedTo = %q", task.DelegatedTo)
	}
	if task.WaitingSince == nil {
		t.Error("WaitingSince is nil")
	}
	if task.Source != "projects/test-project" {
		t.Errorf("Source = %q", task.Source)
	}
	wantNotes := "These are some detailed notes.\nThey span multiple lines."
	if task.Notes != wantNotes {
		t.Errorf("Notes = %q, want %q", task.Notes, wantNotes)
	}
}
