// Package tui implements the Bubbletea terminal user interface for g-tuddy.
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/g-tuddy/g-tuddy/internal/model"
	"github.com/g-tuddy/g-tuddy/internal/service"
	"github.com/g-tuddy/g-tuddy/internal/store"
)

// mode represents the current UI mode.
type mode int

const (
	modeNormal mode = iota
	modeAdding
	modeAddingProject    // creating a new project
	modeAddingSubGroup   // adding a sub-group to current project
	modeAddingProjectTask // adding a task to current project sub-group
	modePickingProject   // picking a project for refile
	modePickingSubGroup  // picking a sub-group to move a task into
)

// viewState tracks what the user is currently looking at.
type viewState int

const (
	viewList    viewState = iota // viewing a task list (inbox or single-actions)
	viewProjects                // viewing the project list
	viewProjectDetail           // viewing a single project's sub-groups and tasks
)

// Model is the top-level Bubbletea model for the application.
type Model struct {
	svc         *service.Service
	store       *store.Store
	list        *model.TaskList
	currentList model.ListType
	cursor      int
	mode        mode
	view        viewState
	input       textinput.Model
	width       int
	height      int
	statusMsg   string // temporary status message (e.g. "Task refiled")
	err         error

	// Project-related state.
	projects       []service.ProjectSummary
	activeProject  *model.Project           // the project being viewed in detail
	activeFilename string                   // filename of the active project
	projCursor     int                      // cursor within project detail (flat index across sub-groups)

	// Refile state: task being refiled to a project.
	refileTaskID   string
	refileTaskText string
	refileFromList model.ListType

	// Sub-group picker state: task being moved between sub-groups.
	moveTaskID   string
	moveTaskText string
	moveFromSgIdx int
}

// New creates a new TUI model backed by the given data directory.
func New(dataDir string) Model {
	s := store.New(dataDir)
	svc := service.New(s)

	ti := textinput.New()
	ti.Placeholder = "What needs to be done?"
	ti.CharLimit = 256
	ti.SetWidth(60)

	return Model{
		svc:         svc,
		store:       s,
		currentList: model.ListIn,
		input:       ti,
	}
}

// Init loads the current list on startup.
func (m Model) Init() tea.Cmd {
	return m.loadCurrentList
}

// loadCurrentList is a tea.Cmd that reads the current list from disk.
func (m Model) loadCurrentList() tea.Msg {
	if err := m.store.Init(); err != nil {
		return errMsg{err}
	}
	list, err := m.store.ReadList(m.currentList)
	if err != nil {
		return errMsg{err}
	}
	return listLoadedMsg{list}
}

// Messages
type listLoadedMsg struct{ list *model.TaskList }
type taskAddedMsg struct{ task *model.Task }
type taskRefiledMsg struct{ text string }
type clearStatusMsg struct{}
type projectsLoadedMsg struct{ projects []service.ProjectSummary }
type projectDetailMsg struct {
	project     *model.Project
	filename    string
	resetCursor bool // true when entering detail view for the first time
}
type projectCreatedMsg struct{ title string }
type errMsg struct{ err error }

// Update handles messages and user input.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case listLoadedMsg:
		m.list = msg.list
		m.clampCursor()
		return m, nil

	case taskAddedMsg:
		m.statusMsg = fmt.Sprintf("Added: %s", msg.task.Text)
		return m, tea.Batch(m.loadCurrentList, m.clearStatusAfter())

	case taskRefiledMsg:
		m.statusMsg = fmt.Sprintf("Refiled: %s", msg.text)
		// Reload whatever view we're on.
		var reload tea.Cmd
		if m.view == viewList {
			reload = m.loadCurrentList
		} else if m.view == viewProjects {
			reload = m.loadProjects
		} else if m.view == viewProjectDetail && m.activeFilename != "" {
			reload = m.reloadProjectDetail()
		}
		return m, tea.Batch(reload, m.clearStatusAfter())

	case projectsLoadedMsg:
		m.projects = msg.projects
		m.clampCursor()
		return m, nil

	case projectDetailMsg:
		m.activeProject = msg.project
		m.activeFilename = msg.filename
		m.view = viewProjectDetail
		if msg.resetCursor {
			m.projCursor = 0
		}
		return m, nil

	case projectCreatedMsg:
		m.statusMsg = fmt.Sprintf("Created project: %s", msg.title)
		return m, tea.Batch(m.loadProjects, m.clearStatusAfter())

	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyPressMsg:
		// Clear any status message on keypress.
		m.statusMsg = ""

		switch m.mode {
		case modeAdding:
			return m.updateAdding(msg)
		case modeAddingProject:
			return m.updateAddingProject(msg)
		case modeAddingSubGroup:
			return m.updateAddingSubGroup(msg)
		case modeAddingProjectTask:
			return m.updateAddingProjectTask(msg)
		case modePickingProject:
			return m.updatePickingProject(msg)
		case modePickingSubGroup:
			return m.updatePickingSubGroup(msg)
		default:
			switch m.view {
			case viewProjects:
				return m.updateProjectList(msg)
			case viewProjectDetail:
				return m.updateProjectDetail(msg)
			default:
				return m.updateNormal(msg)
			}
		}
	}

	// Pass through to text input in input modes.
	if m.mode == modeAdding || m.mode == modeAddingProject || m.mode == modeAddingSubGroup || m.mode == modeAddingProjectTask {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

// loadProjects loads the project list.
func (m Model) loadProjects() tea.Msg {
	if err := m.store.Init(); err != nil {
		return errMsg{err}
	}
	projects, err := m.svc.ListProjects()
	if err != nil {
		return errMsg{err}
	}
	return projectsLoadedMsg{projects}
}

// loadProjectDetail loads a full project by filename, resetting the cursor.
// Used when entering the detail view for the first time.
func (m Model) loadProjectDetail(filename string) tea.Cmd {
	return func() tea.Msg {
		proj, err := m.svc.GetProject(filename)
		if err != nil {
			return errMsg{err}
		}
		return projectDetailMsg{proj, filename, true}
	}
}

// reloadProjectDetail reloads the project without resetting the cursor.
// Used after mutations (reorder, add task, mark done, etc.).
func (m Model) reloadProjectDetail() tea.Cmd {
	filename := m.activeFilename
	return func() tea.Msg {
		proj, err := m.svc.GetProject(filename)
		if err != nil {
			return errMsg{err}
		}
		return projectDetailMsg{proj, filename, false}
	}
}

// clearStatusAfter returns a Cmd that clears the status message after a delay.
func (m Model) clearStatusAfter() tea.Cmd {
	return tea.Tick(time.Second*3, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

// updateNormal handles keys in normal (browsing) mode.
func (m Model) updateNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	// Navigation.
	case "j", "down":
		if m.list != nil && m.cursor < len(m.list.Tasks)-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case "g":
		m.cursor = 0

	case "G":
		if m.list != nil && len(m.list.Tasks) > 0 {
			m.cursor = len(m.list.Tasks) - 1
		}

	// List switching.
	case "1":
		m.view = viewList
		m.currentList = model.ListIn
		m.cursor = 0
		return m, m.loadCurrentList

	case "2":
		m.view = viewList
		m.currentList = model.ListSingleActions
		m.cursor = 0
		return m, m.loadCurrentList

	case "3":
		m.view = viewProjects
		m.cursor = 0
		return m, m.loadProjects

	case "tab":
		switch {
		case m.view == viewList && m.currentList == model.ListIn:
			m.currentList = model.ListSingleActions
			m.cursor = 0
			return m, m.loadCurrentList
		case m.view == viewList && m.currentList == model.ListSingleActions:
			m.view = viewProjects
			m.cursor = 0
			return m, m.loadProjects
		default:
			m.view = viewList
			m.currentList = model.ListIn
			m.cursor = 0
			return m, m.loadCurrentList
		}

	// Add task (inbox only).
	case "a":
		if m.currentList != model.ListIn {
			m.statusMsg = "Can only add tasks to the inbox"
			return m, m.clearStatusAfter()
		}
		m.mode = modeAdding
		m.input.Reset()
		cmd := m.input.Focus()
		return m, cmd

	// Refile to single-actions as next-action (inbox only).
	case "r":
		if m.currentList != model.ListIn || m.list == nil || len(m.list.Tasks) == 0 {
			return m, nil
		}
		task := m.list.Tasks[m.cursor]
		return m, m.refileTask(task.ID, task.Text, model.ListSingleActions, model.StateNextAction)

	// Move to someday/maybe (inbox or single-actions).
	case "s":
		if m.list == nil || len(m.list.Tasks) == 0 {
			return m, nil
		}
		task := m.list.Tasks[m.cursor]
		return m, m.setStateSomeday(task.ID, task.Text)

	// Refile to a project (from inbox or single-actions).
	case "p":
		if m.list == nil || len(m.list.Tasks) == 0 {
			return m, nil
		}
		task := m.list.Tasks[m.cursor]
		m.refileTaskID = task.ID
		m.refileTaskText = task.Text
		m.refileFromList = m.currentList
		m.mode = modePickingProject
		m.cursor = 0
		return m, m.loadProjects

	// Set to waiting-for.
	case "w":
		if m.list == nil || len(m.list.Tasks) == 0 {
			return m, nil
		}
		task := m.list.Tasks[m.cursor]
		return m, m.setStateWaiting(task.ID, task.Text)

	// Mark done.
	case "d":
		if m.list != nil && len(m.list.Tasks) > 0 {
			task := m.list.Tasks[m.cursor]
			_ = m.svc.UpdateState(m.list.Type, task.ID, model.StateDone)
			return m, m.loadCurrentList
		}

	// Trash task.
	case "x":
		if m.list != nil && len(m.list.Tasks) > 0 {
			task := m.list.Tasks[m.cursor]
			_ = m.svc.TrashTask(m.list.Type, task.ID)
			return m, m.loadCurrentList
		}
	}

	return m, nil
}

// refileTask moves a task from the current list to a destination list.
func (m Model) refileTask(taskID, text string, destList model.ListType, newState model.TaskState) tea.Cmd {
	return func() tea.Msg {
		err := m.svc.MoveToList(m.currentList, taskID, destList, newState)
		if err != nil {
			return errMsg{err}
		}
		return taskRefiledMsg{text}
	}
}

// setStateSomeday sets a task to someday/maybe state.
// If on inbox, refiles to single-actions. If already on single-actions, updates in place.
func (m Model) setStateSomeday(taskID, text string) tea.Cmd {
	return func() tea.Msg {
		if m.currentList == model.ListIn {
			err := m.svc.MoveToList(model.ListIn, taskID, model.ListSingleActions, model.StateSomeday)
			if err != nil {
				return errMsg{err}
			}
		} else {
			err := m.svc.UpdateState(m.currentList, taskID, model.StateSomeday)
			if err != nil {
				return errMsg{err}
			}
		}
		return taskRefiledMsg{text}
	}
}

// setStateWaiting sets a task to waiting-for state.
// If on inbox, refiles to single-actions. If already on single-actions, updates in place.
func (m Model) setStateWaiting(taskID, text string) tea.Cmd {
	return func() tea.Msg {
		if m.currentList == model.ListIn {
			err := m.svc.MoveToList(model.ListIn, taskID, model.ListSingleActions, model.StateWaitingFor)
			if err != nil {
				return errMsg{err}
			}
		} else {
			err := m.svc.UpdateState(m.currentList, taskID, model.StateWaitingFor)
			if err != nil {
				return errMsg{err}
			}
		}
		return taskRefiledMsg{text}
	}
}

// updateProjectList handles keys when viewing the project list.
func (m Model) updateProjectList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		if m.cursor < len(m.projects)-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case "g":
		m.cursor = 0

	case "G":
		if len(m.projects) > 0 {
			m.cursor = len(m.projects) - 1
		}

	case "enter":
		if len(m.projects) > 0 {
			fn := m.projects[m.cursor].Filename
			return m, m.loadProjectDetail(fn)
		}

	case "a":
		m.mode = modeAddingProject
		m.input.Reset()
		m.input.Placeholder = "Project name"
		cmd := m.input.Focus()
		return m, cmd

	case "1":
		m.view = viewList
		m.currentList = model.ListIn
		m.cursor = 0
		return m, m.loadCurrentList

	case "2":
		m.view = viewList
		m.currentList = model.ListSingleActions
		m.cursor = 0
		return m, m.loadCurrentList

	case "3":
		// Already here.

	case "tab":
		m.view = viewList
		m.currentList = model.ListIn
		m.cursor = 0
		return m, m.loadCurrentList
	}

	return m, nil
}

// updateProjectDetail handles keys when viewing a single project.
func (m Model) updateProjectDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	flatItems := m.flattenProject()

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Go back to project list.
		m.view = viewProjects
		m.cursor = 0
		return m, m.loadProjects

	case "j", "down":
		if m.projCursor < len(flatItems)-1 {
			m.projCursor++
		}

	case "k", "up":
		if m.projCursor > 0 {
			m.projCursor--
		}

	case "g":
		m.projCursor = 0

	case "G":
		if len(flatItems) > 0 {
			m.projCursor = len(flatItems) - 1
		}

	case "a":
		// Add task to the sub-group the cursor is in.
		if m.activeProject == nil || len(m.activeProject.SubGroups) == 0 {
			return m, nil
		}
		m.mode = modeAddingProjectTask
		m.input.Reset()
		m.input.Placeholder = "New task"
		cmd := m.input.Focus()
		return m, cmd

	case "n":
		// Add new sub-group.
		m.mode = modeAddingSubGroup
		m.input.Reset()
		m.input.Placeholder = "Sub-group name"
		cmd := m.input.Focus()
		return m, cmd

	case "d":
		// Mark task done.
		if len(flatItems) > 0 {
			item := flatItems[m.projCursor]
			if item.isTask {
				_ = m.svc.UpdateProjectTaskState(m.activeFilename, item.sgIdx, item.task.ID, model.StateDone)
				return m, m.reloadProjectDetail()
			}
		}

	case "ctrl+k":
		// Move task up within its sub-group.
		if len(flatItems) > 0 {
			item := flatItems[m.projCursor]
			if item.isTask {
				err := m.svc.ReorderTaskInSubGroup(m.activeFilename, item.sgIdx, item.task.ID, -1)
				if err == nil {
					m.projCursor--
					if m.projCursor < 0 {
						m.projCursor = 0
					}
				}
				return m, m.reloadProjectDetail()
			}
		}

	case "ctrl+j":
		// Move task down within its sub-group.
		if len(flatItems) > 0 {
			item := flatItems[m.projCursor]
			if item.isTask {
				err := m.svc.ReorderTaskInSubGroup(m.activeFilename, item.sgIdx, item.task.ID, 1)
				if err == nil {
					m.projCursor++
					if m.projCursor >= len(flatItems) {
						m.projCursor = len(flatItems) - 1
					}
				}
				return m, m.reloadProjectDetail()
			}
		}

	case "m":
		// Move task to a different sub-group.
		if m.activeProject == nil || len(m.activeProject.SubGroups) < 2 {
			m.statusMsg = "Need at least 2 sub-groups to move between"
			return m, m.clearStatusAfter()
		}
		if len(flatItems) > 0 {
			item := flatItems[m.projCursor]
			if item.isTask {
				m.moveTaskID = item.task.ID
				m.moveTaskText = item.task.Text
				m.moveFromSgIdx = item.sgIdx
				m.mode = modePickingSubGroup
				m.cursor = 0
				return m, nil
			}
		}

	case "1":
		m.view = viewList
		m.currentList = model.ListIn
		m.cursor = 0
		return m, m.loadCurrentList

	case "2":
		m.view = viewList
		m.currentList = model.ListSingleActions
		m.cursor = 0
		return m, m.loadCurrentList

	case "3":
		m.view = viewProjects
		m.cursor = 0
		return m, m.loadProjects

	case "tab":
		m.view = viewList
		m.currentList = model.ListIn
		m.cursor = 0
		return m, m.loadCurrentList
	}

	return m, nil
}

// updatePickingProject handles keys when picking a project for refile.
func (m Model) updatePickingProject(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.refileTaskID = ""
		m.refileTaskText = ""
		return m, m.loadCurrentList

	case "j", "down":
		if m.cursor < len(m.projects)-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case "enter":
		if len(m.projects) > 0 {
			proj := m.projects[m.cursor]
			m.mode = modeNormal
			// Move to first sub-group of selected project (index 0).
			return m, m.moveToProject(proj.Filename, proj.Title)
		}
	}

	return m, nil
}

// updatePickingSubGroup handles keys when picking a destination sub-group.
func (m Model) updatePickingSubGroup(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.moveTaskID = ""
		m.moveTaskText = ""
		return m, nil

	case "j", "down":
		if m.cursor < len(m.activeProject.SubGroups)-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case "enter":
		if m.activeProject != nil && m.cursor < len(m.activeProject.SubGroups) {
			toSgIdx := m.cursor
			m.mode = modeNormal
			taskID := m.moveTaskID
			taskText := m.moveTaskText
			fromSgIdx := m.moveFromSgIdx
			filename := m.activeFilename
			return m, func() tea.Msg {
				err := m.svc.MoveTaskBetweenSubGroups(filename, fromSgIdx, taskID, toSgIdx)
				if err != nil {
					return errMsg{err}
				}
				return taskRefiledMsg{fmt.Sprintf("Moved \"%s\" to %s", taskText, m.activeProject.SubGroups[toSgIdx].Title)}
			}
		}
	}

	return m, nil
}

// moveToProject moves the refile task to the first sub-group of a project.
func (m Model) moveToProject(filename, projTitle string) tea.Cmd {
	taskID := m.refileTaskID
	taskText := m.refileTaskText
	return func() tea.Msg {
		// Ensure project has at least one sub-group.
		proj, err := m.svc.GetProject(filename)
		if err != nil {
			return errMsg{err}
		}
		sgIdx := 0
		if len(proj.SubGroups) == 0 {
			// Create a default sub-group.
			_, err = m.svc.AddSubGroup(filename, "Tasks")
			if err != nil {
				return errMsg{err}
			}
		}
		err = m.svc.MoveToProject(m.refileFromList, taskID, filename, sgIdx, model.StateNextAction)
		if err != nil {
			return errMsg{err}
		}
		return taskRefiledMsg{fmt.Sprintf("%s -> %s", taskText, projTitle)}
	}
}

// updateAddingProject handles input when creating a new project.
func (m Model) updateAddingProject(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		title := strings.TrimSpace(m.input.Value())
		if title == "" {
			m.mode = modeNormal
			return m, nil
		}
		m.mode = modeNormal
		return m, func() tea.Msg {
			_, err := m.svc.CreateProject(title, "Tasks")
			if err != nil {
				return errMsg{err}
			}
			return projectCreatedMsg{title}
		}

	case "esc":
		m.mode = modeNormal
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateAddingSubGroup handles input when adding a sub-group.
func (m Model) updateAddingSubGroup(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		title := strings.TrimSpace(m.input.Value())
		if title == "" {
			m.mode = modeNormal
			return m, nil
		}
		m.mode = modeNormal
		return m, func() tea.Msg {
			_, err := m.svc.AddSubGroup(m.activeFilename, title)
			if err != nil {
				return errMsg{err}
			}
			return taskRefiledMsg{fmt.Sprintf("Added sub-group: %s", title)}
		}

	case "esc":
		m.mode = modeNormal
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateAddingProjectTask handles input when adding a task to a project.
func (m Model) updateAddingProjectTask(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			m.mode = modeNormal
			return m, nil
		}
		m.mode = modeNormal
		// Find which sub-group the cursor is in.
		sgIdx := m.currentSubGroupIdx()
		return m, func() tea.Msg {
			_, err := m.svc.AddTaskToProject(m.activeFilename, sgIdx, text, model.StateNextAction)
			if err != nil {
				return errMsg{err}
			}
			return taskRefiledMsg{fmt.Sprintf("Added: %s", text)}
		}

	case "esc":
		m.mode = modeNormal
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// flatItem represents a row in the flattened project detail view.
type flatItem struct {
	isTask bool
	sgIdx  int
	sgTitle string
	task   model.Task
}

// flattenProject creates a flat list of items for the project detail view.
// Each sub-group heading is an item, followed by its tasks.
func (m Model) flattenProject() []flatItem {
	if m.activeProject == nil {
		return nil
	}
	var items []flatItem
	for i, sg := range m.activeProject.SubGroups {
		items = append(items, flatItem{isTask: false, sgIdx: i, sgTitle: sg.Title})
		for _, t := range sg.Tasks {
			items = append(items, flatItem{isTask: true, sgIdx: i, task: t})
		}
	}
	return items
}

// currentSubGroupIdx returns the sub-group index the cursor is currently in.
func (m Model) currentSubGroupIdx() int {
	items := m.flattenProject()
	if len(items) == 0 {
		return 0
	}
	idx := m.projCursor
	if idx >= len(items) {
		idx = len(items) - 1
	}
	return items[idx].sgIdx
}

// updateAdding handles keys when the user is typing a new task.
func (m Model) updateAdding(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			m.mode = modeNormal
			return m, nil
		}
		m.mode = modeNormal
		return m, m.addTask(text)

	case "esc":
		m.mode = modeNormal
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// addTask returns a Cmd that adds a task to the inbox.
func (m Model) addTask(text string) tea.Cmd {
	return func() tea.Msg {
		task, err := m.svc.AddToInbox(text)
		if err != nil {
			return errMsg{err}
		}
		return taskAddedMsg{task}
	}
}

// View renders the TUI.
func (m Model) View() tea.View {
	var v tea.View
	v.AltScreen = true

	if m.err != nil {
		v.SetContent(fmt.Sprintf("\n  Error: %v\n\n  Press q to quit.\n", m.err))
		return v
	}

	var b strings.Builder

	// Tab bar (always shown).
	b.WriteString(m.renderTabBar())
	b.WriteString("\n\n")

	switch {
	case m.mode == modePickingProject:
		m.renderProjectPicker(&b)
	case m.mode == modePickingSubGroup:
		m.renderSubGroupPicker(&b)
	case m.view == viewProjects:
		m.renderProjectListView(&b)
	case m.view == viewProjectDetail:
		m.renderProjectDetailView(&b)
	default:
		m.renderListView(&b)
	}

	// Input area (for various input modes).
	if m.mode == modeAdding || m.mode == modeAddingProject || m.mode == modeAddingSubGroup || m.mode == modeAddingProjectTask {
		b.WriteString("\n")
		var prompt string
		switch m.mode {
		case modeAdding:
			prompt = "  New task: "
		case modeAddingProject:
			prompt = "  New project: "
		case modeAddingSubGroup:
			prompt = "  New sub-group: "
		case modeAddingProjectTask:
			prompt = "  New task: "
		}
		b.WriteString(inputPromptStyle.Render(prompt))
		b.WriteString(m.input.View())
		b.WriteString("\n")
	}

	// Status message.
	if m.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(statusMsgStyle.Render("  " + m.statusMsg))
		b.WriteString("\n")
	}

	// Help bar.
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  " + m.helpText()))
	b.WriteString("\n")

	v.SetContent(b.String())
	return v
}

// renderListView renders the task list (inbox or single-actions).
func (m Model) renderListView(b *strings.Builder) {
	if m.list == nil {
		b.WriteString("  Loading...")
		return
	}

	if len(m.list.Tasks) == 0 {
		emptyMsg := "No tasks here."
		if m.currentList == model.ListIn {
			emptyMsg = "Inbox is empty. Press 'a' to add a task."
		}
		b.WriteString(taskStyle.Render(emptyMsg))
		b.WriteString("\n")
	} else {
		for i, task := range m.list.Tasks {
			b.WriteString(m.renderTask(i, task))
			b.WriteString("\n")
		}
	}
}

// renderProjectListView renders the list of all projects.
func (m Model) renderProjectListView(b *strings.Builder) {
	if len(m.projects) == 0 {
		b.WriteString(taskStyle.Render("No projects yet. Press 'a' to create one."))
		b.WriteString("\n")
		return
	}

	for i, p := range m.projects {
		isSelected := i == m.cursor

		if isSelected {
			b.WriteString(cursorStyle.Render(" > "))
		} else {
			b.WriteString("   ")
		}

		// Project title.
		titleText := p.Title
		if isSelected {
			b.WriteString(selectedTaskStyle.Render(titleText))
		} else {
			b.WriteString(titleText)
		}

		// Metadata.
		var meta []string
		if p.State != model.StateEmpty {
			meta = append(meta, stateStyle.Render(string(p.State)))
		}
		meta = append(meta, stateStyle.Render(fmt.Sprintf("%d tasks", p.TaskCount)))
		if p.Deadline != nil {
			meta = append(meta, deadlineStyle.Render("due:"+p.Deadline.Format("2006-01-02")))
		}
		if p.NextAction != "" {
			meta = append(meta, tagStyle.Render("next: "+p.NextAction))
		}
		if len(meta) > 0 {
			b.WriteString("  ")
			b.WriteString(strings.Join(meta, " "))
		}
		b.WriteString("\n")
	}
}

// renderProjectDetailView renders the expanded project with sub-groups and tasks.
func (m Model) renderProjectDetailView(b *strings.Builder) {
	if m.activeProject == nil {
		b.WriteString("  Loading...")
		return
	}

	// Project title.
	b.WriteString(projectTitleStyle.Render(m.activeProject.Title))
	b.WriteString("\n\n")

	items := m.flattenProject()
	if len(items) == 0 {
		b.WriteString(taskStyle.Render("No sub-groups yet. Press 'n' to add one."))
		b.WriteString("\n")
		return
	}

	for i, item := range items {
		isSelected := i == m.projCursor

		if item.isTask {
			// Render as task (indented under sub-group).
			if isSelected {
				b.WriteString(cursorStyle.Render("   > "))
			} else {
				b.WriteString("     ")
			}

			checkbox := model.CheckboxFor(item.task.State)
			switch checkbox {
			case model.CheckboxDone:
				b.WriteString(checkboxDoneStyle.Render(fmt.Sprintf("[x] %s", item.task.Text)))
			case model.CheckboxCanceled:
				b.WriteString(checkboxCanceledStyle.Render(fmt.Sprintf("[-] %s", item.task.Text)))
			default:
				if isSelected {
					b.WriteString(selectedTaskStyle.Render(fmt.Sprintf("[ ] %s", item.task.Text)))
				} else {
					b.WriteString(checkboxOpenStyle.Render("[ ] "))
					b.WriteString(item.task.Text)
				}
			}

			// Inline metadata.
			var meta []string
			if item.task.State != model.StateEmpty && item.task.State != model.StateDone {
				meta = append(meta, stateStyle.Render(string(item.task.State)))
			}
			if item.task.Deadline != nil {
				meta = append(meta, deadlineStyle.Render("due:"+item.task.Deadline.Format("2006-01-02")))
			}
			if len(meta) > 0 {
				b.WriteString("  ")
				b.WriteString(strings.Join(meta, " "))
			}
		} else {
			// Render as sub-group heading.
			if isSelected {
				b.WriteString(cursorStyle.Render(" > "))
			} else {
				b.WriteString("   ")
			}
			b.WriteString(subGroupStyle.Render(item.sgTitle))
		}
		b.WriteString("\n")
	}
}

// renderProjectPicker renders the project selection list for refiling.
func (m Model) renderProjectPicker(b *strings.Builder) {
	b.WriteString(inputPromptStyle.Render(fmt.Sprintf("  Refile \"%s\" to project:", m.refileTaskText)))
	b.WriteString("\n\n")

	if len(m.projects) == 0 {
		b.WriteString(taskStyle.Render("No projects yet. Press esc to cancel."))
		b.WriteString("\n")
		return
	}

	for i, p := range m.projects {
		isSelected := i == m.cursor

		if isSelected {
			b.WriteString(cursorStyle.Render(" > "))
			b.WriteString(selectedTaskStyle.Render(p.Title))
		} else {
			b.WriteString("   ")
			b.WriteString(p.Title)
		}
		b.WriteString(stateStyle.Render(fmt.Sprintf("  (%d tasks)", p.TaskCount)))
		b.WriteString("\n")
	}
}

// renderSubGroupPicker renders the sub-group selection list for moving a task.
func (m Model) renderSubGroupPicker(b *strings.Builder) {
	b.WriteString(inputPromptStyle.Render(fmt.Sprintf("  Move \"%s\" to sub-group:", m.moveTaskText)))
	b.WriteString("\n\n")

	if m.activeProject == nil {
		return
	}

	for i, sg := range m.activeProject.SubGroups {
		isSelected := i == m.cursor
		isCurrent := i == m.moveFromSgIdx

		if isSelected {
			b.WriteString(cursorStyle.Render(" > "))
			b.WriteString(selectedTaskStyle.Render(sg.Title))
		} else {
			b.WriteString("   ")
			b.WriteString(sg.Title)
		}

		if isCurrent {
			b.WriteString(stateStyle.Render("  (current)"))
		} else {
			b.WriteString(stateStyle.Render(fmt.Sprintf("  (%d tasks)", len(sg.Tasks))))
		}
		b.WriteString("\n")
	}
}

// renderTask renders a single task line.
func (m Model) renderTask(idx int, task model.Task) string {
	var b strings.Builder

	isSelected := idx == m.cursor

	// Cursor indicator.
	if isSelected {
		b.WriteString(cursorStyle.Render(" > "))
	} else {
		b.WriteString("   ")
	}

	// Checkbox.
	checkbox := model.CheckboxFor(task.State)
	switch checkbox {
	case model.CheckboxDone:
		b.WriteString(checkboxDoneStyle.Render(fmt.Sprintf("[x] %s", task.Text)))
	case model.CheckboxCanceled:
		b.WriteString(checkboxCanceledStyle.Render(fmt.Sprintf("[-] %s", task.Text)))
	default:
		if isSelected {
			b.WriteString(selectedTaskStyle.Render(fmt.Sprintf("[ ] %s", task.Text)))
		} else {
			b.WriteString(checkboxOpenStyle.Render("[ ] "))
			b.WriteString(task.Text)
		}
	}

	// Inline metadata hints.
	var meta []string
	if task.State != model.StateEmpty && task.State != model.StateDone && task.State != model.StateCanceled {
		meta = append(meta, stateStyle.Render(string(task.State)))
	}
	if task.Deadline != nil {
		meta = append(meta, deadlineStyle.Render("due:"+task.Deadline.Format("2006-01-02")))
	}
	if len(task.Tags) > 0 {
		for _, tag := range task.Tags {
			meta = append(meta, tagStyle.Render(tag))
		}
	}
	if len(meta) > 0 {
		b.WriteString("  ")
		b.WriteString(strings.Join(meta, " "))
	}

	return b.String()
}

// renderTabBar renders the list switcher tabs.
func (m Model) renderTabBar() string {
	inboxLabel := " 1 Inbox "
	actionsLabel := " 2 Actions "
	projectsLabel := " 3 Projects "

	// Build each tab with optional count.
	var tabs [3]string

	if m.view == viewList && m.currentList == model.ListIn {
		count := 0
		if m.list != nil {
			count = len(m.list.Tasks)
		}
		tabs[0] = activeTabStyle.Render(fmt.Sprintf("%s(%d)", inboxLabel, count))
	} else {
		tabs[0] = inactiveTabStyle.Render(inboxLabel)
	}

	if m.view == viewList && m.currentList == model.ListSingleActions {
		count := 0
		if m.list != nil {
			count = len(m.list.Tasks)
		}
		tabs[1] = activeTabStyle.Render(fmt.Sprintf("%s(%d)", actionsLabel, count))
	} else {
		tabs[1] = inactiveTabStyle.Render(actionsLabel)
	}

	if m.view == viewProjects || m.view == viewProjectDetail {
		count := len(m.projects)
		tabs[2] = activeTabStyle.Render(fmt.Sprintf("%s(%d)", projectsLabel, count))
	} else {
		tabs[2] = inactiveTabStyle.Render(projectsLabel)
	}

	return tabs[0] + "  " + tabs[1] + "  " + tabs[2]
}

// helpText returns contextual help based on mode and current view.
func (m Model) helpText() string {
	if m.mode == modeAdding || m.mode == modeAddingProject || m.mode == modeAddingSubGroup || m.mode == modeAddingProjectTask {
		return "enter: save  esc: cancel"
	}
	if m.mode == modePickingProject || m.mode == modePickingSubGroup {
		return "enter: select  esc: cancel  j/k: navigate"
	}

	nav := "j/k: navigate  tab: switch list  q: quit"

	switch m.view {
	case viewProjects:
		return "enter: open  a: new project  " + nav
	case viewProjectDetail:
		return "a: add task  n: new sub-group  d: done  C-j/C-k: reorder  m: move to sub-group  esc: back  " + nav
	default:
		if m.currentList == model.ListIn {
			return "a: add  r: refile  p: to project  s: someday  w: waiting  d: done  x: trash  " + nav
		}
		return "p: to project  s: someday  w: waiting  d: done  x: trash  " + nav
	}
}

// clampCursor ensures the cursor is within bounds for the current view.
func (m *Model) clampCursor() {
	var max int
	switch m.view {
	case viewProjects:
		max = len(m.projects)
	case viewProjectDetail:
		max = len(m.flattenProject())
	default:
		if m.list == nil {
			max = 0
		} else {
			max = len(m.list.Tasks)
		}
	}

	if max == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= max {
		m.cursor = max - 1
	}
}
