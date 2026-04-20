// wnwn-server: HTTP/JSON server wrapping internal/core for the Electron GUI client.
//
// Usage:
//
//	wnwn-server [--addr 127.0.0.1:9274]
//
// On startup, prints {"ready":true,"addr":"<addr>"} to stdout so the Electron
// main process knows the server is accepting connections before loading the
// renderer.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chipkoziara/wnwn/internal/config"
	"github.com/chipkoziara/wnwn/internal/core"
	"github.com/chipkoziara/wnwn/internal/model"
	"github.com/chipkoziara/wnwn/internal/service"
	"github.com/chipkoziara/wnwn/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9274", "listen address")
	flag.Parse()

	c := initCore()
	bus := newEventBus()

	mux := buildRoutes(c, bus)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("wnwn-server: listen %s: %v", *addr, err)
	}

	// Startup handshake — Electron reads this from stdout before loading the renderer.
	handshake, _ := json.Marshal(map[string]any{"ready": true, "addr": ln.Addr().String()})
	fmt.Println(string(handshake))

	log.Fatal(http.Serve(ln, mux))
}

// ---------------------------------------------------------------------------
// Initialisation
// ---------------------------------------------------------------------------

func initCore() *core.Core {
	dataDir := getDataDir()
	s := store.New(dataDir)
	if err := s.Init(); err != nil {
		log.Fatalf("wnwn-server: initializing store: %v", err)
	}
	cfg, err := config.Load(dataDir)
	if err != nil {
		log.Fatalf("wnwn-server: loading config: %v", err)
	}
	var savedViews []model.SavedView
	for _, v := range cfg.Views.Saved {
		savedViews = append(savedViews, model.SavedView{
			Name:            v.Name,
			Query:           v.Query,
			IncludeArchived: v.IncludeArchived,
		})
	}
	coreCfg := core.CoreConfig{
		AutoArchiveDone:     cfg.Archive.AutoArchiveDone,
		AutoArchiveCanceled: cfg.Archive.AutoArchiveCanceled,
		SavedViews:          savedViews,
	}
	return core.New(s, coreCfg)
}

func getDataDir() string {
	if dir := os.Getenv("WNWN_DATA_DIR"); dir != "" {
		return dir
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "wnwn")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("wnwn-server: cannot determine home directory: %v", err)
	}
	return filepath.Join(home, ".local", "share", "wnwn")
}

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

func buildRoutes(c *core.Core, bus *eventBus) http.Handler {
	mux := http.NewServeMux()

	h := &handlers{core: c, bus: bus}

	// Tasks
	mux.HandleFunc("GET /api/tasks", h.listTasks)
	mux.HandleFunc("POST /api/inbox", h.captureToInbox)
	mux.HandleFunc("GET /api/tasks/{id}", h.getTask)
	mux.HandleFunc("PATCH /api/tasks/{id}", h.updateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", h.trashTask)
	mux.HandleFunc("POST /api/tasks/{id}/archive", h.archiveTask)
	mux.HandleFunc("POST /api/tasks/{id}/restore", h.restoreTask)
	mux.HandleFunc("POST /api/tasks/{id}/move", h.moveTask)
	mux.HandleFunc("POST /api/tasks/{id}/move-subgroup", h.moveTaskToSubgroup)
	mux.HandleFunc("POST /api/tasks/{id}/reorder", h.reorderTask)

	// Projects
	mux.HandleFunc("GET /api/projects", h.listProjects)
	mux.HandleFunc("POST /api/projects", h.createProject)
	mux.HandleFunc("GET /api/projects/{id}", h.getProject)
	mux.HandleFunc("PATCH /api/projects/{id}", h.updateProject)

	// Subgroups
	mux.HandleFunc("POST /api/projects/{pid}/subgroups", h.createSubgroup)
	mux.HandleFunc("PATCH /api/projects/{pid}/subgroups/{sid}", h.renameSubgroup)
	mux.HandleFunc("DELETE /api/projects/{pid}/subgroups/{sid}", h.deleteSubgroup)
	mux.HandleFunc("POST /api/projects/{pid}/subgroups/{sid}/tasks", h.addProjectTask)

	// Views & queries
	mux.HandleFunc("GET /api/views", h.listViews)
	mux.HandleFunc("GET /api/views/{name}/run", h.runView)
	mux.HandleFunc("POST /api/query", h.runQuery)
	mux.HandleFunc("POST /api/query/projects", h.queryProjects)

	// Weekly review
	mux.HandleFunc("GET /api/review/weekly", h.weeklyReview)

	// Process Inbox sessions
	mux.HandleFunc("POST /api/inbox-sessions", h.startInboxSession)
	mux.HandleFunc("GET /api/inbox-sessions/{id}", h.getInboxSession)
	mux.HandleFunc("PATCH /api/inbox-sessions/{id}/draft", h.updateInboxDraft)
	mux.HandleFunc("POST /api/inbox-sessions/{id}/decide", h.commitInboxDecision)
	mux.HandleFunc("POST /api/inbox-sessions/{id}/skip", h.skipInboxItem)
	mux.HandleFunc("DELETE /api/inbox-sessions/{id}", h.discardInboxSession)

	// Import / export
	mux.HandleFunc("POST /api/export-md", h.exportMarkdown)
	mux.HandleFunc("POST /api/import-md", h.importMarkdown)

	// SSE event stream
	mux.HandleFunc("GET /api/events", h.events)

	return cors(mux)
}

// cors wraps a handler to add Access-Control-Allow-Origin: * so Electron's
// Chromium renderer can fetch without CORS errors when using contextIsolation.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Handler struct
// ---------------------------------------------------------------------------

type handlers struct {
	core *core.Core
	bus  *eventBus
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeData(w http.ResponseWriter, v any) {
	writeJSON(w, http.StatusOK, map[string]any{"data": v})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------
// These are the deliberate public API shapes. Transitional bridge fields from
// core (Filename, SubgroupIx, SgIdx, Source as raw list-type string) are
// intentionally omitted so clients only see stable IDs.

type wireTask struct {
	ID          string     `json:"id"`
	Text        string     `json:"text"`
	State       string     `json:"state"`
	Created     time.Time  `json:"created"`
	ModifiedAt  *time.Time `json:"modified_at,omitempty"`
	Deadline    *time.Time `json:"deadline,omitempty"`
	Scheduled   *time.Time `json:"scheduled,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	URL         string     `json:"url,omitempty"`
	Notes       string     `json:"notes,omitempty"`
	WaitingOn   string     `json:"waiting_on,omitempty"`
	WaitingSince *time.Time `json:"waiting_since,omitempty"`
	Source      string     `json:"source,omitempty"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
}

func toWireTask(t model.Task) wireTask {
	return wireTask{
		ID:           t.ID,
		Text:         t.Text,
		State:        string(t.State),
		Created:      t.Created,
		ModifiedAt:   t.ModifiedAt,
		Deadline:     t.Deadline,
		Scheduled:    t.Scheduled,
		Tags:         t.Tags,
		URL:          t.URL,
		Notes:        t.Notes,
		WaitingOn:    t.WaitingOn,
		WaitingSince: t.WaitingSince,
		Source:       t.Source,
		ArchivedAt:   t.ArchivedAt,
	}
}

type wireSubGroup struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	State    string     `json:"state,omitempty"`
	Deadline *time.Time `json:"deadline,omitempty"`
	Tasks    []wireTask `json:"tasks"`
}

func toWireSubGroup(sg model.SubGroup) wireSubGroup {
	tasks := make([]wireTask, len(sg.Tasks))
	for i, t := range sg.Tasks {
		tasks[i] = toWireTask(t)
	}
	return wireSubGroup{
		ID:       sg.ID,
		Title:    sg.Title,
		State:    string(sg.State),
		Deadline: sg.Deadline,
		Tasks:    tasks,
	}
}

type wireProject struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	State            string         `json:"state,omitempty"`
	Deadline         *time.Time     `json:"deadline,omitempty"`
	Tags             []string       `json:"tags,omitempty"`
	URL              string         `json:"url,omitempty"`
	WaitingOn        string         `json:"waiting_on,omitempty"`
	DefinitionOfDone string         `json:"definition_of_done,omitempty"`
	SubGroups        []wireSubGroup `json:"sub_groups"`
}

func toWireProject(p model.Project) wireProject {
	sgs := make([]wireSubGroup, len(p.SubGroups))
	for i, sg := range p.SubGroups {
		sgs[i] = toWireSubGroup(sg)
	}
	return wireProject{
		ID:               p.ID,
		Title:            p.Title,
		State:            string(p.State),
		Deadline:         p.Deadline,
		Tags:             p.Tags,
		URL:              p.URL,
		WaitingOn:        p.WaitingOn,
		DefinitionOfDone: p.DefinitionOfDone,
		SubGroups:        sgs,
	}
}

type wireProjectSummary struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	State         string     `json:"state,omitempty"`
	Deadline      *time.Time `json:"deadline,omitempty"`
	Tags          []string   `json:"tags,omitempty"`
	SubGroupCount int        `json:"sub_group_count"`
	TaskCount     int        `json:"task_count"`
	NextAction    string     `json:"next_action,omitempty"`
}

func toWireProjectSummary(ps service.ProjectSummary) wireProjectSummary {
	return wireProjectSummary{
		ID:            ps.ID,
		Title:         ps.Title,
		State:         string(ps.State),
		Deadline:      ps.Deadline,
		Tags:          ps.Tags,
		SubGroupCount: ps.SubGroupCount,
		TaskCount:     ps.TaskCount,
		NextAction:    ps.NextAction,
	}
}

type wireTaskLocation struct {
	Kind       string   `json:"kind"`
	Task       wireTask `json:"task"`
	ProjectID  string   `json:"project_id,omitempty"`
	SubgroupID string   `json:"subgroup_id,omitempty"`
	Archived   bool     `json:"archived,omitempty"`
}

func toWireTaskLocation(loc core.TaskLocation) wireTaskLocation {
	return wireTaskLocation{
		Kind:       string(loc.Kind),
		Task:       toWireTask(loc.Task),
		ProjectID:  loc.ProjectID,
		SubgroupID: loc.SubgroupID,
		Archived:   loc.Archived,
	}
}

type wireProjectLocation struct {
	ProjectID string      `json:"project_id"`
	Project   wireProject `json:"project"`
}

func toWireProjectLocation(loc core.ProjectLocation) wireProjectLocation {
	return wireProjectLocation{
		ProjectID: loc.ProjectID,
		Project:   toWireProject(loc.Project),
	}
}

type wireViewTask struct {
	Task        wireTask `json:"task"`
	SourceLabel string   `json:"source_label"`
	ProjectID   string   `json:"project_id,omitempty"`
	IsProject   bool     `json:"is_project"`
	IsArchived  bool     `json:"is_archived"`
}

func toWireViewTask(vt service.ViewTask) wireViewTask {
	label := vt.Source
	archived := vt.Task.ArchivedAt != nil
	return wireViewTask{
		Task:        toWireTask(vt.Task),
		SourceLabel: label,
		ProjectID:   vt.ProjectID,
		IsProject:   vt.IsProject,
		IsArchived:  archived,
	}
}

type wireSavedView struct {
	Name            string `json:"name"`
	Query           string `json:"query"`
	IncludeArchived bool   `json:"include_archived"`
}

type wireInboxSession struct {
	ID       string                `json:"id"`
	Current  wireInboxSessionItem  `json:"current"`
	Progress wireInboxProgress     `json:"progress"`
	Summary  wireInboxSummary      `json:"summary"`
	Done     bool                  `json:"done"`
}

type wireInboxSessionItem struct {
	Original wireTask `json:"original"`
	Draft    wireTask `json:"draft"`
	Step     string   `json:"step"`
}

type wireInboxProgress struct {
	Current int `json:"current"`
	Total   int `json:"total"`
}

type wireInboxSummary struct {
	Trashed   int `json:"trashed"`
	Someday   int `json:"someday"`
	Done      int `json:"done"`
	Waiting   int `json:"waiting"`
	Refiled   int `json:"refiled"`
	ToProject int `json:"to_project"`
	Skipped   int `json:"skipped"`
}

func toWireInboxSession(s *core.InboxSession) wireInboxSession {
	return wireInboxSession{
		ID: s.ID,
		Current: wireInboxSessionItem{
			Original: toWireTask(s.Current.Original),
			Draft:    toWireTask(s.Current.Draft),
			Step:     string(s.Current.Step),
		},
		Progress: wireInboxProgress{
			Current: s.Progress.Current,
			Total:   s.Progress.Total,
		},
		Summary: wireInboxSummary{
			Trashed:   s.Summary.Trashed,
			Someday:   s.Summary.Someday,
			Done:      s.Summary.Done,
			Waiting:   s.Summary.Waiting,
			Refiled:   s.Summary.Refiled,
			ToProject: s.Summary.ToProject,
			Skipped:   s.Summary.Skipped,
		},
		Done: s.Done,
	}
}

// ---------------------------------------------------------------------------
// Task handlers
// ---------------------------------------------------------------------------

// GET /api/tasks
// Query params:
//
//	?list=in              → inbox only
//	?list=single-actions  → single actions only
//	?archived=true        → include archived tasks
//	(no params)           → all active tasks
func (h *handlers) listTasks(w http.ResponseWriter, r *http.Request) {
	list := r.URL.Query().Get("list")
	archived := r.URL.Query().Get("archived") == "true"

	switch list {
	case "in", "single-actions":
		lt := model.ListIn
		if list == "single-actions" {
			lt = model.ListSingleActions
		}
		tasks, err := h.core.UnderlyingStore().ReadList(lt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		wt := make([]wireTask, len(tasks.Tasks))
		for i, t := range tasks.Tasks {
			wt[i] = toWireTask(t)
		}
		writeData(w, wt)
	default:
		all, err := h.core.CollectViewTasks(archived)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		wt := make([]wireViewTask, len(all))
		for i, vt := range all {
			wt[i] = toWireViewTask(vt)
		}
		writeData(w, wt)
	}
}

// POST /api/inbox
func (h *handlers) captureToInbox(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text      string   `json:"text"`
		Deadline  *string  `json:"deadline"`
		Scheduled *string  `json:"scheduled"`
		Tags      []string `json:"tags"`
		URL       string   `json:"url"`
		Notes     string   `json:"notes"`
		WaitingOn string   `json:"waiting_on"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	opts := core.CaptureOpts{
		Tags:      body.Tags,
		URL:       body.URL,
		Notes:     body.Notes,
		WaitingOn: body.WaitingOn,
	}
	if body.Deadline != nil {
		t, err := parseDateTime(*body.Deadline)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid deadline: "+err.Error())
			return
		}
		opts.Deadline = &t
	}
	if body.Scheduled != nil {
		t, err := parseDateTime(*body.Scheduled)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid scheduled: "+err.Error())
			return
		}
		opts.Scheduled = &t
	}
	task, err := h.core.CaptureToInbox(body.Text, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "task_created", TaskID: task.ID})
	writeData(w, map[string]any{"task": toWireTask(task)})
}

// GET /api/tasks/{id}
func (h *handlers) getTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	loc, err := h.core.ResolveTask(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeData(w, map[string]any{"location": toWireTaskLocation(*loc)})
}

// PATCH /api/tasks/{id}
func (h *handlers) updateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Text      *string  `json:"text"`
		State     *string  `json:"state"`
		Deadline  *string  `json:"deadline"`
		Scheduled *string  `json:"scheduled"`
		Tags      *[]string `json:"tags"`
		URL       *string  `json:"url"`
		Notes     *string  `json:"notes"`
		WaitingOn *string  `json:"waiting_on"`
		Clear     []string `json:"clear"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	patch := core.TaskPatch{
		Text:      body.Text,
		Tags:      body.Tags,
		URL:       body.URL,
		Notes:     body.Notes,
		WaitingOn: body.WaitingOn,
		Clear:     body.Clear,
	}
	if body.State != nil {
		s := model.TaskState(*body.State)
		patch.State = &s
	}
	if body.Deadline != nil {
		t, err := parseDateTime(*body.Deadline)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid deadline: "+err.Error())
			return
		}
		patch.Deadline = &t
	}
	if body.Scheduled != nil {
		t, err := parseDateTime(*body.Scheduled)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid scheduled: "+err.Error())
			return
		}
		patch.Scheduled = &t
	}
	loc, err := h.core.UpdateTask(id, patch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "task_updated", TaskID: id})
	writeData(w, map[string]any{"location": toWireTaskLocation(loc)})
}

// DELETE /api/tasks/{id}
func (h *handlers) trashTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.core.TrashTask(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "task_trashed", TaskID: id})
	writeData(w, map[string]any{"trashed": true})
}

// POST /api/tasks/{id}/archive
func (h *handlers) archiveTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.core.ArchiveTask(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "task_archived", TaskID: id})
	writeData(w, map[string]any{"archived": true})
}

// POST /api/tasks/{id}/restore
func (h *handlers) restoreTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	loc, err := h.core.RestoreTask(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "task_restored", TaskID: id})
	writeData(w, map[string]any{"location": toWireTaskLocation(loc)})
}

// POST /api/tasks/{id}/move
// Body: {to: "list", list: "single-actions", state: "next-action"}
//
//	or  {to: "project", project_id: "...", subgroup_id: "...", state: "next-action"}
func (h *handlers) moveTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		To         string `json:"to"`
		List       string `json:"list"`
		ProjectID  string `json:"project_id"`
		SubgroupID string `json:"subgroup_id"`
		State      string `json:"state"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	state := model.TaskState(body.State)
	if state == "" {
		state = model.StateNextAction
	}

	var loc core.TaskLocation
	var err error

	switch body.To {
	case "list":
		lt := model.ListType(body.List)
		if lt == "" {
			writeError(w, http.StatusBadRequest, "list is required when to=list")
			return
		}
		loc, err = h.core.MoveTaskToList(id, lt, state)
	case "project":
		if body.ProjectID == "" || body.SubgroupID == "" {
			writeError(w, http.StatusBadRequest, "project_id and subgroup_id are required when to=project")
			return
		}
		loc, err = h.core.MoveTaskToProject(id, body.ProjectID, body.SubgroupID, state)
	default:
		writeError(w, http.StatusBadRequest, `to must be "list" or "project"`)
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "task_moved", TaskID: id, ProjectID: loc.ProjectID})
	writeData(w, map[string]any{"location": toWireTaskLocation(loc)})
}

// POST /api/tasks/{id}/move-subgroup
func (h *handlers) moveTaskToSubgroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		SubgroupID string `json:"subgroup_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.SubgroupID == "" {
		writeError(w, http.StatusBadRequest, "subgroup_id is required")
		return
	}
	if err := h.core.MoveTaskToSubgroup(id, body.SubgroupID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "task_moved", TaskID: id})
	writeData(w, map[string]any{"moved": true})
}

// POST /api/tasks/{id}/reorder
func (h *handlers) reorderTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Delta int `json:"delta"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := h.core.ReorderProjectTask(id, body.Delta); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Resolve the task to get project ID for the event.
	loc, _ := h.core.ResolveTask(id)
	pid := ""
	if loc != nil {
		pid = loc.ProjectID
	}
	h.bus.publish(event{Type: "task_moved", TaskID: id, ProjectID: pid})
	writeData(w, map[string]any{"reordered": true})
}

// ---------------------------------------------------------------------------
// Project handlers
// ---------------------------------------------------------------------------

// GET /api/projects
func (h *handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.core.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ws := make([]wireProjectSummary, len(summaries))
	for i, ps := range summaries {
		ws[i] = toWireProjectSummary(ps)
	}
	writeData(w, map[string]any{"projects": ws})
}

// POST /api/projects
func (h *handlers) createProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title         string `json:"title"`
		SubgroupTitle string `json:"subgroup_title"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if body.SubgroupTitle == "" {
		body.SubgroupTitle = "Tasks"
	}
	loc, err := h.core.CreateProject(body.Title, body.SubgroupTitle)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "project_created", ProjectID: loc.ProjectID})
	writeData(w, map[string]any{"location": toWireProjectLocation(loc)})
}

// GET /api/projects/{id}
func (h *handlers) getProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	loc, err := h.core.GetProject(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeData(w, map[string]any{"location": toWireProjectLocation(loc)})
}

// PATCH /api/projects/{id}
func (h *handlers) updateProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Title            *string   `json:"title"`
		State            *string   `json:"state"`
		Deadline         *string   `json:"deadline"`
		Tags             *[]string `json:"tags"`
		URL              *string   `json:"url"`
		WaitingOn        *string   `json:"waiting_on"`
		DefinitionOfDone *string   `json:"definition_of_done"`
		Clear            []string  `json:"clear"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	patch := core.ProjectPatch{
		Title:            body.Title,
		Tags:             body.Tags,
		URL:              body.URL,
		WaitingOn:        body.WaitingOn,
		DefinitionOfDone: body.DefinitionOfDone,
		Clear:            body.Clear,
	}
	if body.State != nil {
		s := model.TaskState(*body.State)
		patch.State = &s
	}
	if body.Deadline != nil {
		t, err := parseDateTime(*body.Deadline)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid deadline: "+err.Error())
			return
		}
		patch.Deadline = &t
	}
	loc, err := h.core.UpdateProject(id, patch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "project_updated", ProjectID: loc.ProjectID})
	writeData(w, map[string]any{"location": toWireProjectLocation(loc)})
}

// ---------------------------------------------------------------------------
// Subgroup handlers
// ---------------------------------------------------------------------------

// POST /api/projects/{pid}/subgroups
func (h *handlers) createSubgroup(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("pid")
	var body struct {
		Title string `json:"title"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	loc, err := h.core.CreateSubgroup(pid, body.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "subgroup_changed", ProjectID: loc.ProjectID})
	writeData(w, map[string]any{
		"project_id":  loc.ProjectID,
		"subgroup_id": loc.Subgroup.ID,
		"title":       loc.Subgroup.Title,
	})
}

// PATCH /api/projects/{pid}/subgroups/{sid}
func (h *handlers) renameSubgroup(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("pid")
	sid := r.PathValue("sid")
	var body struct {
		Title string `json:"title"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	loc, err := h.core.RenameSubgroup(pid, sid, body.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "subgroup_changed", ProjectID: loc.ProjectID})
	writeData(w, map[string]any{
		"project_id":  loc.ProjectID,
		"subgroup_id": loc.Subgroup.ID,
		"title":       loc.Subgroup.Title,
	})
}

// DELETE /api/projects/{pid}/subgroups/{sid}
func (h *handlers) deleteSubgroup(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("pid")
	sid := r.PathValue("sid")
	if err := h.core.DeleteSubgroup(pid, sid); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "subgroup_changed", ProjectID: pid})
	writeData(w, map[string]any{"deleted": true})
}

// POST /api/projects/{pid}/subgroups/{sid}/tasks
func (h *handlers) addProjectTask(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("pid")
	sid := r.PathValue("sid")
	var body struct {
		Text      string   `json:"text"`
		Deadline  *string  `json:"deadline"`
		Scheduled *string  `json:"scheduled"`
		Tags      []string `json:"tags"`
		URL       string   `json:"url"`
		Notes     string   `json:"notes"`
		WaitingOn string   `json:"waiting_on"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	opts := core.CaptureOpts{
		Tags:      body.Tags,
		URL:       body.URL,
		Notes:     body.Notes,
		WaitingOn: body.WaitingOn,
	}
	if body.Deadline != nil {
		t, err := parseDateTime(*body.Deadline)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid deadline: "+err.Error())
			return
		}
		opts.Deadline = &t
	}
	if body.Scheduled != nil {
		t, err := parseDateTime(*body.Scheduled)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid scheduled: "+err.Error())
			return
		}
		opts.Scheduled = &t
	}
	loc, err := h.core.AddProjectTask(pid, sid, body.Text, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "task_created", TaskID: loc.Task.ID, ProjectID: loc.ProjectID})
	writeData(w, map[string]any{"location": toWireTaskLocation(loc)})
}

// ---------------------------------------------------------------------------
// Views & query handlers
// ---------------------------------------------------------------------------

// GET /api/views
func (h *handlers) listViews(w http.ResponseWriter, r *http.Request) {
	views := h.core.ListViews()
	ws := make([]wireSavedView, len(views))
	for i, v := range views {
		ws[i] = wireSavedView{Name: v.Name, Query: v.Query, IncludeArchived: v.IncludeArchived}
	}
	writeData(w, map[string]any{"views": ws})
}

// GET /api/views/{name}/run
func (h *handlers) runView(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	results, err := h.core.RunView(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeData(w, map[string]any{"tasks": toWireViewTasks(results)})
}

// POST /api/query
func (h *handlers) runQuery(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query           string `json:"query"`
		IncludeArchived bool   `json:"include_archived"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	results, err := h.core.RunQuery(core.QueryInput{Query: body.Query, IncludeArchived: body.IncludeArchived})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeData(w, map[string]any{"tasks": toWireViewTasks(results)})
}

// POST /api/query/projects
func (h *handlers) queryProjects(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query string `json:"query"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	locs, err := h.core.QueryProjects(core.QueryInput{Query: body.Query})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ws := make([]wireProjectLocation, len(locs))
	for i, loc := range locs {
		ws[i] = toWireProjectLocation(loc)
	}
	writeData(w, map[string]any{"projects": ws})
}

func toWireViewTasks(vts []service.ViewTask) []wireViewTask {
	ws := make([]wireViewTask, len(vts))
	for i, vt := range vts {
		ws[i] = toWireViewTask(vt)
	}
	return ws
}

// ---------------------------------------------------------------------------
// Weekly review handler
// ---------------------------------------------------------------------------

// GET /api/review/weekly
func (h *handlers) weeklyReview(w http.ResponseWriter, r *http.Request) {
	data, err := h.core.WeeklyReview(time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type wireReviewSection struct {
		ProjectsWithoutNextAction []wireProjectSummary `json:"projects_without_next_action"`
		AgingWaitingFor           []wireViewTask       `json:"aging_waiting_for"`
		SomedayMaybe              []wireViewTask       `json:"someday_maybe"`
		RecentArchived            []wireViewTask       `json:"recent_archived"`
	}

	pwna := make([]wireProjectSummary, len(data.ProjectsWithoutNextAction))
	for i, ps := range data.ProjectsWithoutNextAction {
		pwna[i] = toWireProjectSummary(ps)
	}

	writeData(w, wireReviewSection{
		ProjectsWithoutNextAction: pwna,
		AgingWaitingFor:           toWireViewTasks(data.AgingWaitingFor),
		SomedayMaybe:              toWireViewTasks(data.SomedayMaybe),
		RecentArchived:            toWireViewTasks(data.RecentArchived),
	})
}

// ---------------------------------------------------------------------------
// Process Inbox session handlers
// ---------------------------------------------------------------------------

// POST /api/inbox-sessions
func (h *handlers) startInboxSession(w http.ResponseWriter, r *http.Request) {
	session, err := h.core.StartInboxSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, map[string]any{"session": toWireInboxSession(session)})
}

// GET /api/inbox-sessions/{id}
func (h *handlers) getInboxSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	session, err := h.core.GetInboxSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeData(w, map[string]any{"session": toWireInboxSession(session)})
}

// PATCH /api/inbox-sessions/{id}/draft
func (h *handlers) updateInboxDraft(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Text      *string   `json:"text"`
		State     *string   `json:"state"`
		Deadline  *string   `json:"deadline"`
		Scheduled *string   `json:"scheduled"`
		Tags      *[]string `json:"tags"`
		URL       *string   `json:"url"`
		Notes     *string   `json:"notes"`
		WaitingOn *string   `json:"waiting_on"`
		Clear     []string  `json:"clear"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	patch := core.TaskPatch{
		Text:      body.Text,
		Tags:      body.Tags,
		URL:       body.URL,
		Notes:     body.Notes,
		WaitingOn: body.WaitingOn,
		Clear:     body.Clear,
	}
	if body.State != nil {
		s := model.TaskState(*body.State)
		patch.State = &s
	}
	if body.Deadline != nil {
		t, err := parseDateTime(*body.Deadline)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid deadline: "+err.Error())
			return
		}
		patch.Deadline = &t
	}
	if body.Scheduled != nil {
		t, err := parseDateTime(*body.Scheduled)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid scheduled: "+err.Error())
			return
		}
		patch.Scheduled = &t
	}
	session, err := h.core.UpdateInboxDraft(id, patch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, map[string]any{"session": toWireInboxSession(session)})
}

// POST /api/inbox-sessions/{id}/decide
func (h *handlers) commitInboxDecision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Kind         string `json:"kind"`
		WaitingOn    string `json:"waiting_on"`
		ProjectID    string `json:"project_id"`
		ProjectTitle string `json:"project_title"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required")
		return
	}
	decision := core.InboxDecision{
		Kind:         core.InboxDecisionKind(body.Kind),
		WaitingOn:    body.WaitingOn,
		ProjectID:    body.ProjectID,
		ProjectTitle: body.ProjectTitle,
	}
	session, err := h.core.CommitInboxDecision(id, decision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.bus.publish(event{Type: "inbox_session_changed"})
	writeData(w, map[string]any{"session": toWireInboxSession(session)})
}

// POST /api/inbox-sessions/{id}/skip
func (h *handlers) skipInboxItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	session, err := h.core.SkipInboxItem(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, map[string]any{"session": toWireInboxSession(session)})
}

// DELETE /api/inbox-sessions/{id}
func (h *handlers) discardInboxSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.core.DiscardInboxSession(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeData(w, map[string]any{"discarded": true})
}

// ---------------------------------------------------------------------------
// Import / export handlers
// ---------------------------------------------------------------------------

// POST /api/export-md
func (h *handlers) exportMarkdown(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OutputDir string `json:"output_dir"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.OutputDir) == "" {
		writeError(w, http.StatusBadRequest, "output_dir is required")
		return
	}
	if err := h.core.ExportMarkdown(body.OutputDir); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, map[string]any{"exported": true, "output_dir": body.OutputDir})
}

// POST /api/import-md
func (h *handlers) importMarkdown(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dir    string `json:"dir"`
		Mode   string `json:"mode"`
		DryRun bool   `json:"dry_run"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Dir) == "" {
		writeError(w, http.StatusBadRequest, "dir is required")
		return
	}
	result, err := h.core.ImportMarkdown(core.ImportInput{
		Dir:    body.Dir,
		Mode:   core.ImportMode(body.Mode),
		DryRun: body.DryRun,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, result)
}

// ---------------------------------------------------------------------------
// SSE event bus and handler
// ---------------------------------------------------------------------------

type event struct {
	Type      string `json:"type"`
	TaskID    string `json:"task_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type eventBus struct {
	mu      sync.Mutex
	clients map[chan event]struct{}
}

func newEventBus() *eventBus {
	return &eventBus{clients: make(map[chan event]struct{})}
}

func (b *eventBus) subscribe() chan event {
	ch := make(chan event, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *eventBus) unsubscribe(ch chan event) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *eventBus) publish(e event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- e:
		default:
			// Slow client — drop rather than block the publisher.
		}
	}
}

// GET /api/events
func (h *handlers) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := h.bus.subscribe()
	defer h.bus.unsubscribe(ch)

	// Send an initial keep-alive comment so the client knows the connection is up.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(evt)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Date parsing helper
// ---------------------------------------------------------------------------

func parseDateTime(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
		time.RFC3339,
	} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date format %q (expected YYYY-MM-DD or YYYY-MM-DDTHH:MM)", s)
}
