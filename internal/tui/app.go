// Package tui implements the Bubbletea terminal user interface for wnwn.
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/wnwn/wnwn/internal/model"
	"github.com/wnwn/wnwn/internal/query"
	"github.com/wnwn/wnwn/internal/service"
	"github.com/wnwn/wnwn/internal/store"
	"github.com/wnwn/wnwn/internal/tui/datepicker"
)

// mode represents the current UI mode.
type mode int

const (
	modeNormal mode = iota
	modeAdding
	modeAddingProject     // creating a new project
	modeAddingSubGroup    // adding a sub-group to current project
	modeAddingProjectTask // adding a task to current project sub-group
	modePickingProject    // picking a project for refile
	modePickingSubGroup   // picking a sub-group to move a task into
	modeEditingField      // editing a field in the task detail view
	modePickingDate       // date picker overlay for deadline/scheduled fields
)

// viewState tracks what the user is currently looking at.
type viewState int

const (
	viewList          viewState = iota // viewing a task list (inbox or single-actions)
	viewProjects                       // viewing the project list
	viewProjectDetail                  // viewing a single project's sub-groups and tasks
	viewTaskDetail                     // viewing/editing a single task's attributes
	viewProcessInbox                   // guided GTD decision tree for processing inbox items
	viewProjectEdit                    // editing project-level metadata (title, state, tags, etc.)
	viewViews                          // saved view list (4th tab)
	viewViewResults                    // filtered task results for the active or ad-hoc view
)

// projEditField enumerates the editable fields in the project edit view.
type projEditField int

const (
	projFieldTitle projEditField = iota
	projFieldState
	projFieldTags
	projFieldDeadline
	projFieldURL
	projFieldWaitingOn
	projFieldDefinitionOfDone
	projFieldCount // sentinel — keep last
)

// projEditFieldOrder defines the visual and navigation order of fields in the project edit view.
var projEditFieldOrder = []projEditField{
	projFieldTitle, projFieldState, projFieldTags,
	projFieldDeadline, projFieldURL, projFieldWaitingOn, projFieldDefinitionOfDone,
}

// processStep enumerates the steps in the process inbox decision tree.
type processStep int

const (
	stepActionable    processStep = iota // "Is it actionable?" decision
	stepNotActionable                    // "Trash or Someday/Maybe?" decision
	stepEnrich                           // Enrich task fields hub (edit text/tags/deadline/schedule/notes)
	stepEnrichTags                       // Adding tags one at a time (tab to confirm each)
	stepRoute                            // "Where does it go?" routing decision
	stepWaitingOn                        // Text input for waiting_on
	stepNewProject                       // Text input for new project title
	stepComplete                         // Summary screen (inbox processed)
)

// detailFieldOrder defines the visual (and navigation) order of fields in the
// task detail view. Navigation with j/k follows this slice, not the raw enum
// int, so reordering fields here is the single source of truth.
var detailFieldOrder = []detailField{
	fieldText, fieldState, fieldTags, fieldScheduled,
	fieldDeadline, fieldURL, fieldWaitingOn, fieldNotes,
}

// processStats tracks counts per action type for the completion summary.
type processStats struct {
	trashed   int
	someday   int
	done      int
	waiting   int
	refiled   int // to single actions
	toProject int
	skipped   int
}

// detailField enumerates the fields shown in the task detail view.
type detailField int

const (
	fieldText detailField = iota
	fieldState
	fieldTags
	fieldDeadline
	fieldScheduled
	fieldURL
	fieldWaitingOn
	fieldNotes
	fieldCount // sentinel — keep last
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
	activeProject  *model.Project // the project being viewed in detail
	activeFilename string         // filename of the active project
	projCursor     int            // cursor within project detail (flat index across sub-groups)

	// Refile state: task being refiled to a project.
	refileTaskID   string
	refileTaskText string
	refileFromList model.ListType

	// Sub-group picker state: task being moved between sub-groups.
	moveTaskID    string
	moveTaskText  string
	moveFromSgIdx int

	// Task detail / edit state.
	detailTask      model.Task     // a working copy of the task being viewed/edited
	detailField     detailField    // which field is currently selected
	detailFromView  viewState      // view to return to on esc
	detailFromList  model.ListType // set when returning to a list view
	detailFromSgIdx int            // set when returning to a project detail view
	detailIsProject bool           // true if task lives in a project (not a flat list)

	// Date picker state.
	datePicker      datepicker.Model // calendar date picker component
	datePickerField detailField      // which detail field the picker is editing

	// Project edit state.
	projEditProject  model.Project // working copy of the project being edited
	projEditFilename string        // original filename (before possible rename)
	projEditField    projEditField // which field is currently selected
	projEditFromView viewState     // view to return to on esc/save

	// Views state.
	savedViews      []model.SavedView  // list of saved views (hardcoded defaults)
	viewListCursor  int                // cursor in the saved view list
	activeViewName  string             // name of the currently open view (or "Ad-hoc")
	activeViewQuery string             // the query string for the active view
	viewResults     []service.ViewTask // filtered tasks for the active view
	viewCursor      int                // cursor within view results

	// Process inbox state.
	processItems []model.Task // snapshot of inbox tasks taken at activation
	processIdx   int          // index of the current item being processed (0-based)
	processStep  processStep  // current step in the decision tree
	processTask  model.Task   // working copy of the current item (mutated during enrichment)
	processTags  []string     // tag accumulator for stepEnrichTags (tab-separated entry)
	processStats processStats // running totals for the completion summary
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
		datePicker:  datepicker.New(),
		savedViews:  model.DefaultViews(),
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
type taskUpdatedMsg struct{ text string }
type clearStatusMsg struct{}
type projectsLoadedMsg struct{ projects []service.ProjectSummary }
type projectDetailMsg struct {
	project     *model.Project
	filename    string
	resetCursor bool // true when entering detail view for the first time
}
type projectCreatedMsg struct {
	title    string
	filename string // slug filename of the created project
}

// viewResultsLoadedMsg carries the results of a CollectAllTasks+filter operation.
type viewResultsLoadedMsg struct {
	name     string
	queryStr string
	results  []service.ViewTask
	err      error
}

type projectUpdatedMsg struct {
	title       string
	newFilename string
}
type errMsg struct{ err error }

// processInboxLoadedMsg carries the inbox snapshot when entering process inbox mode.
type processInboxLoadedMsg struct{ tasks []model.Task }

// processAdvancedMsg signals that the current item was acted on and we should advance.
// The action field is used to update processStats.
type processAdvancedMsg struct{ action string }

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
		// Process inbox uses processAdvancedMsg for advancing; skip list reload.
		if m.view == viewProcessInbox {
			return m, nil
		}
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

	case taskUpdatedMsg:
		m.statusMsg = fmt.Sprintf("Saved: %s", msg.text)
		// Return to the originating view and reload.
		m.view = m.detailFromView
		var reload tea.Cmd
		if m.detailFromView == viewList {
			m.currentList = m.detailFromList
			reload = m.loadCurrentList
		} else if m.detailFromView == viewProjectDetail && m.activeFilename != "" {
			reload = m.reloadProjectDetail()
		} else if m.detailFromView == viewViewResults {
			// saveDetailTask returns viewResultsLoadedMsg for this case,
			// so we should not reach here normally; fall back to viewViews.
			m.view = viewViews
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
		if m.view == viewProcessInbox {
			// Project was created during process inbox; now refile the current task into it.
			return m, m.processRefileToNewProject(msg.filename)
		}
		m.statusMsg = fmt.Sprintf("Created project: %s", msg.title)
		return m, tea.Batch(m.loadProjects, m.clearStatusAfter())

	case viewResultsLoadedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, m.clearStatusAfter()
		}
		m.activeViewName = msg.name
		m.activeViewQuery = msg.queryStr
		m.viewResults = msg.results
		m.viewCursor = 0
		m.view = viewViewResults
		return m, nil

	case projectEditLoadedMsg:
		m.projEditProject = msg.project
		m.projEditFilename = msg.filename
		m.projEditFromView = msg.fromView
		m.projEditField = projEditFieldOrder[0]
		m.view = viewProjectEdit
		return m, nil

	case projectUpdatedMsg:
		m.statusMsg = fmt.Sprintf("Saved: %s", msg.title)
		m.activeFilename = msg.newFilename
		if m.projEditFromView == viewProjectDetail {
			// Stay in project detail but reload with new filename.
			m.view = viewProjectDetail
			return m, tea.Batch(m.loadProjectDetail(msg.newFilename), m.clearStatusAfter())
		}
		// Return to project list.
		m.view = viewProjects
		m.cursor = 0
		return m, tea.Batch(m.loadProjects, m.clearStatusAfter())

	case processInboxLoadedMsg:
		if len(msg.tasks) == 0 {
			m.statusMsg = "Inbox is empty — nothing to process"
			return m, m.clearStatusAfter()
		}
		m.processItems = msg.tasks
		m.processIdx = 0
		m.processStep = stepActionable
		m.processTask = msg.tasks[0]
		m.processStats = processStats{}
		m.view = viewProcessInbox
		m.currentList = model.ListIn
		return m, nil

	case processAdvancedMsg:
		// Update stats.
		switch msg.action {
		case "trashed":
			m.processStats.trashed++
		case "someday":
			m.processStats.someday++
		case "done":
			m.processStats.done++
		case "waiting":
			m.processStats.waiting++
		case "refiled":
			m.processStats.refiled++
		case "toProject":
			m.processStats.toProject++
		case "skipped":
			m.processStats.skipped++
		}
		m.advanceProcessInbox()
		return m, nil

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
		case modeEditingField:
			return m.updateEditingField(msg)
		case modePickingDate:
			return m.updatePickingDate(msg)
		default:
			switch m.view {
			case viewProjects:
				return m.updateProjectList(msg)
			case viewProjectDetail:
				return m.updateProjectDetail(msg)
			case viewTaskDetail:
				return m.updateTaskDetail(msg)
			case viewProcessInbox:
				return m.updateProcessInbox(msg)
			case viewProjectEdit:
				return m.updateProjectEdit(msg)
			case viewViews:
				return m.updateViewList(msg)
			case viewViewResults:
				return m.updateViewResults(msg)
			default:
				return m.updateNormal(msg)
			}
		}
	}

	// Pass through to text input in input modes.
	if m.mode == modeAdding || m.mode == modeAddingProject || m.mode == modeAddingSubGroup || m.mode == modeAddingProjectTask || m.mode == modeEditingField {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	// Pass through to date picker.
	if m.mode == modePickingDate {
		var cmd tea.Cmd
		m.datePicker, cmd = m.datePicker.Update(msg)
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

	case "4", "V":
		m.view = viewViews
		m.viewListCursor = 0
		return m, nil

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
		case m.view == viewProjects:
			m.view = viewViews
			m.viewListCursor = 0
			return m, nil
		default:
			m.view = viewList
			m.currentList = model.ListIn
			m.cursor = 0
			return m, m.loadCurrentList
		}

	// Open task detail view.
	case "enter":
		if m.list != nil && len(m.list.Tasks) > 0 {
			task := m.list.Tasks[m.cursor]
			m.detailTask = task
			m.detailField = fieldText
			m.detailFromView = viewList
			m.detailFromList = m.currentList
			m.detailIsProject = false
			m.view = viewTaskDetail
			return m, nil
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

	// Process inbox (capital P) — guided GTD decision tree.
	case "P":
		if m.currentList != model.ListIn {
			m.statusMsg = "Process inbox only available from the Inbox tab"
			return m, m.clearStatusAfter()
		}
		if m.list != nil && len(m.list.Tasks) == 0 {
			m.statusMsg = "Inbox is empty — nothing to process"
			return m, m.clearStatusAfter()
		}
		return m, m.loadInboxForProcessing
	}

	return m, nil
}

// loadInboxForProcessing reads the inbox and returns a processInboxLoadedMsg to kick off process mode.
func (m Model) loadInboxForProcessing() tea.Msg {
	list, err := m.store.ReadList(model.ListIn)
	if err != nil {
		return errMsg{err}
	}
	return processInboxLoadedMsg{tasks: list.Tasks}
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

// ── Process Inbox Mode ───────────────────────────────────────────────────────

// advanceProcessInbox moves to the next item after an action completes.
// If all items are processed it switches to the completion step.
func (m *Model) advanceProcessInbox() {
	m.processIdx++
	if m.processIdx >= len(m.processItems) {
		m.processStep = stepComplete
		return
	}
	m.processStep = stepActionable
	m.processTask = m.processItems[m.processIdx]
	m.processTags = nil
}

// processRefileToNewProject refills the current process task into a freshly
// created project. Called from the projectCreatedMsg handler when in process inbox view.
func (m Model) processRefileToNewProject(filename string) tea.Cmd {
	task := m.processTask
	return func() tea.Msg {
		// Persist any enrichment to the inbox first.
		if err := m.svc.UpdateTask(model.ListIn, task); err != nil {
			return errMsg{err}
		}
		if err := m.svc.MoveToProject(model.ListIn, task.ID, filename, 0, model.StateNextAction); err != nil {
			return errMsg{err}
		}
		return processAdvancedMsg{action: "toProject"}
	}
}

// updateProcessInbox handles all keys while viewProcessInbox is active.
// It dispatches by processStep, with shared mode handlers taking priority.
func (m Model) updateProcessInbox(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.processStep {
	case stepActionable:
		return m.updateProcessStepActionable(msg)
	case stepNotActionable:
		return m.updateProcessStepNotActionable(msg)
	case stepEnrich:
		return m.updateProcessStepEnrich(msg)
	case stepEnrichTags:
		return m.updateProcessStepEnrichTags(msg)
	case stepRoute:
		return m.updateProcessStepRoute(msg)
	case stepWaitingOn:
		return m.updateProcessStepWaitingOn(msg)
	case stepNewProject:
		return m.updateProcessStepNewProject(msg)
	case stepComplete:
		// Any keypress returns to inbox.
		m.view = viewList
		m.currentList = model.ListIn
		return m, m.loadCurrentList
	}
	return m, nil
}

func (m Model) updateProcessStepActionable(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		m.processStep = stepEnrich
	case "n":
		m.processStep = stepNotActionable
	case "s":
		// Skip: leave item in inbox, advance.
		m.processStats.skipped++
		m.advanceProcessInbox()
	case "q", "esc":
		// Quit process inbox, return to inbox view.
		m.view = viewList
		m.currentList = model.ListIn
		return m, m.loadCurrentList
	}
	return m, nil
}

func (m Model) updateProcessStepNotActionable(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "t":
		// Trash immediately — no enrichment needed for non-actionable items.
		task := m.processTask
		return m, func() tea.Msg {
			if err := m.svc.TrashTask(model.ListIn, task.ID); err != nil {
				return errMsg{err}
			}
			return processAdvancedMsg{action: "trashed"}
		}
	case "esc":
		m.processStep = stepActionable
	}
	return m, nil
}

func (m Model) updateProcessStepEnrich(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "t":
		// Edit task text.
		m.mode = modeEditingField
		m.detailField = fieldText
		m.input.Reset()
		m.input.Placeholder = "Task text"
		m.input.SetValue(m.processTask.Text)
		cmd := m.input.Focus()
		return m, cmd

	case "g":
		// Add tags (one at a time).
		// Seed processTags from the current task's tags so they display correctly.
		m.processTags = append([]string(nil), m.processTask.Tags...)
		m.processStep = stepEnrichTags
		m.input.Reset()
		m.input.Placeholder = "tag name"
		cmd := m.input.Focus()
		return m, cmd

	case "d":
		// Set deadline via date picker.
		m.mode = modePickingDate
		m.datePickerField = fieldDeadline
		var initial time.Time
		if m.processTask.Deadline != nil {
			initial = *m.processTask.Deadline
		}
		cmd := m.datePicker.Open(initial)
		return m, cmd

	case "c":
		// Set schedule via date picker.
		m.mode = modePickingDate
		m.datePickerField = fieldScheduled
		var initial time.Time
		if m.processTask.Scheduled != nil {
			initial = *m.processTask.Scheduled
		}
		cmd := m.datePicker.Open(initial)
		return m, cmd

	case "n":
		// Edit notes.
		m.mode = modeEditingField
		m.detailField = fieldNotes
		m.input.Reset()
		m.input.Placeholder = "Notes"
		m.input.SetValue(m.processTask.Notes)
		cmd := m.input.Focus()
		return m, cmd

	case "enter":
		// Always proceed to the route step after enrichment.
		m.processStep = stepRoute

	case "esc":
		// Back to actionable step; revert working copy.
		m.processStep = stepActionable
		m.processTask = m.processItems[m.processIdx]
	}
	return m, nil
}

func (m Model) updateProcessStepEnrichTags(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		// Confirm current tag and clear input for the next one.
		tag := strings.TrimSpace(m.input.Value())
		if tag != "" {
			m.processTags = append(m.processTags, tag)
		}
		m.input.Reset()
		return m, m.input.Focus()

	case "enter":
		// Finish: flush any pending tag and merge into working copy.
		tag := strings.TrimSpace(m.input.Value())
		if tag != "" {
			m.processTags = append(m.processTags, tag)
		}
		m.processTask.Tags = m.processTags
		m.processStep = stepEnrich
		m.mode = modeNormal

	case "esc":
		// Discard this tag session, return to enrich without changing tags.
		m.processStep = stepEnrich
		m.mode = modeNormal
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateProcessStepRoute(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "d":
		// Done (< 2 min, did it). Persist enrichment + mark done (auto-archives).
		task := m.processTask
		return m, func() tea.Msg {
			if err := m.svc.UpdateTask(model.ListIn, task); err != nil {
				return errMsg{err}
			}
			if err := m.svc.UpdateState(model.ListIn, task.ID, model.StateDone); err != nil {
				return errMsg{err}
			}
			return processAdvancedMsg{action: "done"}
		}

	case "w":
		// Waiting for: go to waiting_on input step.
		m.processStep = stepWaitingOn
		m.input.Reset()
		m.input.Placeholder = "Waiting on"
		m.input.SetValue(m.processTask.WaitingOn)
		cmd := m.input.Focus()
		return m, cmd

	case "s":
		// Someday/maybe: actionable but deferred indefinitely.
		task := m.processTask
		return m, func() tea.Msg {
			if err := m.svc.UpdateTask(model.ListIn, task); err != nil {
				return errMsg{err}
			}
			if err := m.svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateSomeday); err != nil {
				return errMsg{err}
			}
			return processAdvancedMsg{action: "someday"}
		}

	case "r":
		// Refile to single actions as next-action.
		task := m.processTask
		return m, func() tea.Msg {
			if err := m.svc.UpdateTask(model.ListIn, task); err != nil {
				return errMsg{err}
			}
			if err := m.svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateNextAction); err != nil {
				return errMsg{err}
			}
			return processAdvancedMsg{action: "refiled"}
		}

	case "p":
		// Pick an existing project.
		m.refileTaskID = m.processTask.ID
		m.refileTaskText = m.processTask.Text
		m.refileFromList = model.ListIn
		m.mode = modePickingProject
		m.cursor = 0
		return m, m.loadProjects

	case "n":
		// Create a new project: go to project title input step.
		m.processStep = stepNewProject
		m.input.Reset()
		m.input.Placeholder = "Project title"
		cmd := m.input.Focus()
		return m, cmd

	case "esc":
		m.processStep = stepEnrich
	}
	return m, nil
}

func (m Model) updateProcessStepWaitingOn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		waitingOn := strings.TrimSpace(m.input.Value())
		m.processTask.WaitingOn = waitingOn
		task := m.processTask
		return m, func() tea.Msg {
			if err := m.svc.UpdateTask(model.ListIn, task); err != nil {
				return errMsg{err}
			}
			if err := m.svc.MoveToList(model.ListIn, task.ID, model.ListSingleActions, model.StateWaitingFor); err != nil {
				return errMsg{err}
			}
			return processAdvancedMsg{action: "waiting"}
		}

	case "esc":
		m.processStep = stepRoute
		m.mode = modeNormal
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateProcessStepNewProject(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		title := strings.TrimSpace(m.input.Value())
		if title == "" {
			return m, nil
		}
		m.mode = modeNormal
		// Persist enrichment before creating project + refiling.
		task := m.processTask
		return m, func() tea.Msg {
			if err := m.svc.UpdateTask(model.ListIn, task); err != nil {
				return errMsg{err}
			}
			_, err := m.svc.CreateProject(title, "Tasks")
			if err != nil {
				return errMsg{err}
			}
			filename := store.Slugify(title) + ".md"
			return projectCreatedMsg{title: title, filename: filename}
		}

	case "esc":
		m.processStep = stepRoute
		m.mode = modeNormal
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
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

	case "E":
		// Open edit view for the selected project.
		if len(m.projects) > 0 {
			return m, m.openProjectEdit(m.projects[m.cursor].Filename, viewProjects)
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
		// Already here.

	case "4", "V":
		m.view = viewViews
		m.viewListCursor = 0
		return m, nil

	case "tab":
		m.view = viewViews
		m.viewListCursor = 0
		return m, nil
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

	case "enter":
		// Open task detail for the selected task.
		if len(flatItems) > 0 {
			item := flatItems[m.projCursor]
			if item.isTask {
				m.detailTask = item.task
				m.detailField = fieldText
				m.detailFromView = viewProjectDetail
				m.detailIsProject = true
				m.detailFromSgIdx = item.sgIdx
				m.view = viewTaskDetail
				return m, nil
			}
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

	case "E":
		// Open edit view for the active project.
		if m.activeFilename != "" {
			return m, m.openProjectEdit(m.activeFilename, viewProjectDetail)
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

	case "4", "V":
		m.view = viewViews
		m.viewListCursor = 0
		return m, nil

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
		if m.view == viewProcessInbox {
			// Return to the route step, not the list.
			m.processStep = stepRoute
			return m, nil
		}
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
			if m.view == viewProcessInbox {
				// Persist enrichment then refile into the chosen project.
				return m, m.processMoveToProject(proj.Filename, proj.Title)
			}
			// Move to first sub-group of selected project (index 0).
			return m, m.moveToProject(proj.Filename, proj.Title)
		}
	}

	return m, nil
}

// processMoveToProject persists enrichment edits then moves the current process
// task into the first sub-group of the chosen project.
func (m Model) processMoveToProject(filename, projTitle string) tea.Cmd {
	task := m.processTask
	return func() tea.Msg {
		// Persist enrichment to inbox first.
		if err := m.svc.UpdateTask(model.ListIn, task); err != nil {
			return errMsg{err}
		}
		// Ensure the project has at least one sub-group.
		proj, err := m.svc.GetProject(filename)
		if err != nil {
			return errMsg{err}
		}
		if len(proj.SubGroups) == 0 {
			if _, err := m.svc.AddSubGroup(filename, "Tasks"); err != nil {
				return errMsg{err}
			}
		}
		if err := m.svc.MoveToProject(model.ListIn, task.ID, filename, 0, model.StateNextAction); err != nil {
			return errMsg{err}
		}
		return processAdvancedMsg{action: "toProject"}
	}
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
			filename := store.Slugify(title) + ".md"
			return projectCreatedMsg{title: title, filename: filename}
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
	isTask  bool
	sgIdx   int
	sgTitle string
	task    model.Task
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
	case m.mode == modePickingDate:
		m.renderDatePicker(&b)
	case m.view == viewProcessInbox:
		m.renderProcessInbox(&b)
	case m.view == viewTaskDetail:
		m.renderTaskDetailView(&b)
	case m.view == viewProjectEdit:
		m.renderProjectEditView(&b)
	case m.view == viewViews:
		m.renderViewList(&b)
	case m.view == viewViewResults:
		m.renderViewResults(&b)
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
	// Editing field input is rendered inline in renderTaskDetailView.

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
		// Only show state when it's noteworthy — active and next-action are the
		// normal/default states and don't need to be displayed.
		if p.State != model.StateEmpty && p.State != model.StateActive && p.State != model.StateNextAction {
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

// ── Task detail view ────────────────────────────────────────────────────────

// fieldLabel returns a display label for each editable field.
func fieldLabel(f detailField) string {
	switch f {
	case fieldText:
		return "Task"
	case fieldState:
		return "State"
	case fieldTags:
		return "Tags"
	case fieldDeadline:
		return "Deadline"
	case fieldScheduled:
		return "Scheduled"
	case fieldURL:
		return "URL"
	case fieldWaitingOn:
		return "Waiting on"
	case fieldNotes:
		return "Notes"
	}
	return ""
}

// fieldValue returns the current display value for a field from detailTask.
func (m Model) fieldValue(f detailField) string {
	switch f {
	case fieldText:
		return m.detailTask.Text
	case fieldState:
		if m.detailTask.State == model.StateEmpty {
			return "(unprocessed)"
		}
		return string(m.detailTask.State)
	case fieldTags:
		if len(m.detailTask.Tags) == 0 {
			return ""
		}
		return strings.Join(m.detailTask.Tags, ", ")
	case fieldDeadline:
		return formatOptionalTime(m.detailTask.Deadline)
	case fieldScheduled:
		return formatOptionalTime(m.detailTask.Scheduled)
	case fieldURL:
		return m.detailTask.URL
	case fieldWaitingOn:
		return m.detailTask.WaitingOn
	case fieldNotes:
		return m.detailTask.Notes
	}
	return ""
}

// renderTaskDetailView renders the task detail / edit view.
func (m Model) renderTaskDetailView(b *strings.Builder) {
	b.WriteString(projectTitleStyle.Render("Task Detail"))
	b.WriteString("\n\n")

	for _, f := range detailFieldOrder {
		isSelected := f == m.detailField
		isEditing := isSelected && m.mode == modeEditingField

		label := fieldLabel(f)
		value := m.fieldValue(f)

		// Cursor.
		if isSelected {
			b.WriteString(cursorStyle.Render(" > "))
		} else {
			b.WriteString("   ")
		}

		// Label.
		// Pre-compute the nudge condition so the label can also be styled.
		isDateField := f == fieldDeadline || f == fieldScheduled
		isWaitingOnNudge := f == fieldWaitingOn &&
			m.detailTask.State == model.StateWaitingFor &&
			m.fieldValue(fieldWaitingOn) == "" && !isEditing
		if isWaitingOnNudge {
			b.WriteString(waitingNudgeStyle.Render(fmt.Sprintf("%-14s", label+":")))
		} else if isSelected {
			b.WriteString(selectedTaskStyle.Render(fmt.Sprintf("%-14s", label+":")))
		} else {
			b.WriteString(stateStyle.Render(fmt.Sprintf("%-14s", label+":")))
		}
		b.WriteString(" ")

		// Value or input.
		isWaitingOnNudge = f == fieldWaitingOn &&
			m.detailTask.State == model.StateWaitingFor &&
			value == "" && !isEditing
		if isEditing {
			b.WriteString(m.input.View())
		} else if isWaitingOnNudge {
			// Nudge: waiting-for but waiting_on not yet set.
			b.WriteString(waitingNudgeStyle.Render("— who or what are you waiting on?"))
		} else if value == "" {
			b.WriteString(helpStyle.Render("—"))
			if isSelected && isDateField {
				b.WriteString(helpStyle.Render("  (enter: open calendar)"))
			}
		} else if f == fieldState {
			b.WriteString(stateStyle.Render(value))
		} else if isDateField {
			b.WriteString(deadlineStyle.Render(value))
			if isSelected {
				b.WriteString(helpStyle.Render("  (enter: open calendar)"))
			}
		} else if f == fieldTags {
			b.WriteString(tagStyle.Render(value))
		} else {
			b.WriteString(value)
		}
		b.WriteString("\n")
	}

	// Separator before read-only metadata.
	b.WriteString("\n")
	b.WriteString(stateStyle.Render("  ─── read-only ───────────────────────────────"))
	b.WriteString("\n")

	// Created.
	b.WriteString("   ")
	b.WriteString(stateStyle.Render(fmt.Sprintf("%-14s", "Created:")))
	b.WriteString(" ")
	b.WriteString(stateStyle.Render(m.detailTask.Created.Format("2006-01-02 15:04")))
	b.WriteString("\n")

	// Waiting since (only when relevant).
	if m.detailTask.WaitingSince != nil {
		b.WriteString("   ")
		b.WriteString(stateStyle.Render(fmt.Sprintf("%-14s", "Waiting since:")))
		b.WriteString(" ")
		b.WriteString(stateStyle.Render(m.detailTask.WaitingSince.Format("2006-01-02")))
		b.WriteString("\n")
	}

	// ID.
	b.WriteString("   ")
	b.WriteString(stateStyle.Render(fmt.Sprintf("%-14s", "ID:")))
	b.WriteString(" ")
	b.WriteString(stateStyle.Render(m.detailTask.ID))
	b.WriteString("\n")

	// Source (only when set).
	if m.detailTask.Source != "" {
		b.WriteString("   ")
		b.WriteString(stateStyle.Render(fmt.Sprintf("%-14s", "Source:")))
		b.WriteString(" ")
		b.WriteString(stateStyle.Render(m.detailTask.Source))
		b.WriteString("\n")
	}
}

// renderDatePicker renders the date picker overlay.
func (m Model) renderDatePicker(b *strings.Builder) {
	fieldName := fieldLabel(m.datePickerField)
	b.WriteString(projectTitleStyle.Render(fmt.Sprintf("Pick date: %s", fieldName)))
	b.WriteString("\n\n")
	b.WriteString(m.datePicker.View())
}

// updatePickingDate handles messages while the date picker is open.
func (m Model) updatePickingDate(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.datePicker, cmd = m.datePicker.Update(msg)

	// Check for a result.
	if t, hasTime, confirmed, cancelled := m.datePicker.Result(); confirmed {
		// When hasTime is false, strip the time component so it stores as date-only (midnight).
		// This lets callers distinguish "user picked a date" from "user picked a datetime".
		picked := t
		if !hasTime {
			picked = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		}
		if m.view == viewProcessInbox {
			// Write back into the process inbox working copy.
			switch m.datePickerField {
			case fieldDeadline:
				m.processTask.Deadline = &picked
			case fieldScheduled:
				m.processTask.Scheduled = &picked
			}
		} else if m.view == viewProjectEdit {
			// Write back into the project edit working copy.
			// Only deadline is supported for projects.
			m.projEditProject.Deadline = &picked
		} else {
			// Write the picked time back into the task detail working copy.
			switch m.datePickerField {
			case fieldDeadline:
				m.detailTask.Deadline = &picked
			case fieldScheduled:
				m.detailTask.Scheduled = &picked
			}
		}
		m.mode = modeNormal
		return m, nil
	} else if cancelled {
		m.mode = modeNormal
		return m, nil
	}

	return m, cmd
}

// updateTaskDetail handles keys in the task detail view (normal mode within the view).
func (m Model) updateTaskDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Discard changes and return to originating view.
		m.view = m.detailFromView
		if m.detailFromView == viewList {
			m.currentList = m.detailFromList
		}
		if m.detailFromView == viewViewResults {
			// Re-run the query so the list reflects any ad-hoc changes
			// the user may have made before discarding.
			return m, m.runQuery(m.activeViewName, m.activeViewQuery)
		}
		return m, nil

	case "j", "down":
		m.detailField = nextDetailField(m.detailField, 1)

	case "k", "up":
		m.detailField = nextDetailField(m.detailField, -1)

	case "g":
		m.detailField = detailFieldOrder[0]

	case "G":
		m.detailField = detailFieldOrder[len(detailFieldOrder)-1]

	case "e", "enter":
		// Enter edit mode for the selected field.
		// State uses space/enter to cycle instead of text input.
		if m.detailField == fieldState {
			m.detailTask.State = cycleState(m.detailTask.State)
			return m, nil
		}
		// Deadline and scheduled use the calendar date picker.
		if m.detailField == fieldDeadline || m.detailField == fieldScheduled {
			m.mode = modePickingDate
			m.datePickerField = m.detailField
			var initial time.Time
			if m.detailField == fieldDeadline && m.detailTask.Deadline != nil {
				initial = *m.detailTask.Deadline
			} else if m.detailField == fieldScheduled && m.detailTask.Scheduled != nil {
				initial = *m.detailTask.Scheduled
			}
			cmd := m.datePicker.Open(initial)
			return m, cmd
		}
		m.mode = modeEditingField
		m.input.Reset()
		m.input.Placeholder = fieldLabel(m.detailField)
		m.input.SetValue(m.fieldValue(m.detailField))
		cmd := m.input.Focus()
		return m, cmd

	case "space":
		// Cycle state with space too (convenient shortcut on state field).
		if m.detailField == fieldState {
			m.detailTask.State = cycleState(m.detailTask.State)
		}

	case "s":
		// Save and return.
		return m, m.saveDetailTask()
	}

	return m, nil
}

// nextDetailField returns the field that is delta positions away from current
// in detailFieldOrder, clamped to the ends.
func nextDetailField(current detailField, delta int) detailField {
	for i, f := range detailFieldOrder {
		if f == current {
			next := i + delta
			if next < 0 {
				next = 0
			} else if next >= len(detailFieldOrder) {
				next = len(detailFieldOrder) - 1
			}
			return detailFieldOrder[next]
		}
	}
	return detailFieldOrder[0]
}

// cycleState advances through task states in order.
func cycleState(s model.TaskState) model.TaskState {
	states := []model.TaskState{
		model.StateEmpty,
		model.StateNextAction,
		model.StateWaitingFor,
		model.StateSomeday,
		model.StateDone,
		model.StateCanceled,
	}
	for i, st := range states {
		if st == s {
			return states[(i+1)%len(states)]
		}
	}
	return model.StateNextAction
}

// updateEditingField handles keys when editing a field value.
func (m Model) updateEditingField(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if m.view == viewProcessInbox {
			// Apply the edit to the process working copy and return to enrich step.
			m.applyProcessFieldEdit(val)
			m.mode = modeNormal
			return m, nil
		}
		if m.view == viewProjectEdit {
			m.applyProjEditFieldEdit(val)
			m.mode = modeNormal
			return m, nil
		}
		m.applyFieldEdit(val)
		m.mode = modeNormal
		return m, nil

	case "esc":
		m.mode = modeNormal
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// applyFieldEdit writes the edited string value back into the detailTask.
func (m *Model) applyFieldEdit(val string) {
	switch m.detailField {
	case fieldText:
		if val != "" {
			m.detailTask.Text = val
		}
	case fieldTags:
		if val == "" {
			m.detailTask.Tags = nil
		} else {
			parts := strings.Split(val, ",")
			tags := make([]string, 0, len(parts))
			for _, p := range parts {
				if t := strings.TrimSpace(p); t != "" {
					tags = append(tags, t)
				}
			}
			m.detailTask.Tags = tags
		}
	case fieldDeadline:
		if val == "" {
			m.detailTask.Deadline = nil
		} else {
			t := parseDateTime(val)
			if t != nil {
				m.detailTask.Deadline = t
			}
		}
	case fieldScheduled:
		if val == "" {
			m.detailTask.Scheduled = nil
		} else {
			t := parseDateTime(val)
			if t != nil {
				m.detailTask.Scheduled = t
			}
		}
	case fieldURL:
		m.detailTask.URL = val
	case fieldWaitingOn:
		m.detailTask.WaitingOn = val
		if val != "" && m.detailTask.State != model.StateWaitingFor {
			m.detailTask.State = model.StateWaitingFor
		}
	case fieldNotes:
		m.detailTask.Notes = val
	}
}

// applyProcessFieldEdit writes an edited value back into processTask (the process inbox working copy).
func (m *Model) applyProcessFieldEdit(val string) {
	switch m.detailField {
	case fieldText:
		if val != "" {
			m.processTask.Text = val
		}
	case fieldNotes:
		m.processTask.Notes = val
	case fieldWaitingOn:
		m.processTask.WaitingOn = val
	}
}

// parseDateTime attempts to parse a date/time string in several common formats.
func parseDateTime(s string) *time.Time {
	formats := []string{
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return &t
		}
	}
	return nil
}

// saveDetailTask writes the working copy back to disk and returns to the prior view.
func (m Model) saveDetailTask() tea.Cmd {
	task := m.detailTask
	isProject := m.detailIsProject
	filename := m.activeFilename
	sgIdx := m.detailFromSgIdx
	listType := m.detailFromList
	fromView := m.detailFromView
	viewName := m.activeViewName
	viewQueryStr := m.activeViewQuery
	return func() tea.Msg {
		var err error
		if isProject {
			err = m.svc.UpdateProjectTask(filename, sgIdx, task)
		} else {
			err = m.svc.UpdateTask(listType, task)
		}
		if err != nil {
			return errMsg{err}
		}
		// If we came from a view results screen, refresh the view after saving.
		if fromView == viewViewResults {
			clauses, _ := query.Parse(viewQueryStr, time.Now())
			all, err2 := m.svc.CollectAllTasks()
			if err2 != nil {
				return taskUpdatedMsg{task.Text}
			}
			var filtered []service.ViewTask
			for _, v := range all {
				if query.MatchAll(clauses, v.Task, v.Source) {
					filtered = append(filtered, v)
				}
			}
			return viewResultsLoadedMsg{name: viewName, queryStr: viewQueryStr, results: filtered}
		}
		return taskUpdatedMsg{task.Text}
	}
}

// ── Process Inbox Rendering ──────────────────────────────────────────────────

// renderProcessInbox renders the process inbox view, dispatching by step.
// When a shared mode overlay is active (modeEditingField, modePickingDate,
// modePickingProject) those renderers take priority via the View() switch, so
// this function only runs in modeNormal.
func (m Model) renderProcessInbox(b *strings.Builder) {
	total := len(m.processItems)

	// ── Completion screen ───────────────────────────────────────────────────
	if m.processStep == stepComplete {
		b.WriteString(projectTitleStyle.Render("Inbox Processed!"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("  %d items reviewed\n\n", total))
		type row struct {
			label string
			n     int
		}
		rows := []row{
			{"Trashed", m.processStats.trashed},
			{"Someday/Maybe", m.processStats.someday},
			{"Done (<2 min)", m.processStats.done},
			{"Waiting For", m.processStats.waiting},
			{"Single Actions", m.processStats.refiled},
			{"To Projects", m.processStats.toProject},
			{"Skipped", m.processStats.skipped},
		}
		for _, r := range rows {
			b.WriteString(stateStyle.Render(fmt.Sprintf("  %-16s %d\n", r.label+":", r.n)))
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  Press any key to return to inbox"))
		b.WriteString("\n")
		return
	}

	// ── Progress header ─────────────────────────────────────────────────────
	current := m.processIdx + 1
	header := fmt.Sprintf("Process Inbox  (%d of %d)", current, total)
	b.WriteString(projectTitleStyle.Render(header))
	b.WriteString("\n\n")

	// ── Task preview (shown in actionable, not-actionable, and route steps) ──
	// The enrich step renders its own full field list with inline input support.
	showTask := m.processStep == stepActionable ||
		m.processStep == stepNotActionable ||
		m.processStep == stepRoute
	if showTask {
		m.renderProcessTaskPreview(b)
		b.WriteString("\n")
	}

	// ── Step-specific prompt ────────────────────────────────────────────────
	switch m.processStep {
	case stepActionable:
		b.WriteString(inputPromptStyle.Render("  Is it actionable?"))
		b.WriteString("\n\n")
		b.WriteString("  ")
		b.WriteString(selectedTaskStyle.Render("[y]"))
		b.WriteString(" yes    ")
		b.WriteString(selectedTaskStyle.Render("[n]"))
		b.WriteString(" no    ")
		b.WriteString(selectedTaskStyle.Render("[s]"))
		b.WriteString(" skip    ")
		b.WriteString(stateStyle.Render("[q] quit"))
		b.WriteString("\n")

	case stepNotActionable:
		b.WriteString(inputPromptStyle.Render("  Not actionable — what to do?"))
		b.WriteString("\n\n")
		b.WriteString("  ")
		b.WriteString(selectedTaskStyle.Render("[t]"))
		b.WriteString(" trash    ")
		b.WriteString(stateStyle.Render("[esc] back"))
		b.WriteString("\n")

	case stepEnrich:
		b.WriteString(inputPromptStyle.Render("  Enrich before routing"))
		b.WriteString("\n\n")
		m.renderProcessEnrichFields(b)
		b.WriteString("\n  ")
		b.WriteString(selectedTaskStyle.Render("[t]"))
		b.WriteString(" text  ")
		b.WriteString(selectedTaskStyle.Render("[g]"))
		b.WriteString(" tags  ")
		b.WriteString(selectedTaskStyle.Render("[d]"))
		b.WriteString(" deadline  ")
		b.WriteString(selectedTaskStyle.Render("[c]"))
		b.WriteString(" schedule  ")
		b.WriteString(selectedTaskStyle.Render("[n]"))
		b.WriteString(" notes")
		b.WriteString("\n  ")
		b.WriteString(selectedTaskStyle.Render("[enter]"))
		b.WriteString(" continue    ")
		b.WriteString(stateStyle.Render("[esc] back"))
		b.WriteString("\n")

	case stepEnrichTags:
		b.WriteString(inputPromptStyle.Render("  Add tags (tab to confirm each, enter when done)"))
		b.WriteString("\n\n")
		// Show accumulated tags so far.
		if len(m.processTags) > 0 {
			b.WriteString(stateStyle.Render("  Tags so far: "))
			for i, tag := range m.processTags {
				if i > 0 {
					b.WriteString(stateStyle.Render(", "))
				}
				b.WriteString(tagStyle.Render(tag))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n  ")
		b.WriteString(stateStyle.Render("Add tag: "))
		b.WriteString(m.input.View())
		b.WriteString("\n")

	case stepRoute:
		b.WriteString(inputPromptStyle.Render("  Route this task:"))
		b.WriteString("\n\n")
		actions := []struct{ key, desc string }{
			{"d", "done (<2 min, did it)"},
			{"w", "waiting for (delegate)"},
			{"s", "someday/maybe"},
			{"r", "single actions (next action)"},
			{"p", "add to project"},
			{"n", "new project"},
		}
		for _, a := range actions {
			b.WriteString("  ")
			b.WriteString(selectedTaskStyle.Render("[" + a.key + "]"))
			b.WriteString(" " + a.desc + "\n")
		}
		b.WriteString("  ")
		b.WriteString(stateStyle.Render("[esc] back"))
		b.WriteString("\n")

	case stepWaitingOn:
		b.WriteString(inputPromptStyle.Render("  Waiting on:"))
		b.WriteString("\n\n  ")
		b.WriteString(m.input.View())
		b.WriteString("\n")

	case stepNewProject:
		b.WriteString(inputPromptStyle.Render("  New project title:"))
		b.WriteString("\n\n  ")
		b.WriteString(m.input.View())
		b.WriteString("\n")
	}
}

// renderProcessEnrichFields renders the enrichable fields for the current process task,
// showing the text input inline for whichever field is being edited.
func (m Model) renderProcessEnrichFields(b *strings.Builder) {
	task := m.processTask
	isEditing := m.mode == modeEditingField

	type enrichField struct {
		label string
		field detailField
		value string
	}
	fields := []enrichField{
		{"Text", fieldText, task.Text},
		{"Tags", fieldTags, strings.Join(task.Tags, ", ")},
		{"Scheduled", fieldScheduled, formatOptionalTime(task.Scheduled)},
		{"Deadline", fieldDeadline, formatOptionalTime(task.Deadline)},
		{"Notes", fieldNotes, task.Notes},
	}

	for _, f := range fields {
		editing := isEditing && m.detailField == f.field
		b.WriteString("  ")
		b.WriteString(stateStyle.Render(fmt.Sprintf("%-10s", f.label+":")))
		b.WriteString(" ")
		if editing {
			b.WriteString(m.input.View())
		} else if f.value == "" {
			b.WriteString(helpStyle.Render("—"))
		} else if f.field == fieldTags {
			b.WriteString(tagStyle.Render(f.value))
		} else if f.field == fieldDeadline || f.field == fieldScheduled {
			b.WriteString(deadlineStyle.Render(f.value))
		} else {
			// Text and notes: truncate long values for display.
			display := f.value
			if len(display) > 60 {
				display = display[:57] + "..."
			}
			b.WriteString(display)
		}
		b.WriteString("\n")
	}
}

// formatOptionalTime formats a *time.Time for display, omitting time when hasTime is false.
// When t is nil, returns "".
func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	if t.Hour() == 0 && t.Minute() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04")
}

// renderProcessTaskPreview renders a compact read-only summary of the current
// process task. Used in the actionable, not-actionable, and route steps.
// The enrich step uses renderProcessEnrichFields instead.
func (m Model) renderProcessTaskPreview(b *strings.Builder) {
	task := m.processTask

	// Task text.
	b.WriteString(selectedTaskStyle.Render("  "+task.Text) + "\n")

	// Tags.
	if len(task.Tags) > 0 {
		b.WriteString(stateStyle.Render("  Tags:      "))
		for i, tag := range task.Tags {
			if i > 0 {
				b.WriteString(stateStyle.Render(", "))
			}
			b.WriteString(tagStyle.Render(tag))
		}
		b.WriteString("\n")
	}

	// Scheduled (before deadline).
	if s := formatOptionalTime(task.Scheduled); s != "" {
		b.WriteString(stateStyle.Render("  Scheduled: "))
		b.WriteString(deadlineStyle.Render(s))
		b.WriteString("\n")
	}

	// Deadline.
	if d := formatOptionalTime(task.Deadline); d != "" {
		b.WriteString(stateStyle.Render("  Deadline:  "))
		b.WriteString(deadlineStyle.Render(d))
		b.WriteString("\n")
	}

	// Notes (first line only, truncated).
	if task.Notes != "" {
		preview := task.Notes
		if idx := strings.IndexByte(preview, '\n'); idx >= 0 {
			preview = preview[:idx]
		}
		if len(preview) > 60 {
			preview = preview[:57] + "..."
		}
		b.WriteString(stateStyle.Render("  Notes:     "))
		b.WriteString(helpStyle.Render(preview))
		b.WriteString("\n")
	}
}

// renderTabBar renders the list switcher tabs.
func (m Model) renderTabBar() string {
	inboxLabel := " 1 Inbox "
	actionsLabel := " 2 Actions "
	projectsLabel := " 3 Projects "
	viewsLabel := " 4 Views "

	var tabs [4]string

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

	if m.view == viewProjects || m.view == viewProjectDetail || m.view == viewProjectEdit {
		count := len(m.projects)
		tabs[2] = activeTabStyle.Render(fmt.Sprintf("%s(%d)", projectsLabel, count))
	} else {
		tabs[2] = inactiveTabStyle.Render(projectsLabel)
	}

	if m.view == viewViews || m.view == viewViewResults {
		tabs[3] = activeTabStyle.Render(viewsLabel)
	} else {
		tabs[3] = inactiveTabStyle.Render(viewsLabel)
	}

	// In process inbox mode, highlight the inbox tab with a special label.
	if m.view == viewProcessInbox {
		label := fmt.Sprintf(" Processing Inbox (%d of %d) ", m.processIdx+1, len(m.processItems))
		if m.processStep == stepComplete {
			label = " Inbox Processed! "
		}
		tabs[0] = activeTabStyle.Render(label)
		tabs[1] = inactiveTabStyle.Render(actionsLabel)
		tabs[2] = inactiveTabStyle.Render(projectsLabel)
		tabs[3] = inactiveTabStyle.Render(viewsLabel)
	}

	return tabs[0] + "  " + tabs[1] + "  " + tabs[2] + "  " + tabs[3]
}

// helpText returns contextual help based on mode and current view.
func (m Model) helpText() string {
	if m.mode == modeAdding || m.mode == modeAddingProject || m.mode == modeAddingSubGroup || m.mode == modeAddingProjectTask {
		return "enter: save  esc: cancel"
	}
	if m.mode == modePickingProject || m.mode == modePickingSubGroup {
		return "enter: select  esc: cancel  j/k: navigate"
	}
	if m.mode == modePickingDate {
		return "arrows/hjkl: move day  </> or [/]: month  t: toggle time  enter: confirm  esc: cancel"
	}
	if m.view == viewTaskDetail {
		if m.mode == modeEditingField {
			return "enter: save field  esc: cancel edit"
		}
		return "j/k: navigate fields  e/enter: edit  space: cycle state  s: save & back  esc: back (discard)"
	}
	if m.view == viewProcessInbox {
		if m.mode == modeEditingField {
			return "enter: save  esc: cancel"
		}
		switch m.processStep {
		case stepActionable:
			return "y: actionable  n: not actionable  s: skip  q: quit"
		case stepNotActionable:
			return "t: trash  esc: back"
		case stepEnrich:
			return "t: text  g: tags  d: deadline  c: schedule  n: notes  enter: continue  esc: back"
		case stepEnrichTags:
			return "tab: add tag  enter: done  esc: cancel"
		case stepRoute:
			return "d: done  w: waiting  s: someday/maybe  r: single actions  p: project  n: new project  esc: back"
		case stepWaitingOn:
			return "enter: confirm  esc: back"
		case stepNewProject:
			return "enter: create & refile  esc: back"
		case stepComplete:
			return "any key: return to inbox"
		}
	}

	if m.view == viewProjectEdit {
		if m.mode == modeEditingField {
			return "enter: save field  esc: cancel edit"
		}
		return "j/k: navigate fields  e/enter: edit  space: cycle state  s: save & back  esc: back (discard)"
	}

	if m.view == viewViews {
		if m.mode == modeEditingField {
			return "enter: run query  esc: cancel"
		}
		return "enter: open view  /: ad-hoc query  j/k: navigate  1-4/tab: switch tab  q: quit"
	}
	if m.view == viewViewResults {
		return "enter: task detail  d: done  s: someday  w: waiting  x: trash  R: refresh  esc: back  j/k: navigate"
	}

	nav := "j/k: navigate  tab: switch list  q: quit"

	switch m.view {
	case viewProjects:
		return "enter: open  a: new project  E: edit project  " + nav
	case viewProjectDetail:
		return "enter: detail  a: add task  n: new sub-group  d: done  E: edit project  C-j/C-k: reorder  m: move to sub-group  esc: back  " + nav
	default:
		if m.currentList == model.ListIn {
			return "enter: detail  a: add  P: process inbox  r: refile  p: to project  s: someday  w: waiting  d: done  x: trash  " + nav
		}
		return "enter: detail  p: to project  s: someday  w: waiting  d: done  x: trash  " + nav
	}
}

// ── Project Edit View ────────────────────────────────────────────────────────

// openProjectEdit loads a project from disk into the working copy and switches to viewProjectEdit.
func (m Model) openProjectEdit(filename string, fromView viewState) tea.Cmd {
	return func() tea.Msg {
		proj, err := m.svc.GetProject(filename)
		if err != nil {
			return errMsg{err}
		}
		return projectEditLoadedMsg{project: *proj, filename: filename, fromView: fromView}
	}
}

// projectEditLoadedMsg carries the project data when opening the project edit view.
type projectEditLoadedMsg struct {
	project  model.Project
	filename string
	fromView viewState
}

// projEditLabel returns a display label for a project edit field.
func projEditLabel(f projEditField) string {
	switch f {
	case projFieldTitle:
		return "Title"
	case projFieldState:
		return "State"
	case projFieldTags:
		return "Tags"
	case projFieldDeadline:
		return "Deadline"
	case projFieldURL:
		return "URL"
	case projFieldWaitingOn:
		return "Waiting on"
	case projFieldDefinitionOfDone:
		return "Done when"
	}
	return ""
}

// projEditFieldValue returns the current display value of a project edit field.
func (m Model) projEditFieldValue(f projEditField) string {
	switch f {
	case projFieldTitle:
		return m.projEditProject.Title
	case projFieldState:
		switch m.projEditProject.State {
		case model.StateEmpty:
			return "active" // treat empty as active (legacy files)
		case model.StateNextAction:
			return "active (legacy: next-action)" // surface legacy state clearly
		default:
			return string(m.projEditProject.State)
		}
	case projFieldTags:
		return strings.Join(m.projEditProject.Tags, ", ")
	case projFieldDeadline:
		return formatOptionalTime(m.projEditProject.Deadline)
	case projFieldURL:
		return m.projEditProject.URL
	case projFieldWaitingOn:
		return m.projEditProject.WaitingOn
	case projFieldDefinitionOfDone:
		return m.projEditProject.DefinitionOfDone
	}
	return ""
}

// nextProjEditField returns the field delta positions away from current, clamped.
func nextProjEditField(current projEditField, delta int) projEditField {
	for i, f := range projEditFieldOrder {
		if f == current {
			next := i + delta
			if next < 0 {
				next = 0
			} else if next >= len(projEditFieldOrder) {
				next = len(projEditFieldOrder) - 1
			}
			return projEditFieldOrder[next]
		}
	}
	return projEditFieldOrder[0]
}

// updateProjectEdit handles keys in the project edit view.
func (m Model) updateProjectEdit(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Discard — return to the originating view.
		m.view = m.projEditFromView
		return m, nil

	case "j", "down":
		m.projEditField = nextProjEditField(m.projEditField, 1)

	case "k", "up":
		m.projEditField = nextProjEditField(m.projEditField, -1)

	case "g":
		m.projEditField = projEditFieldOrder[0]

	case "G":
		m.projEditField = projEditFieldOrder[len(projEditFieldOrder)-1]

	case "e", "enter":
		if m.projEditField == projFieldState {
			m.projEditProject.State = cycleProjectState(m.projEditProject.State)
			return m, nil
		}
		if m.projEditField == projFieldDeadline {
			m.mode = modePickingDate
			m.datePickerField = fieldDeadline // reuse task detailField enum for date picker routing
			var initial time.Time
			if m.projEditProject.Deadline != nil {
				initial = *m.projEditProject.Deadline
			}
			cmd := m.datePicker.Open(initial)
			return m, cmd
		}
		// Text input for all other fields.
		m.mode = modeEditingField
		m.input.Reset()
		m.input.Placeholder = projEditLabel(m.projEditField)
		m.input.SetValue(m.projEditFieldValue(m.projEditField))
		cmd := m.input.Focus()
		return m, cmd

	case "space":
		if m.projEditField == projFieldState {
			m.projEditProject.State = cycleProjectState(m.projEditProject.State)
		}

	case "s":
		return m, m.saveProjectEdit()
	}

	return m, nil
}

// cycleProjectState cycles through project-relevant states.
// Projects use StateActive (not StateNextAction) as their "being worked on" state.
// StateWaitingFor is included because a project can be blocked on an external dependency.
// StateEmpty and StateNextAction are task-only concepts and are excluded.
func cycleProjectState(s model.TaskState) model.TaskState {
	states := []model.TaskState{
		model.StateActive,
		model.StateWaitingFor,
		model.StateSomeday,
		model.StateDone,
		model.StateCanceled,
	}
	for i, st := range states {
		if st == s {
			return states[(i+1)%len(states)]
		}
	}
	return model.StateActive
}

// applyProjEditFieldEdit applies a text input value to the project working copy.
func (m *Model) applyProjEditFieldEdit(val string) {
	switch m.projEditField {
	case projFieldTitle:
		if val != "" {
			m.projEditProject.Title = val
		}
	case projFieldTags:
		if val == "" {
			m.projEditProject.Tags = nil
		} else {
			parts := strings.Split(val, ",")
			tags := make([]string, 0, len(parts))
			for _, p := range parts {
				if t := strings.TrimSpace(p); t != "" {
					tags = append(tags, t)
				}
			}
			m.projEditProject.Tags = tags
		}
	case projFieldURL:
		m.projEditProject.URL = val
	case projFieldWaitingOn:
		m.projEditProject.WaitingOn = val
	case projFieldDefinitionOfDone:
		m.projEditProject.DefinitionOfDone = val
	}
}

// saveProjectEdit writes the working copy to disk via UpdateProject.
func (m Model) saveProjectEdit() tea.Cmd {
	proj := m.projEditProject
	oldFilename := m.projEditFilename
	return func() tea.Msg {
		newFilename, err := m.svc.UpdateProject(oldFilename, proj)
		if err != nil {
			return errMsg{err}
		}
		return projectUpdatedMsg{title: proj.Title, newFilename: newFilename}
	}
}

// renderProjectEditView renders the project metadata edit form.
func (m Model) renderProjectEditView(b *strings.Builder) {
	b.WriteString(projectTitleStyle.Render("Edit Project"))
	b.WriteString("\n\n")

	for _, f := range projEditFieldOrder {
		isSelected := f == m.projEditField
		isEditing := isSelected && m.mode == modeEditingField

		label := projEditLabel(f)
		value := m.projEditFieldValue(f)

		isDateField := f == projFieldDeadline
		isWaitingOnNudge := f == projFieldWaitingOn &&
			m.projEditProject.State == model.StateWaitingFor &&
			value == "" && !isEditing

		if isSelected {
			b.WriteString(cursorStyle.Render(" > "))
		} else {
			b.WriteString("   ")
		}

		if isWaitingOnNudge {
			b.WriteString(waitingNudgeStyle.Render(fmt.Sprintf("%-14s", label+":")))
		} else if isSelected {
			b.WriteString(selectedTaskStyle.Render(fmt.Sprintf("%-14s", label+":")))
		} else {
			b.WriteString(stateStyle.Render(fmt.Sprintf("%-14s", label+":")))
		}
		b.WriteString(" ")

		if isEditing {
			b.WriteString(m.input.View())
		} else if isWaitingOnNudge {
			b.WriteString(waitingNudgeStyle.Render("— who or what are you waiting on?"))
		} else if value == "" {
			b.WriteString(helpStyle.Render("—"))
			if isSelected && isDateField {
				b.WriteString(helpStyle.Render("  (enter: open calendar)"))
			}
		} else if f == projFieldState {
			b.WriteString(stateStyle.Render(value))
		} else if isDateField {
			b.WriteString(deadlineStyle.Render(value))
			if isSelected {
				b.WriteString(helpStyle.Render("  (enter: open calendar)"))
			}
		} else if f == projFieldTags {
			b.WriteString(tagStyle.Render(value))
		} else {
			b.WriteString(value)
		}
		b.WriteString("\n")
	}

	// Read-only section.
	b.WriteString("\n")
	b.WriteString(stateStyle.Render("  ─── read-only ───────────────────────────────"))
	b.WriteString("\n")
	b.WriteString("   ")
	b.WriteString(stateStyle.Render(fmt.Sprintf("%-14s", "ID:")))
	b.WriteString(" ")
	b.WriteString(stateStyle.Render(m.projEditProject.ID))
	b.WriteString("\n")
	b.WriteString("   ")
	b.WriteString(stateStyle.Render(fmt.Sprintf("%-14s", "File:")))
	b.WriteString(" ")
	b.WriteString(stateStyle.Render(m.projEditFilename))
	b.WriteString("\n")
}

// ── Views ─────────────────────────────────────────────────────────────────

// runQuery collects all tasks and filters them using the given DSL query string.
// Returns a viewResultsLoadedMsg — intended to be returned as a tea.Cmd.
func (m Model) runQuery(name, queryStr string) tea.Cmd {
	return func() tea.Msg {
		clauses, err := query.Parse(queryStr, time.Now())
		if err != nil {
			return viewResultsLoadedMsg{name: name, queryStr: queryStr, err: err}
		}
		all, err := m.svc.CollectAllTasks()
		if err != nil {
			return viewResultsLoadedMsg{name: name, queryStr: queryStr, err: err}
		}
		var filtered []service.ViewTask
		for _, vt := range all {
			if query.MatchAll(clauses, vt.Task, vt.Source) {
				filtered = append(filtered, vt)
			}
		}
		return viewResultsLoadedMsg{name: name, queryStr: queryStr, results: filtered}
	}
}

// updateViewList handles keys in the saved view list screen.
func (m Model) updateViewList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.view = viewList
		m.currentList = model.ListIn
		m.cursor = 0
		return m, m.loadCurrentList

	case "j", "down":
		if m.viewListCursor < len(m.savedViews)-1 {
			m.viewListCursor++
		}

	case "k", "up":
		if m.viewListCursor > 0 {
			m.viewListCursor--
		}

	case "g":
		m.viewListCursor = 0

	case "G":
		if len(m.savedViews) > 0 {
			m.viewListCursor = len(m.savedViews) - 1
		}

	case "enter":
		if len(m.savedViews) > 0 {
			sv := m.savedViews[m.viewListCursor]
			return m, m.runQuery(sv.Name, sv.Query)
		}

	case "/":
		// Ad-hoc query: open inline text input.
		m.mode = modeEditingField
		m.input.Reset()
		m.input.Placeholder = "type a query, e.g. state:waiting-for tag:@home"
		m.input.SetValue("")
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
		m.view = viewProjects
		m.cursor = 0
		return m, m.loadProjects

	case "4", "V":
		// Already here.

	case "tab":
		m.view = viewList
		m.currentList = model.ListIn
		m.cursor = 0
		return m, m.loadCurrentList
	}

	// Ad-hoc query input: when modeEditingField is active in viewViews,
	// forward to the shared editing handler except for enter (run query).
	if m.mode == modeEditingField {
		switch msg.String() {
		case "enter":
			queryStr := strings.TrimSpace(m.input.Value())
			m.mode = modeNormal
			if queryStr == "" {
				return m, nil
			}
			return m, m.runQuery("Ad-hoc", queryStr)
		case "esc":
			m.mode = modeNormal
			return m, nil
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// updateViewResults handles keys in the view results screen.
func (m Model) updateViewResults(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.view = viewViews
		return m, nil

	case "j", "down":
		if m.viewCursor < len(m.viewResults)-1 {
			m.viewCursor++
		}

	case "k", "up":
		if m.viewCursor > 0 {
			m.viewCursor--
		}

	case "g":
		m.viewCursor = 0

	case "G":
		if len(m.viewResults) > 0 {
			m.viewCursor = len(m.viewResults) - 1
		}

	case "enter":
		if len(m.viewResults) == 0 {
			return m, nil
		}
		vt := m.viewResults[m.viewCursor]
		m.detailTask = vt.Task
		m.detailField = fieldText
		m.detailFromView = viewViewResults
		m.detailIsProject = vt.IsProject
		if vt.IsProject {
			m.activeFilename = vt.Filename
			m.detailFromSgIdx = vt.SgIdx
		} else {
			m.detailFromList = vt.ListType
		}
		m.view = viewTaskDetail
		return m, nil

	case "R":
		// Refresh: re-run the current query.
		return m, m.runQuery(m.activeViewName, m.activeViewQuery)

	case "d":
		return m, m.viewResultStateChange(model.StateDone)

	case "s":
		return m, m.viewResultStateChange(model.StateSomeday)

	case "w":
		return m, m.viewResultStateChange(model.StateWaitingFor)

	case "x":
		return m, m.viewResultTrash()

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

	case "4", "V":
		m.view = viewViews
		return m, nil

	case "tab":
		m.view = viewViews
		return m, nil
	}

	return m, nil
}

// viewResultStateChange applies a state change to the selected view result task
// and re-runs the query to refresh.
func (m Model) viewResultStateChange(newState model.TaskState) tea.Cmd {
	if len(m.viewResults) == 0 {
		return nil
	}
	vt := m.viewResults[m.viewCursor]
	name := m.activeViewName
	queryStr := m.activeViewQuery
	return func() tea.Msg {
		var err error
		if vt.IsProject {
			err = m.svc.UpdateProjectTaskState(vt.Filename, vt.SgIdx, vt.Task.ID, newState)
		} else {
			err = m.svc.UpdateState(vt.ListType, vt.Task.ID, newState)
		}
		if err != nil {
			return errMsg{err}
		}
		// Re-collect and re-filter.
		clauses, _ := query.Parse(queryStr, time.Now())
		all, err := m.svc.CollectAllTasks()
		if err != nil {
			return viewResultsLoadedMsg{name: name, queryStr: queryStr, err: err}
		}
		var filtered []service.ViewTask
		for _, v := range all {
			if query.MatchAll(clauses, v.Task, v.Source) {
				filtered = append(filtered, v)
			}
		}
		return viewResultsLoadedMsg{name: name, queryStr: queryStr, results: filtered}
	}
}

// viewResultTrash permanently removes the selected view result task.
func (m Model) viewResultTrash() tea.Cmd {
	if len(m.viewResults) == 0 {
		return nil
	}
	vt := m.viewResults[m.viewCursor]
	name := m.activeViewName
	queryStr := m.activeViewQuery
	return func() tea.Msg {
		var err error
		if vt.IsProject {
			// No TrashProjectTask method yet — use UpdateProjectTaskState to canceled.
			err = m.svc.UpdateProjectTaskState(vt.Filename, vt.SgIdx, vt.Task.ID, model.StateCanceled)
		} else {
			err = m.svc.TrashTask(vt.ListType, vt.Task.ID)
		}
		if err != nil {
			return errMsg{err}
		}
		clauses, _ := query.Parse(queryStr, time.Now())
		all, err2 := m.svc.CollectAllTasks()
		if err2 != nil {
			return viewResultsLoadedMsg{name: name, queryStr: queryStr, err: err2}
		}
		var filtered []service.ViewTask
		for _, v := range all {
			if query.MatchAll(clauses, v.Task, v.Source) {
				filtered = append(filtered, v)
			}
		}
		return viewResultsLoadedMsg{name: name, queryStr: queryStr, results: filtered}
	}
}

// sourceBadge returns a short muted badge string for the source of a view task.
func sourceBadge(source string) string {
	switch source {
	case "in":
		return "[inbox]"
	case "single-actions":
		return "[actions]"
	}
	// projects/launch-website.md → [launch-website]
	if after, ok := strings.CutPrefix(source, "projects/"); ok {
		name := strings.TrimSuffix(after, ".md")
		if len(name) > 18 {
			name = name[:15] + "..."
		}
		return "[" + name + "]"
	}
	return "[" + source + "]"
}

// renderViewList renders the saved view list screen.
func (m Model) renderViewList(b *strings.Builder) {
	b.WriteString(projectTitleStyle.Render("Views"))
	b.WriteString("\n\n")

	if len(m.savedViews) == 0 {
		b.WriteString(taskStyle.Render("No views defined."))
		b.WriteString("\n")
		return
	}

	for i, sv := range m.savedViews {
		isSelected := i == m.viewListCursor

		if isSelected {
			b.WriteString(cursorStyle.Render(" > "))
		} else {
			b.WriteString("   ")
		}

		if isSelected {
			b.WriteString(selectedTaskStyle.Render(sv.Name))
		} else {
			b.WriteString(sv.Name)
		}
		b.WriteString("  ")
		b.WriteString(helpStyle.Render(sv.Query))
		b.WriteString("\n")
	}

	// Ad-hoc query input when active.
	if m.mode == modeEditingField {
		b.WriteString("\n")
		b.WriteString(inputPromptStyle.Render("  Query: "))
		b.WriteString(m.input.View())
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  /: ad-hoc query"))
		b.WriteString("\n")
	}
}

// renderViewResults renders the filtered task list for an open view.
func (m Model) renderViewResults(b *strings.Builder) {
	// Header: view name + query.
	header := m.activeViewName
	b.WriteString(projectTitleStyle.Render(header))
	if m.activeViewQuery != "" {
		b.WriteString("  ")
		b.WriteString(helpStyle.Render(m.activeViewQuery))
	}
	b.WriteString("\n\n")

	if len(m.viewResults) == 0 {
		b.WriteString(taskStyle.Render("No tasks match this query."))
		b.WriteString("\n")
		return
	}

	for i, vt := range m.viewResults {
		isSelected := i == m.viewCursor
		task := vt.Task

		if isSelected {
			b.WriteString(cursorStyle.Render(" > "))
		} else {
			b.WriteString("   ")
		}

		// Checkbox + text.
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

		// Inline metadata: state, deadline, tags, source badge.
		var meta []string
		if task.State != model.StateEmpty && task.State != model.StateDone && task.State != model.StateCanceled {
			meta = append(meta, stateStyle.Render(string(task.State)))
		}
		if task.Deadline != nil {
			meta = append(meta, deadlineStyle.Render("due:"+task.Deadline.Format("2006-01-02")))
		}
		for _, tag := range task.Tags {
			meta = append(meta, tagStyle.Render(tag))
		}
		// Muted source badge.
		meta = append(meta, helpStyle.Render(sourceBadge(vt.Source)))

		if len(meta) > 0 {
			b.WriteString("  ")
			b.WriteString(strings.Join(meta, " "))
		}
		b.WriteString("\n")
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
	case viewViews:
		max = len(m.savedViews)
	case viewViewResults:
		max = len(m.viewResults)
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
