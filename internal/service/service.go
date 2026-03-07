// Package service implements GTD business logic on top of the store layer.
// It provides operations for adding tasks, updating state, moving tasks
// between lists, and archiving completed items.
package service

import (
	"fmt"
	"sort"
	"time"

	"github.com/wnwn/wnwn/internal/id"
	"github.com/wnwn/wnwn/internal/model"
	"github.com/wnwn/wnwn/internal/store"
)

// Service provides GTD task operations.
type Service struct {
	Store    *store.Store
	behavior BehaviorConfig
}

type BehaviorConfig struct {
	AutoArchiveDone     bool
	AutoArchiveCanceled bool
}

// WeeklyReviewData contains grouped review buckets used by weekly review mode.
type WeeklyReviewData struct {
	ProjectsWithoutNextAction []ProjectSummary
	AgingWaitingFor           []ViewTask
	SomedayMaybe              []ViewTask
	RecentArchived            []ViewTask
}

// New creates a Service backed by the given store.
func New(s *store.Store) *Service {
	return NewWithBehavior(s, BehaviorConfig{})
}

// NewWithBehavior creates a Service with configurable behavior flags.
func NewWithBehavior(s *store.Store, behavior BehaviorConfig) *Service {
	return &Service{Store: s, behavior: behavior}
}

// AddToInbox creates a new task and appends it to the inbox.
func (svc *Service) AddToInbox(text string, opts ...TaskOption) (*model.Task, error) {
	task := model.Task{
		ID:      id.New(),
		Created: time.Now().Truncate(time.Minute),
		Text:    text,
	}

	for _, opt := range opts {
		opt(&task)
	}

	list, err := svc.Store.ReadList(model.ListIn)
	if err != nil {
		return nil, fmt.Errorf("reading inbox: %w", err)
	}

	list.Tasks = append(list.Tasks, task)

	if err := svc.Store.WriteList(list); err != nil {
		return nil, fmt.Errorf("writing inbox: %w", err)
	}

	return &task, nil
}

// UpdateState changes a task's state within a list.
// If the new state is "waiting-for", it auto-sets WaitingSince.
func (svc *Service) UpdateState(listType model.ListType, taskID string, newState model.TaskState) error {
	list, err := svc.Store.ReadList(listType)
	if err != nil {
		return fmt.Errorf("reading list: %w", err)
	}

	idx := findTaskIndex(list.Tasks, taskID)
	if idx == -1 {
		return fmt.Errorf("task %s not found in %s", taskID, listType)
	}

	task := &list.Tasks[idx]
	task.State = newState

	if newState == model.StateWaitingFor && task.WaitingSince == nil {
		now := time.Now().Truncate(24 * time.Hour)
		task.WaitingSince = &now
	}

	if svc.shouldAutoArchive(newState) {
		task.Source = string(listType)
		if err := svc.archiveTask(*task); err != nil {
			return fmt.Errorf("archiving task: %w", err)
		}
		list.Tasks = append(list.Tasks[:idx], list.Tasks[idx+1:]...)
	}

	return svc.Store.WriteList(list)
}

// MoveToList moves a task from one list to another, setting its state.
func (svc *Service) MoveToList(fromList model.ListType, taskID string, toList model.ListType, newState model.TaskState) error {
	src, err := svc.Store.ReadList(fromList)
	if err != nil {
		return fmt.Errorf("reading source list: %w", err)
	}

	idx := findTaskIndex(src.Tasks, taskID)
	if idx == -1 {
		return fmt.Errorf("task %s not found in %s", taskID, fromList)
	}

	task := src.Tasks[idx]
	task.State = newState

	// Remove from source.
	src.Tasks = append(src.Tasks[:idx], src.Tasks[idx+1:]...)

	// Add to destination.
	dst, err := svc.Store.ReadList(toList)
	if err != nil {
		return fmt.Errorf("reading destination list: %w", err)
	}
	dst.Tasks = append(dst.Tasks, task)

	// Write both lists.
	if err := svc.Store.WriteList(src); err != nil {
		return fmt.Errorf("writing source list: %w", err)
	}
	if err := svc.Store.WriteList(dst); err != nil {
		return fmt.Errorf("writing destination list: %w", err)
	}

	return nil
}

// UpdateTask replaces all mutable fields of a task within a list.
// The task is identified by ID; ID, Created, and Source are never changed.
// If the new state is "waiting-for" and WaitingSince is not yet set, it is auto-set.
func (svc *Service) UpdateTask(listType model.ListType, updated model.Task) error {
	list, err := svc.Store.ReadList(listType)
	if err != nil {
		return fmt.Errorf("reading list: %w", err)
	}

	idx := findTaskIndex(list.Tasks, updated.ID)
	if idx == -1 {
		return fmt.Errorf("task %s not found in %s", updated.ID, listType)
	}

	// Preserve immutable fields.
	updated.Created = list.Tasks[idx].Created
	updated.Source = list.Tasks[idx].Source

	// Auto-set WaitingSince when transitioning to waiting-for.
	if updated.State == model.StateWaitingFor && updated.WaitingSince == nil {
		now := time.Now().Truncate(24 * time.Hour)
		updated.WaitingSince = &now
	}

	list.Tasks[idx] = updated

	if svc.shouldAutoArchive(updated.State) {
		list.Tasks[idx].Source = string(listType)
		if err := svc.archiveTask(list.Tasks[idx]); err != nil {
			return fmt.Errorf("archiving task: %w", err)
		}
		list.Tasks = append(list.Tasks[:idx], list.Tasks[idx+1:]...)
	}

	return svc.Store.WriteList(list)
}

// ArchiveTask moves a task from a list into the monthly archive file.
func (svc *Service) ArchiveTask(listType model.ListType, taskID string) error {
	list, err := svc.Store.ReadList(listType)
	if err != nil {
		return fmt.Errorf("reading list: %w", err)
	}

	idx := findTaskIndex(list.Tasks, taskID)
	if idx == -1 {
		return fmt.Errorf("task %s not found in %s", taskID, listType)
	}

	task := list.Tasks[idx]
	task.Source = string(listType)
	if err := svc.archiveTask(task); err != nil {
		return fmt.Errorf("archiving task: %w", err)
	}

	list.Tasks = append(list.Tasks[:idx], list.Tasks[idx+1:]...)
	return svc.Store.WriteList(list)
}

// TrashTask permanently removes a task from a list (no archive).
func (svc *Service) TrashTask(listType model.ListType, taskID string) error {
	list, err := svc.Store.ReadList(listType)
	if err != nil {
		return fmt.Errorf("reading list: %w", err)
	}

	idx := findTaskIndex(list.Tasks, taskID)
	if idx == -1 {
		return fmt.Errorf("task %s not found in %s", taskID, listType)
	}

	list.Tasks = append(list.Tasks[:idx], list.Tasks[idx+1:]...)
	return svc.Store.WriteList(list)
}

// archiveTask appends a task to the unified archive collection.
func (svc *Service) archiveTask(task model.Task) error {
	now := time.Now().Truncate(time.Second)
	filename := "archive.md"
	task.ArchivedAt = &now

	archive, err := svc.Store.ReadArchive(filename)
	if err != nil {
		// File doesn't exist yet — create it.
		archive = &model.TaskList{
			Title: "Archive",
			Type:  model.ListArchive,
		}
	}

	archive.Tasks = append(archive.Tasks, task)
	return svc.Store.WriteArchive(filename, archive)
}

// findTaskIndex returns the index of a task with the given ID, or -1.
func findTaskIndex(tasks []model.Task, id string) int {
	for i, t := range tasks {
		if t.ID == id {
			return i
		}
	}
	return -1
}

// TaskOption is a functional option for configuring a new task.
type TaskOption func(*model.Task)

// WithDeadline sets the task's deadline.
func WithDeadline(t time.Time) TaskOption {
	return func(task *model.Task) {
		task.Deadline = &t
	}
}

// WithScheduled sets the task's scheduled date.
func WithScheduled(t time.Time) TaskOption {
	return func(task *model.Task) {
		task.Scheduled = &t
	}
}

// WithTags sets the task's tags.
func WithTags(tags []string) TaskOption {
	return func(task *model.Task) {
		task.Tags = tags
	}
}

// WithURL sets the task's URL.
func WithURL(url string) TaskOption {
	return func(task *model.Task) {
		task.URL = url
	}
}

// WithNotes sets the task's notes.
func WithNotes(notes string) TaskOption {
	return func(task *model.Task) {
		task.Notes = notes
	}
}

// ── Cross-list aggregation ──────────────────────────────────────────────────

// ViewTask wraps a task with its source provenance for cross-list views.
type ViewTask struct {
	Task      model.Task
	Source    string // "in", "single-actions", or "projects/<filename>"
	SgIdx     int    // sub-group index within a project; -1 for list tasks
	Filename  string // project filename; empty for list tasks
	ListType  model.ListType
	IsProject bool
}

// CollectAllTasks reads inbox, single-actions, and all projects, returning
// every active task with provenance. No filtering is applied — that is the
// caller's responsibility. Archive files are not scanned.
func (svc *Service) CollectAllTasks() ([]ViewTask, error) {
	var results []ViewTask

	// Inbox and single-actions.
	for _, lt := range []model.ListType{model.ListIn, model.ListSingleActions} {
		list, err := svc.Store.ReadList(lt)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", lt, err)
		}
		source := string(lt)
		for _, task := range list.Tasks {
			results = append(results, ViewTask{
				Task:      task,
				Source:    source,
				SgIdx:     -1,
				ListType:  lt,
				IsProject: false,
			})
		}
	}

	// All projects.
	filenames, err := svc.Store.ListProjects()
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	for _, filename := range filenames {
		proj, err := svc.Store.ReadProject(filename)
		if err != nil {
			return nil, fmt.Errorf("reading project %s: %w", filename, err)
		}
		source := "projects/" + filename
		for sgIdx, sg := range proj.SubGroups {
			for _, task := range sg.Tasks {
				results = append(results, ViewTask{
					Task:      task,
					Source:    source,
					SgIdx:     sgIdx,
					Filename:  filename,
					IsProject: true,
				})
			}
		}
	}

	return results, nil
}

// CollectArchiveTasks reads all archive files and returns every archived task
// with provenance metadata for archive-focused views.
func (svc *Service) CollectArchiveTasks() ([]ViewTask, error) {
	var results []ViewTask

	archiveFiles, err := svc.Store.ListArchives()
	if err != nil {
		return nil, fmt.Errorf("listing archives: %w", err)
	}

	for _, filename := range archiveFiles {
		archive, err := svc.Store.ReadArchive(filename)
		if err != nil {
			return nil, fmt.Errorf("reading archive %s: %w", filename, err)
		}
		source := "archive"
		for _, task := range archive.Tasks {
			results = append(results, ViewTask{
				Task:      task,
				Source:    source,
				SgIdx:     -1,
				Filename:  filename,
				ListType:  model.ListArchive,
				IsProject: false,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		ai := results[i].Task.ArchivedAt
		aj := results[j].Task.ArchivedAt
		if ai == nil && aj == nil {
			return results[i].Task.Created.After(results[j].Task.Created)
		}
		if ai == nil {
			return false
		}
		if aj == nil {
			return true
		}
		return ai.After(*aj)
	})

	return results, nil
}

// WeeklyReview collects grouped items for a guided weekly review flow.
func (svc *Service) WeeklyReview(now time.Time) (WeeklyReviewData, error) {
	var out WeeklyReviewData

	projects, err := svc.ListProjects()
	if err != nil {
		return out, fmt.Errorf("listing projects: %w", err)
	}
	for _, p := range projects {
		if p.State == model.StateDone || p.State == model.StateCanceled || p.State == model.StateSomeday {
			continue
		}
		if p.TaskCount > 0 && p.NextAction == "" {
			out.ProjectsWithoutNextAction = append(out.ProjectsWithoutNextAction, p)
		}
	}

	allTasks, err := svc.CollectAllTasks()
	if err != nil {
		return out, fmt.Errorf("collecting active tasks: %w", err)
	}

	waitingCutoff := now.AddDate(0, 0, -7)
	for _, vt := range allTasks {
		t := vt.Task
		switch t.State {
		case model.StateWaitingFor:
			if t.WaitingSince != nil && !t.WaitingSince.After(waitingCutoff) {
				out.AgingWaitingFor = append(out.AgingWaitingFor, vt)
			}
		case model.StateSomeday:
			out.SomedayMaybe = append(out.SomedayMaybe, vt)
		}
	}

	archived, err := svc.CollectArchiveTasks()
	if err != nil {
		return out, fmt.Errorf("collecting archived tasks: %w", err)
	}
	archiveCutoff := now.AddDate(0, 0, -7)
	for _, vt := range archived {
		if vt.Task.ArchivedAt != nil && !vt.Task.ArchivedAt.Before(archiveCutoff) {
			out.RecentArchived = append(out.RecentArchived, vt)
		}
	}

	return out, nil
}

func (svc *Service) shouldAutoArchive(state model.TaskState) bool {
	if state == model.StateDone {
		return svc.behavior.AutoArchiveDone
	}
	if state == model.StateCanceled {
		return svc.behavior.AutoArchiveCanceled
	}
	return false
}

// WithWaitingOn sets who/what the task is waiting on and automatically sets waiting-for state.
func WithWaitingOn(person string) TaskOption {
	return func(task *model.Task) {
		task.WaitingOn = person
		task.State = model.StateWaitingFor
		now := time.Now().Truncate(24 * time.Hour)
		task.WaitingSince = &now
	}
}
