// Package tui implements the Bubbletea terminal user interface for wnwn.
package tui

import (
	"fmt"
	"math"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chipkoziara/wnwn/internal/config"
	"github.com/chipkoziara/wnwn/internal/core"
	"github.com/chipkoziara/wnwn/internal/model"
	"github.com/chipkoziara/wnwn/internal/query"
	"github.com/chipkoziara/wnwn/internal/search"
	"github.com/chipkoziara/wnwn/internal/service"
	"github.com/chipkoziara/wnwn/internal/store"
	"github.com/chipkoziara/wnwn/internal/tui/datepicker"
)

// mode represents the current UI mode.
type mode int

const (
	modeNormal mode = iota
	modeAdding
	modeAddingProject     // creating a new project
	modeAddingSubGroup    // adding a sub-group to current project
	modeRenamingSubGroup  // renaming a sub-group in current project
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
	viewWeeklyReview                   // guided weekly review workflow
)

type weeklyReviewStep int

const (
	weeklyStepProjects weeklyReviewStep = iota
	weeklyStepWaiting
	weeklyStepSomeday
	weeklyStepArchived
	weeklyStepCount
)

type keyPrefix int

const (
	prefixNone keyPrefix = iota
	prefixState
	prefixRoute
	prefixTime
)

type appTabKind int

const (
	tabInbox appTabKind = iota
	tabActions
	tabProjects
	tabViews
	tabSavedView
)

type appTab struct {
	Kind appTabKind
	View model.SavedView
}

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
	core        *core.Core
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
	refileTaskID    string
	refileTaskText  string
	refileFromList  model.ListType
	refilePrevState model.TaskState

	// Sub-group picker state: task being moved between sub-groups.
	moveTaskID    string
	moveTaskText  string
	moveFromSgIdx int
	renameSgIdx   int

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
	activeViewInclA bool               // whether the active view includes archived tasks
	activeViewFuzzy bool               // whether the active view is fuzzy-search based
	viewResults     []service.ViewTask // filtered tasks for the active view
	viewCursor      int                // cursor within view results
	tabOrder        []appTab
	scrollOffset    int                // top row offset for scrollable list-style views

	// Process inbox state.
	processSessionID string       // active core-owned process inbox session ID
	processItems     []model.Task // snapshot of inbox tasks taken at activation
	processIdx       int          // index of the current item being processed (0-based)
	processStep      processStep  // current step in the decision tree
	processTask      model.Task   // working copy of the current item (mutated during enrichment)
	processTags      []string     // tag accumulator for stepEnrichTags (tab-separated entry)
	processStats     processStats // running totals for the completion summary

	weeklyReviewData    service.WeeklyReviewData
	weeklyReviewStep    weeklyReviewStep
	weeklyReviewCursors [weeklyStepCount]int
	pendingPrefix       keyPrefix

	keybindings    map[string]map[string]string
	disabledAction map[string]map[string]bool

	undoEnabled   bool
	undoDuration  time.Duration
	undoKey       string
	undoSeq       int
	undoApply     func() error
	undoReload    tea.Cmd
	undoSuccess   string
	undoPrompt    string
	undoExpiresAt time.Time
}

// New creates a new TUI model backed by the given data directory.
func New(dataDir string) Model {
	cfg, err := config.Load(dataDir)
	if err != nil {
		cfg = config.Default()
	}
	return NewWithConfig(dataDir, cfg)
}

// NewWithConfig creates a new TUI model with an explicit config.
func NewWithConfig(dataDir string, cfg config.Config) Model {
	s := store.New(dataDir)
	var savedViewsCfg []model.SavedView
	for _, v := range cfg.Views.Saved {
		savedViewsCfg = append(savedViewsCfg, model.SavedView{Name: v.Name, Query: v.Query, IncludeArchived: v.IncludeArchived})
	}
	coreSvc := core.New(s, core.CoreConfig{
		AutoArchiveDone:     cfg.Archive.AutoArchiveDone,
		AutoArchiveCanceled: cfg.Archive.AutoArchiveCanceled,
		SavedViews:          savedViewsCfg,
	})
	svc := coreSvc.UnderlyingService()

	ti := textinput.New()
	ti.Placeholder = "What needs to be done?"
	ti.CharLimit = 256
	ti.SetWidth(60)
	ti.KeyMap.AcceptSuggestion = key.NewBinding()

	keybindings, disabled := mergeKeybindings(cfg)
	m := Model{
		svc:            svc,
		store:          s,
		core:           coreSvc,
		currentList:    model.ListIn,
		input:          ti,
		datePicker:     datepicker.New(),
		savedViews:     resolveSavedViews(cfg),
		keybindings:    keybindings,
		disabledAction: disabled,
		undoEnabled:    cfg.UI.UndoGraceEnabled,
		undoDuration:   time.Duration(cfg.UI.UndoGraceSeconds) * time.Second,
		undoKey:        cfg.UI.UndoKey,
	}
	m.tabOrder = resolveTabs(cfg, m.savedViews)

	switch strings.ToLower(cfg.UI.DefaultView) {
	case "actions", "single-actions":
		m.view = viewList
		m.currentList = model.ListSingleActions
	case "projects":
		m.view = viewProjects
	case "views":
		m.view = viewViews
	default:
		m.view = viewList
		m.currentList = model.ListIn
	}

	return m
}

func resolveSavedViews(cfg config.Config) []model.SavedView {
	views := make([]model.SavedView, 0, len(model.DefaultViews())+len(cfg.Views.Saved))
	if cfg.Views.UseDefaults {
		views = append(views, model.DefaultViews()...)
	}
	for _, v := range cfg.Views.Saved {
		views = append(views, model.SavedView{
			Name:            v.Name,
			Query:           v.Query,
			IncludeArchived: v.IncludeArchived,
		})
	}
	if len(views) == 0 {
		views = model.DefaultViews()
	}
	return views
}

func resolveTabs(cfg config.Config, savedViews []model.SavedView) []appTab {
	if len(cfg.UI.Tabs) == 0 {
		return []appTab{{Kind: tabInbox}, {Kind: tabActions}, {Kind: tabProjects}, {Kind: tabViews}}
	}
	order := make([]appTab, 0, len(cfg.UI.Tabs))
	for _, t := range cfg.UI.Tabs {
		switch t {
		case "inbox":
			order = append(order, appTab{Kind: tabInbox})
		case "actions":
			order = append(order, appTab{Kind: tabActions})
		case "projects":
			order = append(order, appTab{Kind: tabProjects})
		case "views":
			order = append(order, appTab{Kind: tabViews})
		default:
			if strings.HasPrefix(strings.ToLower(t), "view:") {
				parts := strings.SplitN(t, ":", 2)
				if len(parts) != 2 {
					continue
				}
				name := strings.TrimSpace(parts[1])
				if name == "" {
					continue
				}
				if sv, ok := findSavedViewByName(savedViews, name); ok {
					order = append(order, appTab{Kind: tabSavedView, View: sv})
				}
			}
		}
	}
	if len(order) == 0 {
		return []appTab{{Kind: tabInbox}, {Kind: tabActions}, {Kind: tabProjects}, {Kind: tabViews}}
	}
	return order
}

func findSavedViewByName(savedViews []model.SavedView, name string) (model.SavedView, bool) {
	for _, sv := range savedViews {
		if strings.EqualFold(sv.Name, name) {
			return sv, true
		}
	}
	return model.SavedView{}, false
}

func (m Model) activeTabIndex() int {
	for i, tab := range m.tabOrder {
		if m.isTabActive(tab) {
			return i
		}
	}
	return 0
}

func (m Model) isTabActive(tab appTab) bool {
	switch tab.Kind {
	case tabInbox:
		return (m.view == viewList && m.currentList == model.ListIn) || m.view == viewProcessInbox
	case tabActions:
		return m.view == viewList && m.currentList == model.ListSingleActions
	case tabProjects:
		return m.view == viewProjects || m.view == viewProjectDetail || m.view == viewProjectEdit
	case tabViews:
		if !(m.view == viewViews || m.view == viewWeeklyReview || m.view == viewViewResults) {
			return false
		}
		if m.view == viewViewResults {
			for _, t := range m.tabOrder {
				if t.Kind == tabSavedView && strings.EqualFold(t.View.Name, m.activeViewName) {
					return false
				}
			}
		}
		return true
	case tabSavedView:
		if m.view == viewViewResults {
			return strings.EqualFold(m.activeViewName, tab.View.Name)
		}
		if m.view == viewTaskDetail && m.detailFromView == viewViewResults {
			return strings.EqualFold(m.activeViewName, tab.View.Name)
		}
		return false
	default:
		return false
	}
}

func (m Model) activateTab(tab appTab) (tea.Model, tea.Cmd) {
	switch tab.Kind {
	case tabInbox:
		m.view = viewList
		m.currentList = model.ListIn
		m.cursor = 0
		return m, m.loadCurrentList
	case tabActions:
		m.view = viewList
		m.currentList = model.ListSingleActions
		m.cursor = 0
		return m, m.loadCurrentList
	case tabProjects:
		m.view = viewProjects
		m.cursor = 0
		return m, m.loadProjects
	case tabViews:
		m.view = viewViews
		m.viewListCursor = 0
		return m, nil
	case tabSavedView:
		m.view = viewViewResults
		return m, m.runQuery(tab.View.Name, tab.View.Query, tab.View.IncludeArchived)
	default:
		return m, nil
	}
}

func (m Model) handleTabHotkeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	key := msg.String()
	if key == "tab" {
		if len(m.tabOrder) == 0 {
			return m, nil, false
		}
		idx := m.activeTabIndex()
		next := m.tabOrder[(idx+1)%len(m.tabOrder)]
		modelNext, cmd := m.activateTab(next)
		return modelNext, cmd, true
	}
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		i := int(key[0] - '1')
		if i >= 0 && i < len(m.tabOrder) {
			modelNext, cmd := m.activateTab(m.tabOrder[i])
			return modelNext, cmd, true
		}
	}
	return m, nil, false
}

// Init loads the current list on startup.
func (m Model) Init() tea.Cmd {
	if m.view == viewProjects {
		return m.loadProjects
	}
	if m.view == viewViews {
		return nil
	}
	return m.loadCurrentList
}

var defaultKeybindings = map[string]map[string]string{
	"list": {
		"add":            "a",
		"open_url":       "o",
		"refile_actions": "r",
		"refile_project": "p",
		"someday":        "m",
		"waiting":        "w",
		"done":           "d",
		"cancel":         "c",
		"archive":        "A",
		"trash":          "x",
		"process":        "P",
	},
	"project": {
		"add_task":        "a",
		"add_subgroup":    "n",
		"open_url":        "o",
		"rename_subgroup": "R",
		"delete_subgroup": "X",
		"done":            "d",
		"cancel":          "c",
		"archive":         "A",
		"trash":           "x",
		"move_subgroup":   "m",
	},
	"view_results": {
		"done":     "d",
		"cancel":   "c",
		"open_url": "o",
		"someday":  "m",
		"waiting":  "w",
		"archive":  "A",
		"restore":  "U",
		"trash":    "x",
		"refresh":  "R",
	},
}

func mergeKeybindings(cfg config.Config) (map[string]map[string]string, map[string]map[string]bool) {
	out := map[string]map[string]string{}
	disabled := map[string]map[string]bool{
		"list":         {},
		"project":      {},
		"view_results": {},
	}
	for scope, defaults := range defaultKeybindings {
		m := map[string]string{}
		for action, key := range defaults {
			m[action] = key
		}
		out[scope] = m
	}
	for k, v := range cfg.Keys.List {
		if strings.TrimSpace(v) != "" {
			out["list"][k] = v
		}
	}
	for k, v := range cfg.Keys.Project {
		if strings.TrimSpace(v) != "" {
			out["project"][k] = v
		}
	}
	for k, v := range cfg.Keys.ViewResults {
		if strings.TrimSpace(v) != "" {
			out["view_results"][k] = v
		}
	}
	for _, a := range cfg.Keys.Disable.List {
		disabled["list"][a] = true
	}
	for _, a := range cfg.Keys.Disable.Project {
		disabled["project"][a] = true
	}
	for _, a := range cfg.Keys.Disable.ViewResults {
		disabled["view_results"][a] = true
	}
	return out, disabled
}

func (m Model) remapKey(scope string, raw string) string {
	cfgScope, ok := m.keybindings[scope]
	if !ok {
		return raw
	}
	for action, configuredKey := range cfgScope {
		defaultKey := defaultKeybindings[scope][action]
		if configuredKey == raw {
			return defaultKey
		}
	}
	return raw
}

func (m Model) actionDisabled(scope, action string) bool {
	if m.disabledAction == nil {
		return false
	}
	actions, ok := m.disabledAction[scope]
	if !ok {
		return false
	}
	return actions[action]
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
type taskRefiledMsg struct {
	text        string
	undoApply   func() error
	undoPrompt  string
	undoSuccess string
}
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
	name            string
	queryStr        string
	includeArchived bool
	results         []service.ViewTask
	status          string
	undoApply       func() error
	undoPrompt      string
	undoSuccess     string
	isFuzzy         bool
	err             error
}

type projectUpdatedMsg struct {
	title       string
	newFilename string
}
type errMsg struct{ err error }

// processInboxLoadedMsg carries the core-owned inbox session state when entering process mode.
type processInboxLoadedMsg struct{ session *core.InboxSession }

// processAdvancedMsg signals that the current item was acted on and we should advance.
// The action field is used to update processStats.
type processAdvancedMsg struct{ action string }

type weeklyReviewLoadedMsg struct {
	data service.WeeklyReviewData
	err  error
}

type clearPrefixMsg struct{}
type undoCountdownMsg struct{ seq int }

func stepFromCore(step core.InboxStep) processStep {
	switch step {
	case core.InboxStepActionable:
		return stepActionable
	case core.InboxStepNotActionable:
		return stepNotActionable
	case core.InboxStepEnrich:
		return stepEnrich
	case core.InboxStepRoute:
		return stepRoute
	case core.InboxStepWaitingOn:
		return stepWaitingOn
	case core.InboxStepNewProject:
		return stepNewProject
	case core.InboxStepComplete:
		return stepComplete
	default:
		return stepActionable
	}
}

func processStatsFromCore(summary core.InboxSessionSummary) processStats {
	return processStats{
		trashed:   summary.Trashed,
		someday:   summary.Someday,
		done:      summary.Done,
		waiting:   summary.Waiting,
		refiled:   summary.Refiled,
		toProject: summary.ToProject,
		skipped:   summary.Skipped,
	}
}

// Update handles messages and user input.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampCursor()
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
		if msg.undoApply == nil {
			m.statusMsg = fmt.Sprintf("Refiled: %s", msg.text)
		}
		// Reload whatever view we're on.
		var reload tea.Cmd
		if m.view == viewList {
			reload = m.loadCurrentList
		} else if m.view == viewProjects {
			reload = m.loadProjects
		} else if m.view == viewProjectDetail && m.activeFilename != "" {
			reload = m.reloadProjectDetail()
		}
		undoTick := m.setUndo(msg.undoPrompt, msg.undoApply, reload, msg.undoSuccess)
		return m, tea.Batch(reload, undoTick, m.clearStatusAfter())

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
		m.activeViewInclA = msg.includeArchived
		m.activeViewFuzzy = msg.isFuzzy
		m.viewResults = msg.results
		m.viewCursor = 0
		m.view = viewViewResults
		undoTick := m.setUndo(msg.undoPrompt, msg.undoApply, m.runQuery(msg.name, msg.queryStr, msg.includeArchived), msg.undoSuccess)
		if msg.status != "" {
			m.statusMsg = msg.status
			return m, tea.Batch(undoTick, m.clearStatusAfter())
		}
		return m, undoTick

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
		if msg.session == nil || len(msg.session.Items) == 0 {
			m.statusMsg = "Inbox is empty — nothing to process"
			return m, m.clearStatusAfter()
		}
		m.processSessionID = msg.session.ID
		m.processItems = msg.session.Items
		m.processIdx = msg.session.Index
		m.processStep = stepFromCore(msg.session.Current.Step)
		m.processTask = msg.session.Current.Draft
		m.processStats = processStatsFromCore(msg.session.Summary)
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

	case weeklyReviewLoadedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, m.clearStatusAfter()
		}
		m.weeklyReviewData = msg.data
		m.weeklyReviewStep = weeklyStepProjects
		m.weeklyReviewCursors = [weeklyStepCount]int{}
		m.view = viewWeeklyReview
		return m, nil

	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil

	case clearPrefixMsg:
		m.pendingPrefix = prefixNone
		return m, nil

	case undoCountdownMsg:
		if msg.seq != m.undoSeq || m.undoApply == nil {
			return m, nil
		}
		if time.Now().After(m.undoExpiresAt) || time.Now().Equal(m.undoExpiresAt) {
			m.clearUndo()
			m.statusMsg = ""
			return m, nil
		}
		m.statusMsg = m.undoStatusLine()
		return m, m.scheduleUndoCountdown(msg.seq)

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyPressMsg:
		// Keep undo status visible while grace is active.
		if m.undoApply == nil {
			m.statusMsg = ""
		}
		if m.mode == modeNormal && strings.EqualFold(msg.String(), m.undoKey) {
			return m.applyUndo()
		}
		if m.mode == modeNormal && m.view != viewProcessInbox {
			if next, cmd, ok := m.handleTabHotkeys(msg); ok {
				return next, cmd
			}
		}

		switch m.mode {
		case modeAdding:
			return m.updateAdding(msg)
		case modeAddingProject:
			return m.updateAddingProject(msg)
		case modeAddingSubGroup:
			return m.updateAddingSubGroup(msg)
		case modeRenamingSubGroup:
			return m.updateRenamingSubGroup(msg)
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
			case viewWeeklyReview:
				return m.updateWeeklyReview(msg)
			default:
				return m.updateNormal(msg)
			}
		}
	}

	// Pass through to text input in input modes.
	if m.mode == modeAdding || m.mode == modeAddingProject || m.mode == modeAddingSubGroup || m.mode == modeRenamingSubGroup || m.mode == modeAddingProjectTask || m.mode == modeEditingField {
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

func (m Model) loadWeeklyReview() tea.Msg {
	if err := m.store.Init(); err != nil {
		return weeklyReviewLoadedMsg{err: err}
	}
	data, err := m.svc.WeeklyReview(time.Now())
	if err != nil {
		return weeklyReviewLoadedMsg{err: err}
	}
	return weeklyReviewLoadedMsg{data: data}
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
	delay := time.Second * 3
	if m.undoApply != nil && m.undoDuration > delay {
		delay = m.undoDuration
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

func (m Model) clearPrefixAfter() tea.Cmd {
	return tea.Tick(time.Second*3, func(time.Time) tea.Msg {
		return clearPrefixMsg{}
	})
}

func (m *Model) beginPrefix(p keyPrefix) tea.Cmd {
	m.pendingPrefix = p
	switch p {
	case prefixState:
		m.statusMsg = "State: d done, c canceled, w waiting-for, m someday"
	case prefixRoute:
		m.statusMsg = "Route: a single-actions, p project"
	case prefixTime:
		m.statusMsg = "Time: d deadline, s scheduled"
	}
	return tea.Batch(m.clearStatusAfter(), m.clearPrefixAfter())
}

func (m *Model) cancelPrefix() {
	m.pendingPrefix = prefixNone
}

func (m *Model) clearUndo() {
	m.undoApply = nil
	m.undoReload = nil
	m.undoSuccess = ""
	m.undoPrompt = ""
	m.undoExpiresAt = time.Time{}
}

func (m *Model) setUndo(prompt string, apply func() error, reload tea.Cmd, success string) tea.Cmd {
	if !m.undoEnabled || m.undoDuration <= 0 || apply == nil {
		return nil
	}
	m.undoSeq++
	m.undoApply = apply
	m.undoReload = reload
	m.undoSuccess = success
	m.undoPrompt = prompt
	m.undoExpiresAt = time.Now().Add(m.undoDuration)
	m.statusMsg = m.undoStatusLine()
	seq := m.undoSeq
	return m.scheduleUndoCountdown(seq)
}

func (m *Model) scheduleUndoCountdown(seq int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return undoCountdownMsg{seq: seq}
	})
}

func (m *Model) undoStatusLine() string {
	remaining := int(math.Ceil(time.Until(m.undoExpiresAt).Seconds()))
	if remaining < 0 {
		remaining = 0
	}
	if m.undoPrompt == "" {
		return fmt.Sprintf("Press %s to undo (%ds)", strings.ToUpper(m.undoKey), remaining)
	}
	return fmt.Sprintf("%s — press %s to undo (%ds)", m.undoPrompt, strings.ToUpper(m.undoKey), remaining)
}

func (m *Model) applyUndo() (tea.Model, tea.Cmd) {
	if m.undoApply == nil {
		return m, nil
	}
	apply := m.undoApply
	reload := m.undoReload
	success := m.undoSuccess
	m.clearUndo()
	if err := apply(); err != nil {
		m.statusMsg = fmt.Sprintf("Undo failed: %v", err)
		return m, m.clearStatusAfter()
	}
	if success == "" {
		success = "Restored"
	}
	m.statusMsg = success
	return m, tea.Batch(reload, m.clearStatusAfter())
}

// updateNormal handles keys in normal (browsing) mode.
func (m Model) updateNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	raw := msg.String()
	if m.pendingPrefix != prefixNone {
		switch m.pendingPrefix {
		case prefixState:
			m.cancelPrefix()
			if raw == "esc" || m.list == nil || len(m.list.Tasks) == 0 {
				return m, nil
			}
			task := m.list.Tasks[m.cursor]
			switch raw {
			case "d":
				newState := model.StateDone
				if _, err := m.core.UpdateTask(task.ID, core.TaskPatch{State: &newState}); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
					return m, m.clearStatusAfter()
				}
				oldState := task.State
				undoTick := m.setUndo(
					"Task marked done",
					func() error { _, err := m.core.UpdateTask(task.ID, core.TaskPatch{State: &oldState}); return err },
					m.loadCurrentList,
					fmt.Sprintf("Restored: %s", task.Text),
				)
				return m, tea.Batch(m.loadCurrentList, undoTick, m.clearStatusAfter())
			case "c":
				newState := model.StateCanceled
				if _, err := m.core.UpdateTask(task.ID, core.TaskPatch{State: &newState}); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
					return m, m.clearStatusAfter()
				}
				oldState := task.State
				undoTick := m.setUndo(
					"Task canceled",
					func() error { _, err := m.core.UpdateTask(task.ID, core.TaskPatch{State: &oldState}); return err },
					m.loadCurrentList,
					fmt.Sprintf("Restored: %s", task.Text),
				)
				return m, tea.Batch(m.loadCurrentList, undoTick, m.clearStatusAfter())
			case "w":
				return m, m.setStateWaiting(task.ID, task.Text)
			case "m":
				return m, m.setStateSomeday(task.ID, task.Text)
			default:
				return m, nil
			}
		case prefixRoute:
			m.cancelPrefix()
			if raw == "esc" || m.list == nil || len(m.list.Tasks) == 0 {
				return m, nil
			}
			task := m.list.Tasks[m.cursor]
			if raw == "a" {
				if m.currentList != model.ListIn {
					m.statusMsg = "Route to single-actions only from inbox"
					return m, m.clearStatusAfter()
				}
				return m, m.refileTask(task, model.ListSingleActions, model.StateNextAction)
			}
			if raw == "p" {
				m.refileTaskID = task.ID
				m.refileTaskText = task.Text
				m.refileFromList = m.currentList
				m.refilePrevState = task.State
				m.mode = modePickingProject
				m.cursor = 0
				return m, m.loadProjects
			}
			return m, nil
		case prefixTime:
			m.cancelPrefix()
			if raw == "esc" || m.list == nil || len(m.list.Tasks) == 0 {
				return m, nil
			}
			if raw != "d" && raw != "s" {
				return m, nil
			}
			task := m.list.Tasks[m.cursor]
			m.detailTask = task
			m.detailFromView = viewList
			m.detailFromList = m.currentList
			m.detailFromSgIdx = -1
			m.detailIsProject = false
			if raw == "d" {
				m.detailField = fieldDeadline
				m.datePickerField = fieldDeadline
			} else {
				m.detailField = fieldScheduled
				m.datePickerField = fieldScheduled
			}
			m.view = viewTaskDetail
			m.mode = modePickingDate
			var initial time.Time
			if m.datePickerField == fieldDeadline && m.detailTask.Deadline != nil {
				initial = *m.detailTask.Deadline
			}
			if m.datePickerField == fieldScheduled && m.detailTask.Scheduled != nil {
				initial = *m.detailTask.Scheduled
			}
			return m, m.datePicker.OpenWithOptions(initial, true)
		}
	}
	key := m.remapKey("list", raw)
	switch key {
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
	case "e":
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

	case "o":
		if m.actionDisabled("list", "open_url") {
			return m, nil
		}
		if m.list == nil || len(m.list.Tasks) == 0 {
			return m, nil
		}
		task := m.list.Tasks[m.cursor]
		if err := openURLInBrowser(task.URL); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			return m, m.clearStatusAfter()
		}
		m.statusMsg = "Opened link"
		return m, m.clearStatusAfter()

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

	// Begin route prefix.
	case "r":
		return m, m.beginPrefix(prefixRoute)

	// Begin state prefix.
	case "s":
		return m, m.beginPrefix(prefixState)

	// Direct someday alias.
	case "m":
		if m.actionDisabled("list", "someday") {
			return m, nil
		}
		if m.list == nil || len(m.list.Tasks) == 0 {
			return m, nil
		}
		task := m.list.Tasks[m.cursor]
		return m, m.setStateSomeday(task.ID, task.Text)

	// Begin time prefix.
	case "t":
		return m, m.beginPrefix(prefixTime)

	// Refile to a project (from inbox or single-actions).
	case "p":
		if m.actionDisabled("list", "refile_project") {
			return m, nil
		}
		if m.list == nil || len(m.list.Tasks) == 0 {
			return m, nil
		}
		task := m.list.Tasks[m.cursor]
		m.refileTaskID = task.ID
		m.refileTaskText = task.Text
		m.refileFromList = m.currentList
		m.refilePrevState = task.State
		m.mode = modePickingProject
		m.cursor = 0
		return m, m.loadProjects

	// Set to waiting-for.
	case "w":
		if m.actionDisabled("list", "waiting") {
			return m, nil
		}
		if m.list == nil || len(m.list.Tasks) == 0 {
			return m, nil
		}
		task := m.list.Tasks[m.cursor]
		return m, m.setStateWaiting(task.ID, task.Text)

	// Mark done.
	case "d":
		if m.actionDisabled("list", "done") {
			return m, nil
		}
		if m.list != nil && len(m.list.Tasks) > 0 {
			task := m.list.Tasks[m.cursor]
			newState := model.StateDone
			if _, err := m.core.UpdateTask(task.ID, core.TaskPatch{State: &newState}); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				return m, m.clearStatusAfter()
			}
			oldState := task.State
			undoTick := m.setUndo(
				"Task marked done",
				func() error { _, err := m.core.UpdateTask(task.ID, core.TaskPatch{State: &oldState}); return err },
				m.loadCurrentList,
				fmt.Sprintf("Restored: %s", task.Text),
			)
			return m, tea.Batch(m.loadCurrentList, undoTick, m.clearStatusAfter())
		}

	// Mark canceled.
	case "c":
		if m.actionDisabled("list", "cancel") {
			return m, nil
		}
		if m.list != nil && len(m.list.Tasks) > 0 {
			task := m.list.Tasks[m.cursor]
			newState := model.StateCanceled
			if _, err := m.core.UpdateTask(task.ID, core.TaskPatch{State: &newState}); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				return m, m.clearStatusAfter()
			}
			oldState := task.State
			undoTick := m.setUndo(
				"Task canceled",
				func() error { _, err := m.core.UpdateTask(task.ID, core.TaskPatch{State: &oldState}); return err },
				m.loadCurrentList,
				fmt.Sprintf("Restored: %s", task.Text),
			)
			return m, tea.Batch(m.loadCurrentList, undoTick, m.clearStatusAfter())
		}

	// Archive task.
	case "A":
		if m.actionDisabled("list", "archive") {
			return m, nil
		}
		if m.list != nil && len(m.list.Tasks) > 0 {
			task := m.list.Tasks[m.cursor]
			if err := m.core.ArchiveTask(task.ID); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				return m, m.clearStatusAfter()
			}
			undoTick := m.setUndo(
				"Task archived",
				func() error {
					_, err := m.core.RestoreTask(task.ID)
					return err
				},
				m.loadCurrentList,
				fmt.Sprintf("Restored: %s", task.Text),
			)
			return m, tea.Batch(m.loadCurrentList, undoTick, m.clearStatusAfter())
		}

	// Trash task.
	case "x":
		if m.actionDisabled("list", "trash") {
			return m, nil
		}
		if m.list != nil && len(m.list.Tasks) > 0 {
			task := m.list.Tasks[m.cursor]
			if err := m.core.TrashTask(task.ID); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				return m, m.clearStatusAfter()
			}
			undoTick := m.setUndo(
				"Task trashed",
				func() error {
					_, err := m.svc.RestoreTask(task, string(m.list.Type))
					return err
				},
				m.loadCurrentList,
				fmt.Sprintf("Restored: %s", task.Text),
			)
			return m, tea.Batch(m.loadCurrentList, undoTick, m.clearStatusAfter())
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

// loadInboxForProcessing starts a core-owned inbox session and returns it to kick off process mode.
func (m Model) loadInboxForProcessing() tea.Msg {
	session, err := m.core.StartInboxSession()
	if err != nil {
		return errMsg{err}
	}
	return processInboxLoadedMsg{session: session}
}

// refileTask moves a task from the current list to a destination list.
func (m Model) refileTask(task model.Task, destList model.ListType, newState model.TaskState) tea.Cmd {
	fromList := m.currentList
	return func() tea.Msg {
		_, err := m.core.MoveTaskToList(task.ID, destList, newState)
		if err != nil {
			return errMsg{err}
		}
		undoApply := func() error {
			_, err := m.core.MoveTaskToList(task.ID, fromList, task.State)
			return err
		}
		return taskRefiledMsg{text: task.Text, undoApply: undoApply, undoPrompt: "Task refiled", undoSuccess: fmt.Sprintf("Restored: %s", task.Text)}
	}
}

// setStateSomeday sets a task to someday/maybe state.
// If on inbox, refiles to single-actions. If already on single-actions, updates in place.
func (m Model) setStateSomeday(taskID, text string) tea.Cmd {
	return func() tea.Msg {
		if m.currentList == model.ListIn {
			if _, err := m.core.MoveTaskToList(taskID, model.ListSingleActions, model.StateSomeday); err != nil {
				return errMsg{err}
			}
		} else {
			newState := model.StateSomeday
			if _, err := m.core.UpdateTask(taskID, core.TaskPatch{State: &newState}); err != nil {
				return errMsg{err}
			}
		}
		return taskRefiledMsg{text: text}
	}
}

// setStateWaiting sets a task to waiting-for state.
// If on inbox, refiles to single-actions. If already on single-actions, updates in place.
func (m Model) setStateWaiting(taskID, text string) tea.Cmd {
	return func() tea.Msg {
		if m.currentList == model.ListIn {
			if _, err := m.core.MoveTaskToList(taskID, model.ListSingleActions, model.StateWaitingFor); err != nil {
				return errMsg{err}
			}
		} else {
			newState := model.StateWaitingFor
			if _, err := m.core.UpdateTask(taskID, core.TaskPatch{State: &newState}); err != nil {
				return errMsg{err}
			}
		}
		return taskRefiledMsg{text: text}
	}
}

// ── Process Inbox Mode ───────────────────────────────────────────────────────

// advanceProcessInbox moves to the next item after an action completes.
// If all items are processed it switches to the completion step.
func (m *Model) advanceProcessInbox() {
	if m.processSessionID != "" {
		session, err := m.core.SkipInboxItem(m.processSessionID)
		if err == nil {
			m.processItems = session.Items
			m.processIdx = session.Index
			m.processStep = stepFromCore(session.Current.Step)
			m.processTask = session.Current.Draft
			m.processStats = processStatsFromCore(session.Summary)
			m.processTags = nil
			return
		}
	}
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
		if err := m.svc.UpdateTask(model.ListIn, task); err != nil {
			return errMsg{err}
		}
		projects, err := m.core.ListProjectSummaries()
		if err != nil {
			return errMsg{err}
		}
		var projectID string
		for _, p := range projects {
			if p.Filename == filename {
				projectID = p.ID
				break
			}
		}
		if projectID == "" {
			return errMsg{fmt.Errorf("project %q not found after creation", filename)}
		}
		proj, err := m.core.GetProject(projectID)
		if err != nil {
			return errMsg{err}
		}
		targetSubgroupID := ""
		if len(proj.Project.SubGroups) == 0 {
			sg, err := m.core.CreateSubgroup(projectID, "Tasks")
			if err != nil {
				return errMsg{err}
			}
			targetSubgroupID = sg.Subgroup.ID
		} else {
			targetSubgroupID = proj.Project.SubGroups[0].ID
		}
		if _, err := m.core.MoveTaskToProject(task.ID, projectID, targetSubgroupID, model.StateNextAction); err != nil {
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
		// Done (< 2 min, did it). Persist enrichment + mark done.
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
			if _, err := m.core.MoveTaskToList(task.ID, model.ListSingleActions, model.StateSomeday); err != nil {
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
			if _, err := m.core.MoveTaskToList(task.ID, model.ListSingleActions, model.StateNextAction); err != nil {
				return errMsg{err}
			}
			return processAdvancedMsg{action: "refiled"}
		}

	case "p":
		// Pick an existing project.
		m.refileTaskID = m.processTask.ID
		m.refileTaskText = m.processTask.Text
		m.refileFromList = model.ListIn
		m.refilePrevState = m.processTask.State
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
			if _, err := m.core.MoveTaskToList(task.ID, model.ListSingleActions, model.StateWaitingFor); err != nil {
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
	raw := msg.String()
	if m.pendingPrefix != prefixNone {
		switch m.pendingPrefix {
		case prefixState:
			m.cancelPrefix()
			if raw == "esc" || len(flatItems) == 0 {
				return m, nil
			}
			item := flatItems[m.projCursor]
			if !item.isTask {
				return m, nil
			}
			switch raw {
			case "d":
				newState := model.StateDone
				if _, err := m.core.UpdateTask(item.task.ID, core.TaskPatch{State: &newState}); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
					return m, m.clearStatusAfter()
				}
				oldState := item.task.State
				undoTick := m.setUndo(
					"Task marked done",
					func() error { _, err := m.core.UpdateTask(item.task.ID, core.TaskPatch{State: &oldState}); return err },
					m.reloadProjectDetail(),
					fmt.Sprintf("Restored: %s", item.task.Text),
				)
				return m, tea.Batch(m.reloadProjectDetail(), undoTick, m.clearStatusAfter())
			case "c":
				newState := model.StateCanceled
				if _, err := m.core.UpdateTask(item.task.ID, core.TaskPatch{State: &newState}); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
					return m, m.clearStatusAfter()
				}
				oldState := item.task.State
				undoTick := m.setUndo(
					"Task canceled",
					func() error { _, err := m.core.UpdateTask(item.task.ID, core.TaskPatch{State: &oldState}); return err },
					m.reloadProjectDetail(),
					fmt.Sprintf("Restored: %s", item.task.Text),
				)
				return m, tea.Batch(m.reloadProjectDetail(), undoTick, m.clearStatusAfter())
			case "w":
				newState := model.StateWaitingFor
				_, _ = m.core.UpdateTask(item.task.ID, core.TaskPatch{State: &newState})
				return m, m.reloadProjectDetail()
			case "m":
				newState := model.StateSomeday
				_, _ = m.core.UpdateTask(item.task.ID, core.TaskPatch{State: &newState})
				return m, m.reloadProjectDetail()
			default:
				return m, nil
			}
		case prefixTime:
			m.cancelPrefix()
			if raw == "esc" || (raw != "d" && raw != "s") || len(flatItems) == 0 {
				return m, nil
			}
			item := flatItems[m.projCursor]
			if !item.isTask {
				return m, nil
			}
			m.detailTask = item.task
			m.detailFromView = viewProjectDetail
			m.detailFromSgIdx = item.sgIdx
			m.detailIsProject = true
			if raw == "d" {
				m.detailField = fieldDeadline
				m.datePickerField = fieldDeadline
			} else {
				m.detailField = fieldScheduled
				m.datePickerField = fieldScheduled
			}
			m.view = viewTaskDetail
			m.mode = modePickingDate
			var initial time.Time
			if m.datePickerField == fieldDeadline && m.detailTask.Deadline != nil {
				initial = *m.detailTask.Deadline
			}
			if m.datePickerField == fieldScheduled && m.detailTask.Scheduled != nil {
				initial = *m.detailTask.Scheduled
			}
			return m, m.datePicker.OpenWithOptions(initial, true)
		default:
			m.cancelPrefix()
		}
	}
	key := m.remapKey("project", raw)

	switch key {
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

	case "e":
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

	case "o":
		if m.actionDisabled("project", "open_url") {
			return m, nil
		}
		if len(flatItems) == 0 {
			return m, nil
		}
		item := flatItems[m.projCursor]
		if !item.isTask {
			m.statusMsg = "Select a task row to open URL"
			return m, m.clearStatusAfter()
		}
		if err := openURLInBrowser(item.task.URL); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			return m, m.clearStatusAfter()
		}
		m.statusMsg = "Opened link"
		return m, m.clearStatusAfter()

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

	case "R":
		if m.actionDisabled("project", "rename_subgroup") {
			return m, nil
		}
		if len(flatItems) == 0 {
			return m, nil
		}
		item := flatItems[m.projCursor]
		if item.isTask {
			m.statusMsg = "Select a sub-group heading to rename"
			return m, m.clearStatusAfter()
		}
		m.mode = modeRenamingSubGroup
		m.renameSgIdx = item.sgIdx
		m.input.Reset()
		m.input.Placeholder = "Sub-group name"
		m.input.SetValue(item.sgTitle)
		cmd := m.input.Focus()
		return m, cmd

	case "X":
		if m.actionDisabled("project", "delete_subgroup") {
			return m, nil
		}
		if len(flatItems) == 0 {
			return m, nil
		}
		item := flatItems[m.projCursor]
		if item.isTask {
			m.statusMsg = "Select a sub-group heading to delete"
			return m, m.clearStatusAfter()
		}
		if err := m.core.DeleteSubgroup(m.activeProject.ID, m.activeProject.SubGroups[item.sgIdx].ID); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			return m, m.clearStatusAfter()
		}
		m.statusMsg = fmt.Sprintf("Deleted sub-group: %s", item.sgTitle)
		return m, tea.Batch(m.reloadProjectDetail(), m.clearStatusAfter())

	case "s":
		return m, m.beginPrefix(prefixState)

	case "t":
		return m, m.beginPrefix(prefixTime)

	case "d":
		if m.actionDisabled("project", "done") {
			return m, nil
		}
		// Mark task done.
		if len(flatItems) > 0 {
			item := flatItems[m.projCursor]
			if item.isTask {
				newState := model.StateDone
				if _, err := m.core.UpdateTask(item.task.ID, core.TaskPatch{State: &newState}); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
					return m, m.clearStatusAfter()
				}
				oldState := item.task.State
				undoTick := m.setUndo(
					"Task marked done",
					func() error { _, err := m.core.UpdateTask(item.task.ID, core.TaskPatch{State: &oldState}); return err },
					m.reloadProjectDetail(),
					fmt.Sprintf("Restored: %s", item.task.Text),
				)
				return m, tea.Batch(m.reloadProjectDetail(), undoTick, m.clearStatusAfter())
			}
		}

	case "c":
		if m.actionDisabled("project", "cancel") {
			return m, nil
		}
		// Mark task canceled.
		if len(flatItems) > 0 {
			item := flatItems[m.projCursor]
			if item.isTask {
				newState := model.StateCanceled
				if _, err := m.core.UpdateTask(item.task.ID, core.TaskPatch{State: &newState}); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
					return m, m.clearStatusAfter()
				}
				oldState := item.task.State
				undoTick := m.setUndo(
					"Task canceled",
					func() error { _, err := m.core.UpdateTask(item.task.ID, core.TaskPatch{State: &oldState}); return err },
					m.reloadProjectDetail(),
					fmt.Sprintf("Restored: %s", item.task.Text),
				)
				return m, tea.Batch(m.reloadProjectDetail(), undoTick, m.clearStatusAfter())
			}
		}

	case "A":
		if m.actionDisabled("project", "archive") {
			return m, nil
		}
		// Archive selected project task.
		if len(flatItems) > 0 {
			item := flatItems[m.projCursor]
			if item.isTask {
				if err := m.core.ArchiveTask(item.task.ID); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
					return m, m.clearStatusAfter()
				}
				undoTick := m.setUndo(
					"Task archived",
					func() error {
						_, err := m.core.RestoreTask(item.task.ID)
						return err
					},
					m.reloadProjectDetail(),
					fmt.Sprintf("Restored: %s", item.task.Text),
				)
				return m, tea.Batch(m.reloadProjectDetail(), undoTick, m.clearStatusAfter())
			}
		}

	case "x":
		if m.actionDisabled("project", "trash") {
			return m, nil
		}
		// Permanently delete selected project task.
		if len(flatItems) > 0 {
			item := flatItems[m.projCursor]
			if item.isTask {
				if err := m.core.TrashTask(item.task.ID); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
					return m, m.clearStatusAfter()
				}
				source := fmt.Sprintf("projects/%s", m.activeFilename)
				undoTick := m.setUndo(
					"Task trashed",
					func() error {
						_, err := m.svc.RestoreTask(item.task, source)
						return err
					},
					m.reloadProjectDetail(),
					fmt.Sprintf("Restored: %s", item.task.Text),
				)
				return m, tea.Batch(m.reloadProjectDetail(), undoTick, m.clearStatusAfter())
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
		if m.actionDisabled("project", "move_subgroup") {
			return m, nil
		}
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
		m.refilePrevState = model.StateEmpty
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
			return m, m.moveToProject(proj.ID, proj.Title)
		}
	}

	return m, nil
}

// processMoveToProject persists enrichment edits then moves the current process
// task into the first sub-group of the chosen project.
func (m Model) processMoveToProject(filename, projTitle string) tea.Cmd {
	task := m.processTask
	return func() tea.Msg {
		if err := m.svc.UpdateTask(model.ListIn, task); err != nil {
			return errMsg{err}
		}
		projects, err := m.core.ListProjectSummaries()
		if err != nil {
			return errMsg{err}
		}
		var projectID string
		for _, p := range projects {
			if p.Filename == filename {
				projectID = p.ID
				break
			}
		}
		if projectID == "" {
			return errMsg{fmt.Errorf("project %q not found", filename)}
		}
		proj, err := m.core.GetProject(projectID)
		if err != nil {
			return errMsg{err}
		}
		targetSubgroupID := ""
		if len(proj.Project.SubGroups) == 0 {
			sg, err := m.core.CreateSubgroup(projectID, "Tasks")
			if err != nil {
				return errMsg{err}
			}
			targetSubgroupID = sg.Subgroup.ID
		} else {
			targetSubgroupID = proj.Project.SubGroups[0].ID
		}
		if _, err := m.core.MoveTaskToProject(task.ID, projectID, targetSubgroupID, model.StateNextAction); err != nil {
			return errMsg{err}
		}
		_ = projTitle
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
			targetSubgroupID := m.activeProject.SubGroups[toSgIdx].ID
			return m, func() tea.Msg {
				err := m.core.MoveTaskToSubgroup(taskID, targetSubgroupID)
				if err != nil {
					return errMsg{err}
				}
				return taskRefiledMsg{text: fmt.Sprintf("Moved \"%s\" to %s", taskText, m.activeProject.SubGroups[toSgIdx].Title)}
			}
		}
	}

	return m, nil
}

// moveToProject moves the refile task to the first sub-group of a project.
func (m Model) moveToProject(projectID, projTitle string) tea.Cmd {
	taskID := m.refileTaskID
	taskText := m.refileTaskText
	fromList := m.refileFromList
	oldState := m.refilePrevState
	return func() tea.Msg {
		proj, err := m.core.GetProject(projectID)
		if err != nil {
			return errMsg{err}
		}
		targetSubgroupID := ""
		if len(proj.Project.SubGroups) == 0 {
			sg, err := m.core.CreateSubgroup(projectID, "Tasks")
			if err != nil {
				return errMsg{err}
			}
			targetSubgroupID = sg.Subgroup.ID
		} else {
			targetSubgroupID = proj.Project.SubGroups[0].ID
		}
		moved, err := m.core.MoveTaskToProject(taskID, projectID, targetSubgroupID, model.StateNextAction)
		if err != nil {
			return errMsg{err}
		}
		undoApply := func() error {
			_, err := m.core.MoveTaskToList(taskID, fromList, oldState)
			return err
		}
		_ = moved
		return taskRefiledMsg{text: fmt.Sprintf("%s -> %s", taskText, projTitle), undoApply: undoApply, undoPrompt: "Task refiled", undoSuccess: fmt.Sprintf("Restored: %s", taskText)}
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
			_, err := m.core.CreateSubgroup(m.activeProject.ID, title)
			if err != nil {
				return errMsg{err}
			}
			return taskRefiledMsg{text: fmt.Sprintf("Added sub-group: %s", title)}
		}

	case "esc":
		m.mode = modeNormal
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateRenamingSubGroup handles input when renaming a sub-group.
func (m Model) updateRenamingSubGroup(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		title := strings.TrimSpace(m.input.Value())
		if title == "" {
			m.mode = modeNormal
			return m, nil
		}
		sgIdx := m.renameSgIdx
		m.mode = modeNormal
		return m, func() tea.Msg {
			if _, err := m.core.RenameSubgroup(m.activeProject.ID, m.activeProject.SubGroups[sgIdx].ID, title); err != nil {
				return errMsg{err}
			}
			return taskRefiledMsg{text: fmt.Sprintf("Renamed sub-group: %s", title)}
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
		subgroupID := m.activeProject.SubGroups[sgIdx].ID
		return m, func() tea.Msg {
			_, err := m.core.AddProjectTask(m.activeProject.ID, subgroupID, text, core.CaptureOpts{})
			if err != nil {
				return errMsg{err}
			}
			return taskRefiledMsg{text: fmt.Sprintf("Added: %s", text)}
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
	case m.view == viewWeeklyReview:
		m.renderWeeklyReview(&b)
	case m.view == viewProjects:
		m.renderProjectListView(&b)
	case m.view == viewProjectDetail:
		m.renderProjectDetailView(&b)
	default:
		m.renderListView(&b)
	}

	// Input area (for various input modes).
	if m.mode == modeAdding || m.mode == modeAddingProject || m.mode == modeAddingSubGroup || m.mode == modeRenamingSubGroup || m.mode == modeAddingProjectTask {
		b.WriteString("\n")
		var prompt string
		switch m.mode {
		case modeAdding:
			prompt = "  New task: "
		case modeAddingProject:
			prompt = "  New project: "
		case modeAddingSubGroup:
			prompt = "  New sub-group: "
		case modeRenamingSubGroup:
			prompt = "  Rename sub-group: "
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
	helpWidth := m.width - 4
	if helpWidth < 24 {
		helpWidth = 24
	}
	for _, line := range wrapText(m.helpText(), helpWidth) {
		b.WriteString(helpStyle.Render("  " + line))
		b.WriteString("\n")
	}

	v.SetContent(b.String())
	return v
}

func wrapText(text string, width int) []string {
	if width <= 0 || len(text) <= width {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}

	lines := make([]string, 0, 4)
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) <= width {
			line += " " + w
			continue
		}
		lines = append(lines, line)
		line = w
	}
	lines = append(lines, line)
	return lines
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
		return
	}

	start, end := m.visibleRange(len(m.list.Tasks), m.cursor)
	for i := start; i < end; i++ {
		b.WriteString(m.renderTask(i, m.list.Tasks[i]))
		b.WriteString("\n")
	}
}

// renderProjectListView renders the list of all projects.
func (m Model) renderProjectListView(b *strings.Builder) {
	if len(m.projects) == 0 {
		b.WriteString(taskStyle.Render("No projects yet. Press 'a' to create one."))
		b.WriteString("\n")
		return
	}

	start, end := m.visibleRange(len(m.projects), m.cursor)
	for i := start; i < end; i++ {
		p := m.projects[i]
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

	start, end := m.visibleRange(len(items), m.projCursor)
	for i := start; i < end; i++ {
		item := items[i]
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
					b.WriteString(checkboxOpenStyle.Render("[ ] "))
					b.WriteString(selectedTaskStyle.Render(item.task.Text))
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
			if item.task.Scheduled != nil {
				meta = append(meta, scheduledStyle.Render("sched:"+formatOptionalTime(item.task.Scheduled)))
			}
			if item.task.Deadline != nil {
				meta = append(meta, deadlineStyle.Render("due:"+formatOptionalTime(item.task.Deadline)))
			}
			if link := urlIndicatorMeta(item.task.URL); link != "" {
				meta = append(meta, link)
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
			b.WriteString(checkboxOpenStyle.Render("[ ] "))
			b.WriteString(selectedTaskStyle.Render(task.Text))
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
	if task.Scheduled != nil {
		meta = append(meta, scheduledStyle.Render("sched:"+formatOptionalTime(task.Scheduled)))
	}
	if task.Deadline != nil {
		meta = append(meta, deadlineStyle.Render("due:"+formatOptionalTime(task.Deadline)))
	}
	if link := urlIndicatorMeta(task.URL); link != "" {
		meta = append(meta, link)
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
				b.WriteString(helpStyle.Render("  (enter: open calendar, del clears)"))
			}
		} else if f == fieldState {
			b.WriteString(stateStyle.Render(value))
		} else if isDateField {
			b.WriteString(deadlineStyle.Render(value))
			if isSelected {
				b.WriteString(helpStyle.Render("  (enter: open calendar, del clears)"))
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

	// Modified (when tracked).
	b.WriteString("   ")
	b.WriteString(stateStyle.Render(fmt.Sprintf("%-14s", "Modified:")))
	b.WriteString(" ")
	if m.detailTask.ModifiedAt != nil {
		b.WriteString(stateStyle.Render(m.detailTask.ModifiedAt.Format("2006-01-02 15:04")))
	} else {
		b.WriteString(helpStyle.Render("—"))
	}
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
	if t, hasTime, confirmed, cancelled, cleared := m.datePicker.Result(); confirmed {
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
	} else if cleared {
		if m.view == viewProcessInbox {
			switch m.datePickerField {
			case fieldDeadline:
				m.processTask.Deadline = nil
			case fieldScheduled:
				m.processTask.Scheduled = nil
			}
		} else if m.view == viewProjectEdit {
			m.projEditProject.Deadline = nil
		} else {
			switch m.datePickerField {
			case fieldDeadline:
				m.detailTask.Deadline = nil
			case fieldScheduled:
				m.detailTask.Scheduled = nil
			}
		}
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
			return m, m.runQuery(m.activeViewName, m.activeViewQuery, m.activeViewInclA)
		}
		if m.detailFromView == viewWeeklyReview {
			return m, m.loadWeeklyReview
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
			cmd := m.datePicker.OpenWithOptions(initial, true)
			return m, cmd
		}
		m.mode = modeEditingField
		m.input.Reset()
		m.input.Placeholder = fieldLabel(m.detailField)
		m.input.SetValue(m.fieldValue(m.detailField))
		cmd := m.input.Focus()
		return m, cmd

	case "backspace", "delete":
		if m.detailField == fieldDeadline {
			m.detailTask.Deadline = nil
			m.statusMsg = "Deadline cleared"
			return m, m.clearStatusAfter()
		}
		if m.detailField == fieldScheduled {
			m.detailTask.Scheduled = nil
			m.statusMsg = "Scheduled cleared"
			return m, m.clearStatusAfter()
		}

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
		if m.view == viewViews {
			m.mode = modeNormal
			if val == "" {
				return m, nil
			}
			return m, m.runQuery("Ad-hoc", val, false)
		}
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
		prevStatus := m.statusMsg
		m.applyFieldEdit(val)
		if m.statusMsg != prevStatus {
			return m, m.clearStatusAfter()
		}
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
		t, ok := parseDateTime(val)
		if !ok {
			m.statusMsg = "Invalid date/time. Use YYYY-MM-DD or YYYY-MM-DD HH:MM"
			return
		}
		m.detailTask.Deadline = t
	case fieldScheduled:
		t, ok := parseDateTime(val)
		if !ok {
			m.statusMsg = "Invalid date/time. Use YYYY-MM-DD or YYYY-MM-DD HH:MM"
			return
		}
		m.detailTask.Scheduled = t
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
func parseDateTime(s string) (*time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	formats := []string{
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return &t, true
		}
	}
	return nil, false
}

// saveDetailTask writes the working copy back to disk and returns to the prior view.
func (m Model) saveDetailTask() tea.Cmd {
	task := m.detailTask
	_ = m.detailIsProject
	_ = m.activeFilename
	_ = m.detailFromSgIdx
	fromView := m.detailFromView
	viewName := m.activeViewName
	viewQueryStr := m.activeViewQuery
	includeArchived := m.activeViewInclA
	return func() tea.Msg {
		var err error
		_, err = m.core.UpdateTask(task.ID, core.TaskPatch{
			Text:      &task.Text,
			State:     &task.State,
			Deadline:  task.Deadline,
			Scheduled: task.Scheduled,
			Tags:      slicePtr(task.Tags),
			URL:       stringPtr(task.URL),
			Notes:     stringPtr(task.Notes),
			WaitingOn: stringPtr(task.WaitingOn),
		})
		if err != nil {
			return errMsg{err}
		}
		// If we came from a view results screen, refresh the view after saving.
		if fromView == viewViewResults {
			clauses, _ := query.Parse(viewQueryStr, time.Now())
			all, err2 := m.collectViewTasks(viewName, includeArchived)
			if err2 != nil {
				return taskUpdatedMsg{task.Text}
			}
			var filtered []service.ViewTask
			for _, v := range all {
				if query.MatchAll(clauses, v.Task, v.Source) {
					filtered = append(filtered, v)
				}
			}
			return viewResultsLoadedMsg{name: viewName, queryStr: viewQueryStr, includeArchived: includeArchived, results: filtered}
		}
		if fromView == viewWeeklyReview {
			return m.loadWeeklyReview()
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
	labels := make([]string, 0, len(m.tabOrder))
	for i, tab := range m.tabOrder {
		var label string
		switch tab.Kind {
		case tabInbox:
			count := 0
			if m.view == viewList && m.currentList == model.ListIn && m.list != nil {
				count = len(m.list.Tasks)
			}
			label = fmt.Sprintf(" %d Inbox ", i+1)
			if m.isTabActive(tab) {
				label = fmt.Sprintf("%s(%d)", label, count)
			}
		case tabActions:
			count := 0
			if m.view == viewList && m.currentList == model.ListSingleActions && m.list != nil {
				count = len(m.list.Tasks)
			}
			label = fmt.Sprintf(" %d Actions ", i+1)
			if m.isTabActive(tab) {
				label = fmt.Sprintf("%s(%d)", label, count)
			}
		case tabProjects:
			label = fmt.Sprintf(" %d Projects ", i+1)
			if m.isTabActive(tab) {
				label = fmt.Sprintf("%s(%d)", label, len(m.projects))
			}
		case tabViews:
			label = fmt.Sprintf(" %d Views ", i+1)
		case tabSavedView:
			label = fmt.Sprintf(" %d %s ", i+1, tab.View.Name)
		}

		if m.view == viewProcessInbox && tab.Kind == tabInbox {
			label = fmt.Sprintf(" Processing Inbox (%d of %d) ", m.processIdx+1, len(m.processItems))
			if m.processStep == stepComplete {
				label = " Inbox Processed! "
			}
		}

		if (m.view == viewProcessInbox && tab.Kind == tabInbox) || (m.view != viewProcessInbox && m.isTabActive(tab)) {
			labels = append(labels, activeTabStyle.Render(label))
		} else {
			labels = append(labels, inactiveTabStyle.Render(label))
		}
	}
	return strings.Join(labels, "  ")
}

// helpText returns contextual help based on mode and current view.
func (m Model) helpText() string {
	if m.mode == modeAdding || m.mode == modeAddingProject || m.mode == modeAddingSubGroup || m.mode == modeRenamingSubGroup || m.mode == modeAddingProjectTask {
		return "enter: save  esc: cancel"
	}
	if m.mode == modePickingProject || m.mode == modePickingSubGroup {
		return "enter: select  esc: cancel  j/k: navigate"
	}
	if m.mode == modePickingDate {
		return "arrows/hjkl: move day  </> or [/]: month  t: toggle time  enter: confirm (date-only when time hidden)  del/backspace: clear  esc: cancel"
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
			return ""
		case stepNotActionable:
			return "t: trash  esc: back"
		case stepEnrich:
			return ""
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
		return "enter: open view  /: ad-hoc query  ?: fuzzy search  W: weekly review  j/k: navigate  1..N/tab: switch tab  q: quit"
	}
	if m.view == viewWeeklyReview {
		return "j/k: navigate  h/l: prev/next section  enter: open item  o: open URL  d/c/s/w: state  A: archive  x: trash  R: refresh  esc: back"
	}
	if m.view == viewViewResults {
		if m.pendingPrefix == prefixState {
			return "state -> d: done  c: canceled  w: waiting-for  m: someday  esc: cancel"
		}
		if m.pendingPrefix == prefixTime {
			return "time -> d: deadline  s: scheduled  esc: cancel"
		}
		parts := []string{"e: detail", "s: state-prefix", "t: time-prefix"}
		if !m.actionDisabled("view_results", "open_url") {
			parts = append(parts, "o: open URL")
		}
		quick := []string{}
		if !m.actionDisabled("view_results", "done") {
			quick = append(quick, "d")
		}
		if !m.actionDisabled("view_results", "cancel") {
			quick = append(quick, "c")
		}
		if !m.actionDisabled("view_results", "waiting") {
			quick = append(quick, "w")
		}
		if !m.actionDisabled("view_results", "someday") {
			parts = append(parts, "m: someday")
			quick = append(quick, "m")
		}
		if len(quick) > 0 {
			parts = append(parts, strings.Join(quick, "/")+": quick states")
		}
		if !m.actionDisabled("view_results", "archive") {
			parts = append(parts, "A: archive")
		}
		if !m.actionDisabled("view_results", "restore") {
			parts = append(parts, "U: restore")
		}
		if !m.actionDisabled("view_results", "trash") {
			parts = append(parts, "x: trash")
		}
		if !m.actionDisabled("view_results", "refresh") {
			parts = append(parts, "R: refresh")
		}
		parts = append(parts, "esc: back")
		return strings.Join(parts, "  ")
	}

	nav := "j/k: navigate  tab: next tab  q: quit"

	switch m.view {
	case viewProjects:
		return "enter: open  a: new project  E: edit project  " + nav
	case viewProjectDetail:
		if m.pendingPrefix == prefixState {
			return "state -> d: done  c: canceled  w: waiting-for  m: someday  esc: cancel"
		}
		if m.pendingPrefix == prefixTime {
			return "time -> d: deadline  s: scheduled  esc: cancel"
		}
		parts := []string{"e: detail", "a: add task", "n: new sub-group", "s: state-prefix", "t: time-prefix"}
		if !m.actionDisabled("project", "open_url") {
			parts = append(parts, "o: open URL")
		}
		if !m.actionDisabled("project", "rename_subgroup") {
			parts = append(parts, "R: rename sub-group")
		}
		if !m.actionDisabled("project", "delete_subgroup") {
			parts = append(parts, "X: delete sub-group")
		}
		quick := []string{}
		if !m.actionDisabled("project", "done") {
			quick = append(quick, "d")
		}
		if !m.actionDisabled("project", "cancel") {
			quick = append(quick, "c")
		}
		if len(quick) > 0 {
			parts = append(parts, strings.Join(quick, "/")+": quick states")
		}
		if !m.actionDisabled("project", "archive") {
			parts = append(parts, "A: archive")
		}
		if !m.actionDisabled("project", "trash") {
			parts = append(parts, "x: trash")
		}
		parts = append(parts, "E: edit project", "C-j/C-k: reorder")
		if !m.actionDisabled("project", "move_subgroup") {
			parts = append(parts, "m: move")
		}
		parts = append(parts, "esc: back")
		return strings.Join(parts, "  ")
	default:
		if m.pendingPrefix == prefixState {
			return "state -> d: done  c: canceled  w: waiting-for  m: someday  esc: cancel"
		}
		if m.pendingPrefix == prefixRoute {
			return "route -> a: single-actions  p: project  esc: cancel"
		}
		if m.pendingPrefix == prefixTime {
			return "time -> d: deadline  s: scheduled  esc: cancel"
		}
		if m.currentList == model.ListIn {
			parts := []string{"e: detail", "a: add", "P: process", "s: state-prefix", "r: route-prefix", "t: time-prefix"}
			if !m.actionDisabled("list", "open_url") {
				parts = append(parts, "o: open URL")
			}
			if !m.actionDisabled("list", "refile_project") {
				parts = append(parts, "p: to project")
			}
			quick := []string{}
			if !m.actionDisabled("list", "someday") {
				parts = append(parts, "m: someday")
				quick = append(quick, "m")
			}
			if !m.actionDisabled("list", "done") {
				quick = append(quick, "d")
			}
			if !m.actionDisabled("list", "cancel") {
				quick = append(quick, "c")
			}
			if !m.actionDisabled("list", "waiting") {
				quick = append(quick, "w")
			}
			if len(quick) > 0 {
				parts = append(parts, strings.Join(quick, "/")+": quick states")
			}
			if !m.actionDisabled("list", "archive") {
				parts = append(parts, "A: archive")
			}
			if !m.actionDisabled("list", "trash") {
				parts = append(parts, "x: trash")
			}
			return strings.Join(parts, "  ") + "  " + nav
		}
		parts := []string{"e: detail", "s: state-prefix", "r: route-prefix", "t: time-prefix"}
		if !m.actionDisabled("list", "open_url") {
			parts = append(parts, "o: open URL")
		}
		if !m.actionDisabled("list", "refile_project") {
			parts = append(parts, "p: to project")
		}
		quick := []string{}
		if !m.actionDisabled("list", "someday") {
			parts = append(parts, "m: someday")
			quick = append(quick, "m")
		}
		if !m.actionDisabled("list", "done") {
			quick = append(quick, "d")
		}
		if !m.actionDisabled("list", "cancel") {
			quick = append(quick, "c")
		}
		if !m.actionDisabled("list", "waiting") {
			quick = append(quick, "w")
		}
		if len(quick) > 0 {
			parts = append(parts, strings.Join(quick, "/")+": quick states")
		}
		if !m.actionDisabled("list", "archive") {
			parts = append(parts, "A: archive")
		}
		if !m.actionDisabled("list", "trash") {
			parts = append(parts, "x: trash")
		}
		return strings.Join(parts, "  ") + "  " + nav
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
			cmd := m.datePicker.OpenWithOptions(initial, true)
			return m, cmd
		}

	case "backspace", "delete":
		if m.projEditField == projFieldDeadline {
			m.projEditProject.Deadline = nil
			m.statusMsg = "Deadline cleared"
			return m, m.clearStatusAfter()
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

// saveProjectEdit writes the working copy to disk via core.UpdateProject.
func (m Model) saveProjectEdit() tea.Cmd {
	proj := m.projEditProject
	return func() tea.Msg {
		patch := core.ProjectPatch{
			Title:            stringPtr(proj.Title),
			State:            taskStatePtr(proj.State),
			Deadline:         proj.Deadline,
			Tags:             slicePtr(proj.Tags),
			URL:              stringPtr(proj.URL),
			WaitingOn:        stringPtr(proj.WaitingOn),
			DefinitionOfDone: stringPtr(proj.DefinitionOfDone),
		}
		updated, err := m.core.UpdateProject(proj.ID, patch)
		if err != nil {
			return errMsg{err}
		}
		return projectUpdatedMsg{title: updated.Project.Title, newFilename: updated.Filename}
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
				b.WriteString(helpStyle.Render("  (enter: open calendar, del clears)"))
			}
		} else if f == projFieldState {
			b.WriteString(stateStyle.Render(value))
		} else if isDateField {
			b.WriteString(deadlineStyle.Render(value))
			if isSelected {
				b.WriteString(helpStyle.Render("  (enter: open calendar, del clears)"))
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

// runQuery collects tasks and filters them using the given DSL query string.
// Returns a viewResultsLoadedMsg — intended to be returned as a tea.Cmd.
func (m Model) runQuery(name, queryStr string, includeArchived bool) tea.Cmd {
	return func() tea.Msg {
		clauses, err := query.Parse(queryStr, time.Now())
		if err != nil {
			return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, err: err}
		}
		all, err := m.collectViewTasks(name, includeArchived)
		if err != nil {
			return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, err: err}
		}
		var filtered []service.ViewTask
		for _, vt := range all {
			if query.MatchAll(clauses, vt.Task, vt.Source) {
				filtered = append(filtered, vt)
			}
		}
		return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, results: filtered}
	}
}

func (m Model) runFuzzyQuery(name, needle string, includeArchived bool) tea.Cmd {
	return func() tea.Msg {
		all, err := m.collectViewTasks(name, includeArchived)
		if err != nil {
			return viewResultsLoadedMsg{name: name, queryStr: needle, includeArchived: includeArchived, err: err, isFuzzy: true}
		}
		results := search.Rank(all, needle)
		return viewResultsLoadedMsg{name: name, queryStr: needle, includeArchived: includeArchived, results: results, isFuzzy: true}
	}
}

func (m Model) collectViewTasks(name string, includeArchived bool) ([]service.ViewTask, error) {
	if strings.EqualFold(name, "Archives") {
		return m.svc.CollectArchiveTasks()
	}

	all, err := m.svc.CollectAllTasks()
	if err != nil {
		return nil, err
	}
	if !includeArchived {
		return all, nil
	}

	archived, err := m.svc.CollectArchiveTasks()
	if err != nil {
		return nil, err
	}
	return append(all, archived...), nil
}

func weeklyStepTitle(step weeklyReviewStep) string {
	switch step {
	case weeklyStepProjects:
		return "Projects Missing Next Action"
	case weeklyStepWaiting:
		return "Aging Waiting For (7+ days)"
	case weeklyStepSomeday:
		return "Someday / Maybe Triage"
	default:
		return "Recently Archived (7 days)"
	}
}

func (m Model) weeklyTasksForStep(step weeklyReviewStep) []service.ViewTask {
	switch step {
	case weeklyStepWaiting:
		return m.weeklyReviewData.AgingWaitingFor
	case weeklyStepSomeday:
		return m.weeklyReviewData.SomedayMaybe
	case weeklyStepArchived:
		return m.weeklyReviewData.RecentArchived
	default:
		return nil
	}
}

func (m *Model) clampWeeklyCursor() {
	step := m.weeklyReviewStep
	max := 0
	if step == weeklyStepProjects {
		max = len(m.weeklyReviewData.ProjectsWithoutNextAction)
	} else {
		max = len(m.weeklyTasksForStep(step))
	}
	if max == 0 {
		m.weeklyReviewCursors[step] = 0
		return
	}
	if m.weeklyReviewCursors[step] >= max {
		m.weeklyReviewCursors[step] = max - 1
	}
	if m.weeklyReviewCursors[step] < 0 {
		m.weeklyReviewCursors[step] = 0
	}
}

func (m Model) selectedWeeklyTask() (service.ViewTask, bool) {
	tasks := m.weeklyTasksForStep(m.weeklyReviewStep)
	if len(tasks) == 0 {
		return service.ViewTask{}, false
	}
	c := m.weeklyReviewCursors[m.weeklyReviewStep]
	if c < 0 || c >= len(tasks) {
		return service.ViewTask{}, false
	}
	return tasks[c], true
}

func (m Model) weeklyTaskStateChange(newState model.TaskState) tea.Cmd {
	vt, ok := m.selectedWeeklyTask()
	if !ok {
		return nil
	}
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
		return m.loadWeeklyReview()
	}
}

func (m Model) weeklyTaskArchive() tea.Cmd {
	vt, ok := m.selectedWeeklyTask()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		var err error
		if vt.IsProject {
			err = m.svc.ArchiveProjectTask(vt.Filename, vt.SgIdx, vt.Task.ID)
		} else {
			err = m.core.ArchiveTask(vt.Task.ID)
		}
		if err != nil {
			return errMsg{err}
		}
		return m.loadWeeklyReview()
	}
}

func (m Model) weeklyTaskTrash() tea.Cmd {
	vt, ok := m.selectedWeeklyTask()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		var err error
		if vt.IsProject {
			err = m.svc.TrashProjectTask(vt.Filename, vt.SgIdx, vt.Task.ID)
		} else {
			err = m.core.TrashTask(vt.Task.ID)
		}
		if err != nil {
			return errMsg{err}
		}
		return m.loadWeeklyReview()
	}
}

// updateWeeklyReview handles keys in guided weekly review mode.
func (m Model) updateWeeklyReview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.view = viewViews
		return m, nil
	case "h", "left", "p":
		if m.weeklyReviewStep == 0 {
			m.weeklyReviewStep = weeklyStepCount - 1
		} else {
			m.weeklyReviewStep--
		}
		m.clampWeeklyCursor()
		return m, nil
	case "l", "right", "n":
		m.weeklyReviewStep = (m.weeklyReviewStep + 1) % weeklyStepCount
		m.clampWeeklyCursor()
		return m, nil
	case "j", "down":
		m.weeklyReviewCursors[m.weeklyReviewStep]++
		m.clampWeeklyCursor()
		return m, nil
	case "k", "up":
		m.weeklyReviewCursors[m.weeklyReviewStep]--
		m.clampWeeklyCursor()
		return m, nil
	case "g":
		m.weeklyReviewCursors[m.weeklyReviewStep] = 0
		return m, nil
	case "G":
		if m.weeklyReviewStep == weeklyStepProjects {
			if n := len(m.weeklyReviewData.ProjectsWithoutNextAction); n > 0 {
				m.weeklyReviewCursors[m.weeklyReviewStep] = n - 1
			}
		} else {
			if n := len(m.weeklyTasksForStep(m.weeklyReviewStep)); n > 0 {
				m.weeklyReviewCursors[m.weeklyReviewStep] = n - 1
			}
		}
		return m, nil
	case "enter":
		if m.weeklyReviewStep == weeklyStepProjects {
			if len(m.weeklyReviewData.ProjectsWithoutNextAction) == 0 {
				return m, nil
			}
			c := m.weeklyReviewCursors[weeklyStepProjects]
			if c < 0 || c >= len(m.weeklyReviewData.ProjectsWithoutNextAction) {
				return m, nil
			}
			p := m.weeklyReviewData.ProjectsWithoutNextAction[c]
			return m, m.loadProjectDetail(p.Filename)
		}
		vt, ok := m.selectedWeeklyTask()
		if !ok {
			return m, nil
		}
		if vt.ListType == model.ListArchive {
			m.statusMsg = "Archived tasks are read-only"
			return m, m.clearStatusAfter()
		}
		m.detailTask = vt.Task
		m.detailField = detailFieldOrder[0]
		m.detailFromView = viewWeeklyReview
		m.detailFromList = vt.ListType
		m.detailFromSgIdx = vt.SgIdx
		m.detailIsProject = vt.IsProject
		m.view = viewTaskDetail
		return m, nil
	case "R":
		return m, m.loadWeeklyReview
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
		m.view = viewList
		m.currentList = model.ListIn
		m.cursor = 0
		return m, m.loadCurrentList
	}

	if m.weeklyReviewStep == weeklyStepProjects || m.weeklyReviewStep == weeklyStepArchived {
		if m.weeklyReviewStep == weeklyStepProjects {
			return m, nil
		}
		if m.weeklyReviewStep == weeklyStepArchived {
			switch msg.String() {
			case "d", "c", "s", "w", "A", "x":
				m.statusMsg = "Archived tasks are read-only"
				return m, m.clearStatusAfter()
			case "o":
				vt, ok := m.selectedWeeklyTask()
				if !ok {
					return m, nil
				}
				if err := openURLInBrowser(vt.Task.URL); err != nil {
					m.statusMsg = fmt.Sprintf("Error: %v", err)
					return m, m.clearStatusAfter()
				}
				m.statusMsg = "Opened link"
				return m, m.clearStatusAfter()
			}
		}
		return m, nil
	}

	switch msg.String() {
	case "d":
		return m, m.weeklyTaskStateChange(model.StateDone)
	case "c":
		return m, m.weeklyTaskStateChange(model.StateCanceled)
	case "s":
		return m, m.weeklyTaskStateChange(model.StateSomeday)
	case "w":
		return m, m.weeklyTaskStateChange(model.StateWaitingFor)
	case "A":
		return m, m.weeklyTaskArchive()
	case "x":
		return m, m.weeklyTaskTrash()
	case "o":
		vt, ok := m.selectedWeeklyTask()
		if !ok {
			return m, nil
		}
		if err := openURLInBrowser(vt.Task.URL); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			return m, m.clearStatusAfter()
		}
		m.statusMsg = "Opened link"
		return m, m.clearStatusAfter()
	}

	return m, nil
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
			return m, m.runQuery(sv.Name, sv.Query, sv.IncludeArchived)
		}

	case "/":
		// Ad-hoc query: open inline text input.
		m.mode = modeEditingField
		m.input.Reset()
		m.input.Placeholder = "type a query, e.g. state:waiting-for tag:@home"
		m.input.SetValue("")
		cmd := m.input.Focus()
		return m, cmd

	case "?":
		m.mode = modeEditingField
		m.input.Reset()
		m.input.Placeholder = "fuzzy search tasks, tags, notes, projects..."
		m.input.SetValue("")
		cmd := m.input.Focus()
		return m, cmd

	case "W", "w":
		return m, m.loadWeeklyReview

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
			placeholder := m.input.Placeholder
			m.mode = modeNormal
			if queryStr == "" {
				return m, nil
			}
			if strings.Contains(strings.ToLower(placeholder), "fuzzy search") {
				return m, m.runFuzzyQuery("Fuzzy", queryStr, false)
			}
			return m, m.runQuery("Ad-hoc", queryStr, false)
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
	raw := msg.String()
	if m.pendingPrefix != prefixNone {
		switch m.pendingPrefix {
		case prefixState:
			m.cancelPrefix()
			if m.selectedViewResultIsArchived() {
				m.statusMsg = "Archived tasks are read-only"
				return m, m.clearStatusAfter()
			}
			switch raw {
			case "d":
				return m, m.viewResultStateChange(model.StateDone)
			case "c":
				return m, m.viewResultStateChange(model.StateCanceled)
			case "w":
				return m, m.viewResultStateChange(model.StateWaitingFor)
			case "m":
				return m, m.viewResultStateChange(model.StateSomeday)
			default:
				return m, nil
			}
		case prefixTime:
			m.cancelPrefix()
			if raw != "d" && raw != "s" {
				return m, nil
			}
			if len(m.viewResults) == 0 {
				return m, nil
			}
			vt := m.viewResults[m.viewCursor]
			if vt.ListType == model.ListArchive {
				m.statusMsg = "Archived tasks are read-only"
				return m, m.clearStatusAfter()
			}
			m.detailTask = vt.Task
			m.detailFromView = viewViewResults
			m.detailIsProject = vt.IsProject
			if vt.IsProject {
				m.activeFilename = vt.Filename
				m.detailFromSgIdx = vt.SgIdx
			} else {
				m.detailFromList = vt.ListType
			}
			if raw == "d" {
				m.detailField = fieldDeadline
				m.datePickerField = fieldDeadline
			} else {
				m.detailField = fieldScheduled
				m.datePickerField = fieldScheduled
			}
			m.view = viewTaskDetail
			m.mode = modePickingDate
			var initial time.Time
			if m.datePickerField == fieldDeadline && m.detailTask.Deadline != nil {
				initial = *m.detailTask.Deadline
			}
			if m.datePickerField == fieldScheduled && m.detailTask.Scheduled != nil {
				initial = *m.detailTask.Scheduled
			}
			return m, m.datePicker.OpenWithOptions(initial, true)
		default:
			m.cancelPrefix()
		}
	}
	key := m.remapKey("view_results", raw)
	switch key {
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

	case "e":
		if len(m.viewResults) == 0 {
			return m, nil
		}
		vt := m.viewResults[m.viewCursor]
		if vt.ListType == model.ListArchive {
			m.statusMsg = "Archived tasks are read-only"
			return m, m.clearStatusAfter()
		}
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

	case "o":
		if m.actionDisabled("view_results", "open_url") {
			return m, nil
		}
		if len(m.viewResults) == 0 {
			return m, nil
		}
		if err := openURLInBrowser(m.viewResults[m.viewCursor].Task.URL); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			return m, m.clearStatusAfter()
		}
		m.statusMsg = "Opened link"
		return m, m.clearStatusAfter()

	case "R":
		if m.actionDisabled("view_results", "refresh") {
			return m, nil
		}
		// Refresh: re-run the current query.
		return m, m.runQuery(m.activeViewName, m.activeViewQuery, m.activeViewInclA)

	case "d":
		if m.actionDisabled("view_results", "done") {
			return m, nil
		}
		if m.selectedViewResultIsArchived() {
			m.statusMsg = "Archived tasks are read-only"
			return m, m.clearStatusAfter()
		}
		return m, m.viewResultStateChange(model.StateDone)

	case "c":
		if m.actionDisabled("view_results", "cancel") {
			return m, nil
		}
		if m.selectedViewResultIsArchived() {
			m.statusMsg = "Archived tasks are read-only"
			return m, m.clearStatusAfter()
		}
		return m, m.viewResultStateChange(model.StateCanceled)

	case "s":
		return m, m.beginPrefix(prefixState)

	case "m":
		if m.actionDisabled("view_results", "someday") {
			return m, nil
		}
		if m.selectedViewResultIsArchived() {
			m.statusMsg = "Archived tasks are read-only"
			return m, m.clearStatusAfter()
		}
		return m, m.viewResultStateChange(model.StateSomeday)

	case "t":
		return m, m.beginPrefix(prefixTime)

	case "w":
		if m.actionDisabled("view_results", "waiting") {
			return m, nil
		}
		if m.selectedViewResultIsArchived() {
			m.statusMsg = "Archived tasks are read-only"
			return m, m.clearStatusAfter()
		}
		return m, m.viewResultStateChange(model.StateWaitingFor)

	case "x":
		if m.actionDisabled("view_results", "trash") {
			return m, nil
		}
		if m.selectedViewResultIsArchived() {
			m.statusMsg = "Archived tasks are read-only"
			return m, m.clearStatusAfter()
		}
		return m, m.viewResultTrash()

	case "A":
		if m.actionDisabled("view_results", "archive") {
			return m, nil
		}
		if m.selectedViewResultIsArchived() {
			m.statusMsg = "Archived tasks are read-only"
			return m, m.clearStatusAfter()
		}
		return m, m.viewResultArchive()

	case "U":
		if m.actionDisabled("view_results", "restore") {
			return m, nil
		}
		if len(m.viewResults) == 0 {
			return m, nil
		}
		if !m.selectedViewResultIsArchived() {
			m.statusMsg = "Only archived tasks can be restored"
			return m, m.clearStatusAfter()
		}
		return m, m.viewResultRestore()

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

func (m Model) selectedViewResultIsArchived() bool {
	if len(m.viewResults) == 0 || m.viewCursor < 0 || m.viewCursor >= len(m.viewResults) {
		return false
	}
	return m.viewResults[m.viewCursor].ListType == model.ListArchive
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
	includeArchived := m.activeViewInclA
	return func() tea.Msg {
		var err error
		oldState := vt.Task.State
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
		all, err := m.collectViewTasks(name, includeArchived)
		if err != nil {
			return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, err: err}
		}
		var filtered []service.ViewTask
		for _, v := range all {
			if query.MatchAll(clauses, v.Task, v.Source) {
				filtered = append(filtered, v)
			}
		}
		undoApply := func() error {
			if vt.IsProject {
				return m.svc.UpdateProjectTaskState(vt.Filename, vt.SgIdx, vt.Task.ID, oldState)
			}
			return m.svc.UpdateState(vt.ListType, vt.Task.ID, oldState)
		}
		return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, results: filtered, undoApply: undoApply, undoPrompt: "State updated", undoSuccess: fmt.Sprintf("Restored: %s", vt.Task.Text)}
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
	includeArchived := m.activeViewInclA
	return func() tea.Msg {
		var err error
		source := vt.Task.Source
		if source == "" {
			source = vt.Source
		}
		if vt.IsProject {
			err = m.svc.TrashProjectTask(vt.Filename, vt.SgIdx, vt.Task.ID)
		} else {
			err = m.core.TrashTask(vt.Task.ID)
		}
		if err != nil {
			return errMsg{err}
		}
		clauses, _ := query.Parse(queryStr, time.Now())
		all, err2 := m.collectViewTasks(name, includeArchived)
		if err2 != nil {
			return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, err: err2}
		}
		var filtered []service.ViewTask
		for _, v := range all {
			if query.MatchAll(clauses, v.Task, v.Source) {
				filtered = append(filtered, v)
			}
		}
		undoApply := func() error {
			_, err := m.core.RestoreTask(vt.Task.ID)
			return err
		}
		return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, results: filtered, undoApply: undoApply, undoPrompt: "Task trashed", undoSuccess: fmt.Sprintf("Restored: %s", vt.Task.Text)}
	}
}

// viewResultArchive archives the selected view result task.
func (m Model) viewResultArchive() tea.Cmd {
	if len(m.viewResults) == 0 {
		return nil
	}
	vt := m.viewResults[m.viewCursor]
	name := m.activeViewName
	queryStr := m.activeViewQuery
	includeArchived := m.activeViewInclA
	return func() tea.Msg {
		var err error
		if vt.IsProject {
			err = m.svc.ArchiveProjectTask(vt.Filename, vt.SgIdx, vt.Task.ID)
		} else {
			err = m.core.ArchiveTask(vt.Task.ID)
		}
		if err != nil {
			return errMsg{err}
		}
		clauses, _ := query.Parse(queryStr, time.Now())
		all, err2 := m.collectViewTasks(name, includeArchived)
		if err2 != nil {
			return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, err: err2}
		}
		var filtered []service.ViewTask
		for _, v := range all {
			if query.MatchAll(clauses, v.Task, v.Source) {
				filtered = append(filtered, v)
			}
		}
		undoApply := func() error {
			_, err := m.core.RestoreTask(vt.Task.ID)
			return err
		}
		return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, results: filtered, undoApply: undoApply, undoPrompt: "Task archived", undoSuccess: fmt.Sprintf("Restored: %s", vt.Task.Text)}
	}
}

// viewResultRestore restores the selected archived task back to an active location.
func (m Model) viewResultRestore() tea.Cmd {
	if len(m.viewResults) == 0 {
		return nil
	}
	vt := m.viewResults[m.viewCursor]
	name := m.activeViewName
	queryStr := m.activeViewQuery
	includeArchived := m.activeViewInclA
	return func() tea.Msg {
		restored, err := m.core.RestoreTask(vt.Task.ID)
		if err != nil {
			return errMsg{err}
		}
		clauses, _ := query.Parse(queryStr, time.Now())
		all, err2 := m.collectViewTasks(name, includeArchived)
		if err2 != nil {
			return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, err: err2}
		}
		var filtered []service.ViewTask
		for _, v := range all {
			if query.MatchAll(clauses, v.Task, v.Source) {
				filtered = append(filtered, v)
			}
		}
		status := fmt.Sprintf("Restored: %s -> %s", vt.Task.Text, restored.Source)
		return viewResultsLoadedMsg{name: name, queryStr: queryStr, includeArchived: includeArchived, results: filtered, status: status}
	}
}

// sourceBadge returns a short muted badge string for the source of a view task.
func stringPtr(s string) *string {
	return &s
}

func taskStatePtr(s model.TaskState) *model.TaskState {
	return &s
}

func slicePtr[T any](v []T) *[]T {
	cp := append([]T(nil), v...)
	return &cp
}

func sourceBadge(source string) string {
	switch source {
	case "in":
		return "[inbox]"
	case "single-actions":
		return "[actions]"
	case "archive":
		return "[archive]"
	}
	// projects/launch-website.md → [launch-website]
	if after, ok := strings.CutPrefix(source, "projects/"); ok {
		name := strings.TrimSuffix(after, ".md")
		if len(name) > 18 {
			name = name[:15] + "..."
		}
		return "[" + name + "]"
	}
	if after, ok := strings.CutPrefix(source, "archive/"); ok {
		name := strings.TrimSuffix(after, ".md")
		return "[archive:" + name + "]"
	}
	return "[" + source + "]"
}

func openURLInBrowser(url string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("task has no URL")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening URL: %w", err)
	}
	return nil
}

func urlIndicatorMeta(url string) string {
	if strings.TrimSpace(url) == "" {
		return ""
	}
	return tagStyle.Render("🔗")
}

func (m Model) renderWeeklyReview(b *strings.Builder) {
	b.WriteString(projectTitleStyle.Render("Weekly Review"))
	b.WriteString("  ")
	b.WriteString(helpStyle.Render("guided sweep of stale projects, waiting-for, someday, and recent archives"))
	b.WriteString("\n\n")

	sections := []struct {
		step  weeklyReviewStep
		count int
	}{
		{weeklyStepProjects, len(m.weeklyReviewData.ProjectsWithoutNextAction)},
		{weeklyStepWaiting, len(m.weeklyReviewData.AgingWaitingFor)},
		{weeklyStepSomeday, len(m.weeklyReviewData.SomedayMaybe)},
		{weeklyStepArchived, len(m.weeklyReviewData.RecentArchived)},
	}

	for _, s := range sections {
		label := fmt.Sprintf("%s (%d)", weeklyStepTitle(s.step), s.count)
		if m.weeklyReviewStep == s.step {
			b.WriteString(activeTabStyle.Render(" " + label + " "))
		} else {
			b.WriteString(inactiveTabStyle.Render(" " + label + " "))
		}
		b.WriteString(" ")
	}
	b.WriteString("\n\n")

	if m.weeklyReviewStep == weeklyStepProjects {
		if len(m.weeklyReviewData.ProjectsWithoutNextAction) == 0 {
			b.WriteString(taskStyle.Render("No active projects missing a next action."))
			b.WriteString("\n")
			return
		}
		cursor := m.weeklyReviewCursors[weeklyStepProjects]
		start, end := m.visibleRange(len(m.weeklyReviewData.ProjectsWithoutNextAction), cursor)
		for i := start; i < end; i++ {
			p := m.weeklyReviewData.ProjectsWithoutNextAction[i]
			if i == cursor {
				b.WriteString(cursorStyle.Render(" > "))
				b.WriteString(selectedTaskStyle.Render(p.Title))
			} else {
				b.WriteString("   ")
				b.WriteString(taskStyle.Render(p.Title))
			}
			meta := []string{stateStyle.Render(string(p.State)), helpStyle.Render(fmt.Sprintf("%d tasks", p.TaskCount))}
			if p.Deadline != nil {
				meta = append(meta, deadlineStyle.Render("due:"+p.Deadline.Format("2006-01-02")))
			}
			b.WriteString("  ")
			b.WriteString(strings.Join(meta, " "))
			b.WriteString("\n")
		}
		return
	}

	tasks := m.weeklyTasksForStep(m.weeklyReviewStep)
	if len(tasks) == 0 {
		b.WriteString(taskStyle.Render("Nothing to review in this section."))
		b.WriteString("\n")
		return
	}

	cursor := m.weeklyReviewCursors[m.weeklyReviewStep]
	start, end := m.visibleRange(len(tasks), cursor)
	for i := start; i < end; i++ {
		vt := tasks[i]
		isSelected := i == cursor
		if isSelected {
			b.WriteString(cursorStyle.Render(" > "))
		} else {
			b.WriteString("   ")
		}
		checkbox := model.CheckboxFor(vt.Task.State)
		switch checkbox {
		case model.CheckboxDone:
			b.WriteString(checkboxDoneStyle.Render(fmt.Sprintf("[x] %s", vt.Task.Text)))
		case model.CheckboxCanceled:
			b.WriteString(checkboxCanceledStyle.Render(fmt.Sprintf("[-] %s", vt.Task.Text)))
		default:
			if isSelected {
				b.WriteString(checkboxOpenStyle.Render("[ ] "))
				b.WriteString(selectedTaskStyle.Render(vt.Task.Text))
			} else {
				b.WriteString(checkboxOpenStyle.Render("[ ] "))
				b.WriteString(taskStyle.Render(vt.Task.Text))
			}
		}
		var meta []string
		if vt.Task.WaitingSince != nil && m.weeklyReviewStep == weeklyStepWaiting {
			meta = append(meta, helpStyle.Render("since:"+vt.Task.WaitingSince.Format("2006-01-02")))
		}
		if vt.Task.Scheduled != nil {
			meta = append(meta, scheduledStyle.Render("sched:"+formatOptionalTime(vt.Task.Scheduled)))
		}
		if vt.Task.Deadline != nil {
			meta = append(meta, deadlineStyle.Render("due:"+formatOptionalTime(vt.Task.Deadline)))
		}
		if vt.Task.ArchivedAt != nil && m.weeklyReviewStep == weeklyStepArchived {
			meta = append(meta, helpStyle.Render("archived:"+vt.Task.ArchivedAt.Format("2006-01-02")))
		}
		if link := urlIndicatorMeta(vt.Task.URL); link != "" {
			meta = append(meta, link)
		}
		for _, tag := range vt.Task.Tags {
			meta = append(meta, tagStyle.Render(tag))
		}
		meta = append(meta, helpStyle.Render(sourceBadge(vt.Source)))
		if len(meta) > 0 {
			b.WriteString("  ")
			b.WriteString(strings.Join(meta, " "))
		}
		b.WriteString("\n")
	}
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

	start, end := m.visibleRange(len(m.savedViews), m.viewListCursor)
	for i := start; i < end; i++ {
		sv := m.savedViews[i]
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
		label := "  Query: "
		if strings.Contains(strings.ToLower(m.input.Placeholder), "fuzzy search") {
			label = "  Fuzzy: "
		}
		b.WriteString(inputPromptStyle.Render(label))
		b.WriteString(m.input.View())
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  /: ad-hoc query   ?: fuzzy search"))
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
		if m.activeViewFuzzy {
			b.WriteString(helpStyle.Render("fuzzy: " + m.activeViewQuery))
		} else {
			b.WriteString(helpStyle.Render(m.activeViewQuery))
		}
	}
	b.WriteString("\n\n")

	if len(m.viewResults) == 0 {
		b.WriteString(taskStyle.Render("No tasks match this query."))
		b.WriteString("\n")
		return
	}

	start, end := m.visibleRange(len(m.viewResults), m.viewCursor)
	for i := start; i < end; i++ {
		vt := m.viewResults[i]
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
				b.WriteString(checkboxOpenStyle.Render("[ ] "))
				b.WriteString(selectedTaskStyle.Render(task.Text))
			} else {
				b.WriteString(checkboxOpenStyle.Render("[ ] "))
				b.WriteString(taskStyle.Render(task.Text))
			}
		}

		// Inline metadata: state, deadline, tags, source badge.
		var meta []string
		if task.State != model.StateEmpty && task.State != model.StateDone && task.State != model.StateCanceled {
			meta = append(meta, stateStyle.Render(string(task.State)))
		}
		if task.Scheduled != nil {
			meta = append(meta, scheduledStyle.Render("sched:"+formatOptionalTime(task.Scheduled)))
		}
		if task.Deadline != nil {
			meta = append(meta, deadlineStyle.Render("due:"+formatOptionalTime(task.Deadline)))
		}
		if link := urlIndicatorMeta(task.URL); link != "" {
			meta = append(meta, link)
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
func (m Model) contentHeight() int {
	h := m.height - 8 // tab bar, status/help, and spacing
	if m.statusMsg != "" {
		h--
	}
	if m.mode == modeAdding || m.mode == modeAddingProject || m.mode == modeAddingSubGroup || m.mode == modeRenamingSubGroup || m.mode == modeAddingProjectTask {
		h -= 2
	}
	if h < 3 {
		return 3
	}
	return h
}

func (m *Model) ensureScroll(cursor, total int) {
	if total <= 0 {
		m.scrollOffset = 0
		return
	}
	visible := m.contentHeight()
	if visible >= total {
		m.scrollOffset = 0
		return
	}
	if cursor < m.scrollOffset {
		m.scrollOffset = cursor
	}
	if cursor >= m.scrollOffset+visible {
		m.scrollOffset = cursor - visible + 1
	}
	maxOffset := total - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *Model) visibleRange(total, cursor int) (int, int) {
	m.ensureScroll(cursor, total)
	visible := m.contentHeight()
	start := m.scrollOffset
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > total {
		end = total
	}
	if start > end {
		start = end
	}
	return start, end
}

func (m *Model) clampCursor() {
	var max int
	switch m.view {
	case viewProjects:
		max = len(m.projects)
	case viewProjectDetail:
		max = len(m.flattenProject())
		if max == 0 {
			m.projCursor = 0
			m.scrollOffset = 0
			return
		}
		if m.projCursor >= max {
			m.projCursor = max - 1
		}
		if m.projCursor < 0 {
			m.projCursor = 0
		}
		m.ensureScroll(m.projCursor, max)
		return
	case viewViews:
		max = len(m.savedViews)
		if max == 0 {
			m.viewListCursor = 0
			m.scrollOffset = 0
			return
		}
		if m.viewListCursor >= max {
			m.viewListCursor = max - 1
		}
		if m.viewListCursor < 0 {
			m.viewListCursor = 0
		}
		m.ensureScroll(m.viewListCursor, max)
		return
	case viewViewResults:
		max = len(m.viewResults)
		if max == 0 {
			m.viewCursor = 0
			m.scrollOffset = 0
			return
		}
		if m.viewCursor >= max {
			m.viewCursor = max - 1
		}
		if m.viewCursor < 0 {
			m.viewCursor = 0
		}
		m.ensureScroll(m.viewCursor, max)
		return
	case viewWeeklyReview:
		if m.weeklyReviewStep == weeklyStepProjects {
			max = len(m.weeklyReviewData.ProjectsWithoutNextAction)
			if max == 0 {
				m.weeklyReviewCursors[weeklyStepProjects] = 0
				m.scrollOffset = 0
				return
			}
			if m.weeklyReviewCursors[weeklyStepProjects] >= max {
				m.weeklyReviewCursors[weeklyStepProjects] = max - 1
			}
			if m.weeklyReviewCursors[weeklyStepProjects] < 0 {
				m.weeklyReviewCursors[weeklyStepProjects] = 0
			}
			m.ensureScroll(m.weeklyReviewCursors[weeklyStepProjects], max)
			return
		}
		tasks := m.weeklyTasksForStep(m.weeklyReviewStep)
		max = len(tasks)
		if max == 0 {
			m.weeklyReviewCursors[m.weeklyReviewStep] = 0
			m.scrollOffset = 0
			return
		}
		if m.weeklyReviewCursors[m.weeklyReviewStep] >= max {
			m.weeklyReviewCursors[m.weeklyReviewStep] = max - 1
		}
		if m.weeklyReviewCursors[m.weeklyReviewStep] < 0 {
			m.weeklyReviewCursors[m.weeklyReviewStep] = 0
		}
		m.ensureScroll(m.weeklyReviewCursors[m.weeklyReviewStep], max)
		return
	default:
		if m.list == nil {
			max = 0
		} else {
			max = len(m.list.Tasks)
		}
	}

	if max == 0 {
		m.cursor = 0
		m.scrollOffset = 0
		return
	}
	if m.cursor >= max {
		m.cursor = max - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureScroll(m.cursor, max)
}
