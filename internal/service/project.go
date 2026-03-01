package service

import (
	"fmt"
	"time"

	"github.com/g-tuddy/g-tuddy/internal/id"
	"github.com/g-tuddy/g-tuddy/internal/model"
)

// ProjectSummary is a lightweight view of a project for list display.
type ProjectSummary struct {
	Filename      string
	Title         string
	ID            string
	State         model.TaskState
	Deadline      *time.Time
	Tags          []string
	SubGroupCount int
	TaskCount     int
	NextAction    string // text of the first next-action task, if any
}

// ListProjects returns summaries of all projects.
func (svc *Service) ListProjects() ([]ProjectSummary, error) {
	filenames, err := svc.Store.ListProjects()
	if err != nil {
		return nil, err
	}

	var summaries []ProjectSummary
	for _, fn := range filenames {
		proj, err := svc.Store.ReadProject(fn)
		if err != nil {
			// Skip unreadable projects rather than failing entirely.
			continue
		}

		s := ProjectSummary{
			Filename:      fn,
			Title:         proj.Title,
			ID:            proj.ID,
			State:         proj.State,
			Deadline:      proj.Deadline,
			Tags:          proj.Tags,
			SubGroupCount: len(proj.SubGroups),
		}

		for _, sg := range proj.SubGroups {
			s.TaskCount += len(sg.Tasks)
			if s.NextAction == "" {
				for _, t := range sg.Tasks {
					if t.State == model.StateNextAction {
						s.NextAction = t.Text
						break
					}
				}
			}
		}

		summaries = append(summaries, s)
	}

	return summaries, nil
}

// GetProject reads a full project by filename.
func (svc *Service) GetProject(filename string) (*model.Project, error) {
	return svc.Store.ReadProject(filename)
}

// CreateProject creates a new project with an initial sub-group.
func (svc *Service) CreateProject(title string, subGroupTitle string) (*model.Project, error) {
	proj := &model.Project{
		Title: title,
		ID:    id.New(),
		State: model.StateNextAction,
		Tags:  []string{"project"},
	}

	if subGroupTitle != "" {
		proj.SubGroups = []model.SubGroup{
			{
				Title: subGroupTitle,
				ID:    id.New(),
			},
		}
	}

	if err := svc.Store.WriteProject(proj); err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}
	return proj, nil
}

// AddSubGroup adds a new sub-group to an existing project.
func (svc *Service) AddSubGroup(filename string, title string) (*model.SubGroup, error) {
	proj, err := svc.Store.ReadProject(filename)
	if err != nil {
		return nil, err
	}

	sg := model.SubGroup{
		Title: title,
		ID:    id.New(),
	}
	proj.SubGroups = append(proj.SubGroups, sg)

	if err := svc.Store.WriteProject(proj); err != nil {
		return nil, err
	}
	return &sg, nil
}

// AddTaskToProject adds a task to a specific sub-group within a project.
func (svc *Service) AddTaskToProject(filename string, subGroupIdx int, text string, state model.TaskState) (*model.Task, error) {
	proj, err := svc.Store.ReadProject(filename)
	if err != nil {
		return nil, err
	}

	if subGroupIdx < 0 || subGroupIdx >= len(proj.SubGroups) {
		return nil, fmt.Errorf("sub-group index %d out of range (project has %d sub-groups)", subGroupIdx, len(proj.SubGroups))
	}

	task := model.Task{
		ID:      id.New(),
		Created: time.Now().Truncate(time.Minute),
		Text:    text,
		State:   state,
	}

	proj.SubGroups[subGroupIdx].Tasks = append(proj.SubGroups[subGroupIdx].Tasks, task)

	if err := svc.Store.WriteProject(proj); err != nil {
		return nil, err
	}
	return &task, nil
}

// MoveToProject moves a task from a list (e.g. inbox) into a project sub-group.
func (svc *Service) MoveToProject(fromList model.ListType, taskID string, projectFile string, subGroupIdx int, newState model.TaskState) error {
	// Read and remove from source list.
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
	src.Tasks = append(src.Tasks[:idx], src.Tasks[idx+1:]...)

	// Read project and add task to sub-group.
	proj, err := svc.Store.ReadProject(projectFile)
	if err != nil {
		return fmt.Errorf("reading project: %w", err)
	}

	if subGroupIdx < 0 || subGroupIdx >= len(proj.SubGroups) {
		return fmt.Errorf("sub-group index %d out of range", subGroupIdx)
	}

	proj.SubGroups[subGroupIdx].Tasks = append(proj.SubGroups[subGroupIdx].Tasks, task)

	// Write both.
	if err := svc.Store.WriteList(src); err != nil {
		return fmt.Errorf("writing source list: %w", err)
	}
	if err := svc.Store.WriteProject(proj); err != nil {
		return fmt.Errorf("writing project: %w", err)
	}
	return nil
}

// UpdateProjectTaskState changes a task's state within a project.
// If done/canceled, the task is archived and removed from the project.
func (svc *Service) UpdateProjectTaskState(filename string, subGroupIdx int, taskID string, newState model.TaskState) error {
	proj, err := svc.Store.ReadProject(filename)
	if err != nil {
		return err
	}

	if subGroupIdx < 0 || subGroupIdx >= len(proj.SubGroups) {
		return fmt.Errorf("sub-group index %d out of range", subGroupIdx)
	}

	sg := &proj.SubGroups[subGroupIdx]
	idx := findTaskIndex(sg.Tasks, taskID)
	if idx == -1 {
		return fmt.Errorf("task %s not found in sub-group %q", taskID, sg.Title)
	}

	task := &sg.Tasks[idx]
	task.State = newState

	if newState == model.StateWaitingFor && task.WaitingSince == nil {
		now := time.Now().Truncate(24 * time.Hour)
		task.WaitingSince = &now
	}

	if newState == model.StateDone || newState == model.StateCanceled {
		task.Source = fmt.Sprintf("projects/%s", filename)
		if err := svc.archiveTask(*task); err != nil {
			return fmt.Errorf("archiving task: %w", err)
		}
		sg.Tasks = append(sg.Tasks[:idx], sg.Tasks[idx+1:]...)
	}

	return svc.Store.WriteProject(proj)
}

// ReorderTaskInSubGroup moves a task up or down within its sub-group.
// delta is -1 for up, +1 for down.
func (svc *Service) ReorderTaskInSubGroup(filename string, subGroupIdx int, taskID string, delta int) error {
	proj, err := svc.Store.ReadProject(filename)
	if err != nil {
		return err
	}

	if subGroupIdx < 0 || subGroupIdx >= len(proj.SubGroups) {
		return fmt.Errorf("sub-group index %d out of range", subGroupIdx)
	}

	sg := &proj.SubGroups[subGroupIdx]
	idx := findTaskIndex(sg.Tasks, taskID)
	if idx == -1 {
		return fmt.Errorf("task %s not found in sub-group %q", taskID, sg.Title)
	}

	newIdx := idx + delta
	if newIdx < 0 || newIdx >= len(sg.Tasks) {
		return nil // at boundary, nothing to do
	}

	// Swap.
	sg.Tasks[idx], sg.Tasks[newIdx] = sg.Tasks[newIdx], sg.Tasks[idx]

	return svc.Store.WriteProject(proj)
}

// MoveTaskBetweenSubGroups moves a task from one sub-group to another within the same project.
func (svc *Service) MoveTaskBetweenSubGroups(filename string, fromSgIdx int, taskID string, toSgIdx int) error {
	proj, err := svc.Store.ReadProject(filename)
	if err != nil {
		return err
	}

	if fromSgIdx < 0 || fromSgIdx >= len(proj.SubGroups) {
		return fmt.Errorf("source sub-group index %d out of range", fromSgIdx)
	}
	if toSgIdx < 0 || toSgIdx >= len(proj.SubGroups) {
		return fmt.Errorf("destination sub-group index %d out of range", toSgIdx)
	}
	if fromSgIdx == toSgIdx {
		return nil // same sub-group, nothing to do
	}

	fromSg := &proj.SubGroups[fromSgIdx]
	idx := findTaskIndex(fromSg.Tasks, taskID)
	if idx == -1 {
		return fmt.Errorf("task %s not found in sub-group %q", taskID, fromSg.Title)
	}

	task := fromSg.Tasks[idx]
	fromSg.Tasks = append(fromSg.Tasks[:idx], fromSg.Tasks[idx+1:]...)

	toSg := &proj.SubGroups[toSgIdx]
	toSg.Tasks = append(toSg.Tasks, task)

	return svc.Store.WriteProject(proj)
}
