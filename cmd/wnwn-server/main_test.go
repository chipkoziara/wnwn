package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chipkoziara/wnwn/internal/core"
	"github.com/chipkoziara/wnwn/internal/store"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	// Copy testdata fixture if present, otherwise use empty dir.
	s := store.New(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	c := core.New(s, core.CoreConfig{})
	bus := newEventBus()
	mux := buildRoutes(c, bus)
	return httptest.NewServer(mux)
}

func get(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func postJSON(t *testing.T, srv *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func patchJSON(t *testing.T, srv *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", path, err)
	}
	return resp
}

func deleteReq(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

func decodeData(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	defer resp.Body.Close()
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != "" {
		t.Fatalf("server error: %s", envelope.Error)
	}
	if err := json.Unmarshal(envelope.Data, dst); err != nil {
		t.Fatalf("decode data: %v", err)
	}
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Errorf("status = %d, want %d", resp.StatusCode, want)
	}
}

// ---------------------------------------------------------------------------
// Task tests
// ---------------------------------------------------------------------------

func TestCaptureToInbox(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv, "/api/inbox", map[string]any{
		"text": "Buy milk",
		"tags": []string{"errands"},
	})
	assertStatus(t, resp, http.StatusOK)

	var data struct {
		Task wireTask `json:"task"`
	}
	decodeData(t, resp, &data)
	if data.Task.Text != "Buy milk" {
		t.Errorf("text = %q, want %q", data.Task.Text, "Buy milk")
	}
	if len(data.Task.Tags) != 1 || data.Task.Tags[0] != "errands" {
		t.Errorf("tags = %v, want [errands]", data.Task.Tags)
	}
}

func TestCaptureToInbox_MissingText(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv, "/api/inbox", map[string]any{"text": ""})
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestCaptureToInbox_WithDeadline(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv, "/api/inbox", map[string]any{
		"text":     "File taxes",
		"deadline": "2026-04-30",
	})
	assertStatus(t, resp, http.StatusOK)
	var data struct {
		Task wireTask `json:"task"`
	}
	decodeData(t, resp, &data)
	if data.Task.Deadline == nil {
		t.Fatal("deadline is nil")
	}
	if data.Task.Deadline.Format("2006-01-02") != "2026-04-30" {
		t.Errorf("deadline = %s, want 2026-04-30", data.Task.Deadline.Format("2006-01-02"))
	}
}

func TestListTasks_Inbox(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Add two tasks.
	postJSON(t, srv, "/api/inbox", map[string]any{"text": "Task A"})
	postJSON(t, srv, "/api/inbox", map[string]any{"text": "Task B"})

	resp := get(t, srv, "/api/tasks?list=in")
	assertStatus(t, resp, http.StatusOK)

	var tasks []wireTask
	decodeData(t, resp, &tasks)
	if len(tasks) != 2 {
		t.Errorf("got %d tasks, want 2", len(tasks))
	}
}

func TestListTasks_AllActive(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	postJSON(t, srv, "/api/inbox", map[string]any{"text": "Inbox task"})

	resp := get(t, srv, "/api/tasks")
	assertStatus(t, resp, http.StatusOK)

	var tasks []wireViewTask
	decodeData(t, resp, &tasks)
	if len(tasks) == 0 {
		t.Error("expected at least one task")
	}
}

func TestGetTask(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	var created struct {
		Task wireTask `json:"task"`
	}
	resp := postJSON(t, srv, "/api/inbox", map[string]any{"text": "Find me"})
	decodeData(t, resp, &created)

	resp2 := get(t, srv, "/api/tasks/"+created.Task.ID)
	assertStatus(t, resp2, http.StatusOK)

	var data struct {
		Location wireTaskLocation `json:"location"`
	}
	decodeData(t, resp2, &data)
	if data.Location.Task.ID != created.Task.ID {
		t.Errorf("id = %q, want %q", data.Location.Task.ID, created.Task.ID)
	}
}

func TestGetTask_NotFound(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := get(t, srv, "/api/tasks/nonexistent")
	assertStatus(t, resp, http.StatusNotFound)
}

func TestUpdateTask(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	var created struct {
		Task wireTask `json:"task"`
	}
	resp := postJSON(t, srv, "/api/inbox", map[string]any{"text": "Original"})
	decodeData(t, resp, &created)

	newText := "Updated"
	resp2 := patchJSON(t, srv, "/api/tasks/"+created.Task.ID, map[string]any{
		"text":  newText,
		"state": "next-action",
	})
	assertStatus(t, resp2, http.StatusOK)

	var data struct {
		Location wireTaskLocation `json:"location"`
	}
	decodeData(t, resp2, &data)
	if data.Location.Task.Text != newText {
		t.Errorf("text = %q, want %q", data.Location.Task.Text, newText)
	}
}

func TestTrashTask(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	var created struct {
		Task wireTask `json:"task"`
	}
	resp := postJSON(t, srv, "/api/inbox", map[string]any{"text": "Delete me"})
	decodeData(t, resp, &created)

	resp2 := deleteReq(t, srv, "/api/tasks/"+created.Task.ID)
	assertStatus(t, resp2, http.StatusOK)

	// Should no longer be findable.
	resp3 := get(t, srv, "/api/tasks/"+created.Task.ID)
	assertStatus(t, resp3, http.StatusNotFound)
}

func TestArchiveAndRestoreTask(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	var created struct {
		Task wireTask `json:"task"`
	}
	resp := postJSON(t, srv, "/api/inbox", map[string]any{"text": "Archive me"})
	decodeData(t, resp, &created)
	id := created.Task.ID

	// Archive.
	resp2 := postJSON(t, srv, "/api/tasks/"+id+"/archive", nil)
	assertStatus(t, resp2, http.StatusOK)

	// Confirm archived.
	resp3 := get(t, srv, "/api/tasks/"+id)
	assertStatus(t, resp3, http.StatusOK)
	var loc struct {
		Location wireTaskLocation `json:"location"`
	}
	decodeData(t, resp3, &loc)
	if !loc.Location.Archived {
		t.Error("expected archived=true")
	}

	// Restore.
	resp4 := postJSON(t, srv, "/api/tasks/"+id+"/restore", nil)
	assertStatus(t, resp4, http.StatusOK)

	// Confirm active again.
	resp5 := get(t, srv, "/api/tasks/"+id)
	var loc2 struct {
		Location wireTaskLocation `json:"location"`
	}
	decodeData(t, resp5, &loc2)
	if loc2.Location.Archived {
		t.Error("expected archived=false after restore")
	}
}

func TestMoveTask_ToList(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	var created struct {
		Task wireTask `json:"task"`
	}
	resp := postJSON(t, srv, "/api/inbox", map[string]any{"text": "Move me"})
	decodeData(t, resp, &created)

	resp2 := postJSON(t, srv, "/api/tasks/"+created.Task.ID+"/move", map[string]any{
		"to":    "list",
		"list":  "single-actions",
		"state": "next-action",
	})
	assertStatus(t, resp2, http.StatusOK)

	var data struct {
		Location wireTaskLocation `json:"location"`
	}
	decodeData(t, resp2, &data)
	if data.Location.Kind != "actions" {
		t.Errorf("kind = %q, want %q", data.Location.Kind, "actions")
	}
}

// ---------------------------------------------------------------------------
// Project tests
// ---------------------------------------------------------------------------

func TestCreateAndGetProject(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv, "/api/projects", map[string]any{
		"title":          "Launch Website",
		"subgroup_title": "Design",
	})
	assertStatus(t, resp, http.StatusOK)

	var created struct {
		Location wireProjectLocation `json:"location"`
	}
	decodeData(t, resp, &created)
	if created.Location.Project.Title != "Launch Website" {
		t.Errorf("title = %q, want %q", created.Location.Project.Title, "Launch Website")
	}
	if len(created.Location.Project.SubGroups) != 1 {
		t.Errorf("subgroup count = %d, want 1", len(created.Location.Project.SubGroups))
	}

	// Fetch by ID.
	resp2 := get(t, srv, "/api/projects/"+created.Location.ProjectID)
	assertStatus(t, resp2, http.StatusOK)
	var fetched struct {
		Location wireProjectLocation `json:"location"`
	}
	decodeData(t, resp2, &fetched)
	if fetched.Location.ProjectID != created.Location.ProjectID {
		t.Errorf("id mismatch: got %q, want %q", fetched.Location.ProjectID, created.Location.ProjectID)
	}
}

func TestListProjects(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	postJSON(t, srv, "/api/projects", map[string]any{"title": "Project Alpha"})
	postJSON(t, srv, "/api/projects", map[string]any{"title": "Project Beta"})

	resp := get(t, srv, "/api/projects")
	assertStatus(t, resp, http.StatusOK)
	var data struct {
		Projects []wireProjectSummary `json:"projects"`
	}
	decodeData(t, resp, &data)
	if len(data.Projects) != 2 {
		t.Errorf("got %d projects, want 2", len(data.Projects))
	}
}

func TestUpdateProject(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	var created struct {
		Location wireProjectLocation `json:"location"`
	}
	resp := postJSON(t, srv, "/api/projects", map[string]any{"title": "Old Title"})
	decodeData(t, resp, &created)

	newTitle := "New Title"
	resp2 := patchJSON(t, srv, "/api/projects/"+created.Location.ProjectID, map[string]any{
		"title": newTitle,
	})
	assertStatus(t, resp2, http.StatusOK)
	var updated struct {
		Location wireProjectLocation `json:"location"`
	}
	decodeData(t, resp2, &updated)
	if updated.Location.Project.Title != newTitle {
		t.Errorf("title = %q, want %q", updated.Location.Project.Title, newTitle)
	}
}

// ---------------------------------------------------------------------------
// Subgroup and project task tests
// ---------------------------------------------------------------------------

func TestSubgroupCRUD(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	var proj struct {
		Location wireProjectLocation `json:"location"`
	}
	resp := postJSON(t, srv, "/api/projects", map[string]any{"title": "My Project"})
	decodeData(t, resp, &proj)
	pid := proj.Location.ProjectID

	// Create subgroup.
	resp2 := postJSON(t, srv, "/api/projects/"+pid+"/subgroups", map[string]any{"title": "Phase 2"})
	assertStatus(t, resp2, http.StatusOK)
	var sg struct {
		SubgroupID string `json:"subgroup_id"`
		Title      string `json:"title"`
	}
	decodeData(t, resp2, &sg)
	sid := sg.SubgroupID

	// Rename subgroup.
	resp3 := patchJSON(t, srv, "/api/projects/"+pid+"/subgroups/"+sid, map[string]any{"title": "Phase Two"})
	assertStatus(t, resp3, http.StatusOK)

	// Delete subgroup.
	resp4 := deleteReq(t, srv, "/api/projects/"+pid+"/subgroups/"+sid)
	assertStatus(t, resp4, http.StatusOK)
}

func TestAddProjectTask(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	var proj struct {
		Location wireProjectLocation `json:"location"`
	}
	resp := postJSON(t, srv, "/api/projects", map[string]any{"title": "Work Project"})
	decodeData(t, resp, &proj)
	pid := proj.Location.ProjectID
	sid := proj.Location.Project.SubGroups[0].ID

	resp2 := postJSON(t, srv, "/api/projects/"+pid+"/subgroups/"+sid+"/tasks", map[string]any{
		"text": "Write tests",
	})
	assertStatus(t, resp2, http.StatusOK)

	var data struct {
		Location wireTaskLocation `json:"location"`
	}
	decodeData(t, resp2, &data)
	if data.Location.Task.Text != "Write tests" {
		t.Errorf("text = %q, want %q", data.Location.Task.Text, "Write tests")
	}
	if data.Location.ProjectID != pid {
		t.Errorf("project_id = %q, want %q", data.Location.ProjectID, pid)
	}
}

// ---------------------------------------------------------------------------
// Views and query tests
// ---------------------------------------------------------------------------

func TestListViews(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := get(t, srv, "/api/views")
	assertStatus(t, resp, http.StatusOK)
	var data struct {
		Views []wireSavedView `json:"views"`
	}
	decodeData(t, resp, &data)
	if len(data.Views) == 0 {
		t.Error("expected at least one built-in view")
	}
}

func TestRunQuery(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Tags prefixed with @ are stored with the @ (matching the app's convention).
	// The bare @work shorthand in the DSL matches stored tags named "@work".
	// Plain tags (no @) are matched with tag:work syntax.
	postJSON(t, srv, "/api/inbox", map[string]any{
		"text": "Tagged task",
		"tags": []string{"@work"},
	})
	postJSON(t, srv, "/api/inbox", map[string]any{"text": "Untagged task"})

	// @work DSL shorthand matches stored tag "@work".
	resp := postJSON(t, srv, "/api/query", map[string]any{"query": "@work"})
	assertStatus(t, resp, http.StatusOK)
	var data struct {
		Tasks []wireViewTask `json:"tasks"`
	}
	decodeData(t, resp, &data)
	if len(data.Tasks) != 1 {
		t.Errorf("got %d results, want 1", len(data.Tasks))
	}

	// Plain tag DSL (tag:work) matches stored tag "work" (no @).
	postJSON(t, srv, "/api/inbox", map[string]any{
		"text": "Plain tagged task",
		"tags": []string{"errand"},
	})
	resp2 := postJSON(t, srv, "/api/query", map[string]any{"query": "tag:errand"})
	assertStatus(t, resp2, http.StatusOK)
	var data2 struct {
		Tasks []wireViewTask `json:"tasks"`
	}
	decodeData(t, resp2, &data2)
	if len(data2.Tasks) != 1 {
		t.Errorf("got %d plain-tag results, want 1", len(data2.Tasks))
	}
}

func TestRunView_NextActions(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Add a task and move it to single-actions as next-action.
	var created struct {
		Task wireTask `json:"task"`
	}
	resp := postJSON(t, srv, "/api/inbox", map[string]any{"text": "Do something"})
	decodeData(t, resp, &created)
	postJSON(t, srv, "/api/tasks/"+created.Task.ID+"/move", map[string]any{
		"to":    "list",
		"list":  "single-actions",
		"state": "next-action",
	})

	resp2 := get(t, srv, "/api/views/Next Actions/run")
	assertStatus(t, resp2, http.StatusOK)
	var data struct {
		Tasks []wireViewTask `json:"tasks"`
	}
	decodeData(t, resp2, &data)
	if len(data.Tasks) == 0 {
		t.Error("expected at least one next action")
	}
}

// ---------------------------------------------------------------------------
// Weekly review test
// ---------------------------------------------------------------------------

func TestWeeklyReview(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp := get(t, srv, "/api/review/weekly")
	assertStatus(t, resp, http.StatusOK)

	var data struct {
		ProjectsWithoutNextAction []wireProjectSummary `json:"projects_without_next_action"`
		AgingWaitingFor           []wireViewTask       `json:"aging_waiting_for"`
		SomedayMaybe              []wireViewTask       `json:"someday_maybe"`
		RecentArchived            []wireViewTask       `json:"recent_archived"`
	}
	decodeData(t, resp, &data)
	// Just verify the shape comes back without error on an empty store.
}

// ---------------------------------------------------------------------------
// Process Inbox session tests
// ---------------------------------------------------------------------------

func TestInboxSession_StartAndSkip(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Inbox is empty — session should be done immediately.
	resp := postJSON(t, srv, "/api/inbox-sessions", nil)
	assertStatus(t, resp, http.StatusOK)
	var data struct {
		Session wireInboxSession `json:"session"`
	}
	decodeData(t, resp, &data)
	if !data.Session.Done {
		t.Error("expected session to be done with empty inbox")
	}
}

func TestInboxSession_DecideTrash(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Add an inbox item.
	var created struct {
		Task wireTask `json:"task"`
	}
	resp := postJSON(t, srv, "/api/inbox", map[string]any{"text": "Junk"})
	decodeData(t, resp, &created)

	// Start session.
	var sessionData struct {
		Session wireInboxSession `json:"session"`
	}
	resp2 := postJSON(t, srv, "/api/inbox-sessions", nil)
	decodeData(t, resp2, &sessionData)
	sid := sessionData.Session.ID

	if sessionData.Session.Done {
		t.Fatal("expected session not done")
	}

	// Decide: trash.
	resp3 := postJSON(t, srv, "/api/inbox-sessions/"+sid+"/decide", map[string]any{
		"kind": "trash",
	})
	assertStatus(t, resp3, http.StatusOK)
	var after struct {
		Session wireInboxSession `json:"session"`
	}
	decodeData(t, resp3, &after)
	if after.Session.Summary.Trashed != 1 {
		t.Errorf("trashed = %d, want 1", after.Session.Summary.Trashed)
	}
}

func TestInboxSession_UpdateDraft(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	postJSON(t, srv, "/api/inbox", map[string]any{"text": "Draft task"})

	var sessionData struct {
		Session wireInboxSession `json:"session"`
	}
	resp := postJSON(t, srv, "/api/inbox-sessions", nil)
	decodeData(t, resp, &sessionData)
	sid := sessionData.Session.ID

	newText := "Draft task (enriched)"
	resp2 := patchJSON(t, srv, "/api/inbox-sessions/"+sid+"/draft", map[string]any{
		"text": newText,
	})
	assertStatus(t, resp2, http.StatusOK)
	var updated struct {
		Session wireInboxSession `json:"session"`
	}
	decodeData(t, resp2, &updated)
	if updated.Session.Current.Draft.Text != newText {
		t.Errorf("draft text = %q, want %q", updated.Session.Current.Draft.Text, newText)
	}
}

func TestInboxSession_Discard(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	postJSON(t, srv, "/api/inbox", map[string]any{"text": "Keep me"})

	var sessionData struct {
		Session wireInboxSession `json:"session"`
	}
	resp := postJSON(t, srv, "/api/inbox-sessions", nil)
	decodeData(t, resp, &sessionData)
	sid := sessionData.Session.ID

	resp2 := deleteReq(t, srv, "/api/inbox-sessions/"+sid)
	assertStatus(t, resp2, http.StatusOK)

	// Getting the discarded session should 404.
	resp3 := get(t, srv, "/api/inbox-sessions/"+sid)
	assertStatus(t, resp3, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Import / export tests
// ---------------------------------------------------------------------------

func TestExportMarkdown(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	postJSON(t, srv, "/api/inbox", map[string]any{"text": "Export me"})

	outDir := filepath.Join(t.TempDir(), "export")
	resp := postJSON(t, srv, "/api/export-md", map[string]any{
		"output_dir": outDir,
	})
	assertStatus(t, resp, http.StatusOK)

	// Verify the markdown file exists.
	if _, err := os.Stat(filepath.Join(outDir, "in.md")); err != nil {
		t.Errorf("in.md not found after export: %v", err)
	}
}

func TestImportMarkdown_DryRun(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// First export something to import back.
	postJSON(t, srv, "/api/inbox", map[string]any{"text": "Round-trip"})
	outDir := t.TempDir()
	postJSON(t, srv, "/api/export-md", map[string]any{"output_dir": outDir})

	resp := postJSON(t, srv, "/api/import-md", map[string]any{
		"dir":     outDir,
		"mode":    "merge",
		"dry_run": true,
	})
	assertStatus(t, resp, http.StatusOK)
}

// ---------------------------------------------------------------------------
// Date parsing tests
// ---------------------------------------------------------------------------

func TestParseDateTime(t *testing.T) {
	cases := []struct {
		input   string
		wantErr bool
	}{
		{"2026-04-19", false},
		{"2026-04-19T14:30", false},
		{"2026-04-19 14:30", false},
		{"2026-04-19T14:30:00Z", false},
		{"not-a-date", true},
		{"", true},
	}
	for _, c := range cases {
		_, err := parseDateTime(c.input)
		if (err != nil) != c.wantErr {
			t.Errorf("parseDateTime(%q): err=%v, wantErr=%v", c.input, err, c.wantErr)
		}
	}
}

// ---------------------------------------------------------------------------
// Wire type tests
// ---------------------------------------------------------------------------

func TestWireTaskRoundtrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	deadline := now.Add(24 * time.Hour)
	wt := wireTask{
		ID:       "test-id",
		Text:     "Hello",
		State:    "next-action",
		Created:  now,
		Deadline: &deadline,
		Tags:     []string{"a", "b"},
	}
	b, err := json.Marshal(wt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out wireTask
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ID != wt.ID || out.Text != wt.Text {
		t.Errorf("roundtrip mismatch: got %+v", out)
	}
	if out.Deadline == nil || !out.Deadline.Equal(*wt.Deadline) {
		t.Errorf("deadline mismatch: got %v, want %v", out.Deadline, wt.Deadline)
	}
}

func TestCORSHeaders(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/views")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

func TestCORSPreflight(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/api/projects", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", resp.StatusCode)
	}
}
