// Package service implements GTD business logic on top of the store layer.
// It provides operations for adding tasks, updating state, moving tasks
// between lists, and archiving completed items.
package service

import (
	"fmt"
	"time"

	"github.com/g-tuddy/g-tuddy/internal/id"
	"github.com/g-tuddy/g-tuddy/internal/model"
	"github.com/g-tuddy/g-tuddy/internal/store"
)

// Service provides GTD task operations.
type Service struct {
	Store *store.Store
}

// New creates a Service backed by the given store.
func New(s *store.Store) *Service {
	return &Service{Store: s}
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
// If the new state is "done" or "canceled", it archives the task.
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

	// If done or canceled, archive and remove from list.
	if newState == model.StateDone || newState == model.StateCanceled {
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
// If the new state is "done" or "canceled", the task is archived and removed.
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

	// Archive if done or canceled.
	if updated.State == model.StateDone || updated.State == model.StateCanceled {
		list.Tasks[idx].Source = string(listType)
		if err := svc.archiveTask(list.Tasks[idx]); err != nil {
			return fmt.Errorf("archiving task: %w", err)
		}
		list.Tasks = append(list.Tasks[:idx], list.Tasks[idx+1:]...)
	}

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

// archiveTask appends a task to the appropriate monthly archive file.
func (svc *Service) archiveTask(task model.Task) error {
	now := time.Now()
	filename := now.Format("2006-01") + ".md"

	archive, err := svc.Store.ReadArchive(filename)
	if err != nil {
		// File doesn't exist yet — create it.
		archive = &model.TaskList{
			Title: "Archive " + now.Format("January 2006"),
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

// WithWaitingOn sets who/what the task is waiting on and automatically sets waiting-for state.
func WithWaitingOn(person string) TaskOption {
	return func(task *model.Task) {
		task.WaitingOn = person
		task.State = model.StateWaitingFor
		now := time.Now().Truncate(24 * time.Hour)
		task.WaitingSince = &now
	}
}
