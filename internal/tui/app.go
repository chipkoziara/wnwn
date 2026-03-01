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
)

// Model is the top-level Bubbletea model for the application.
type Model struct {
	svc         *service.Service
	store       *store.Store
	list        *model.TaskList
	currentList model.ListType
	cursor      int
	mode        mode
	input       textinput.Model
	width       int
	height      int
	statusMsg   string // temporary status message (e.g. "Task refiled")
	err         error
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
		return m, tea.Batch(m.loadCurrentList, m.clearStatusAfter())

	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyPressMsg:
		// Clear any status message on keypress.
		m.statusMsg = ""

		if m.mode == modeAdding {
			return m.updateAdding(msg)
		}
		return m.updateNormal(msg)
	}

	// Pass through to text input if in adding mode.
	if m.mode == modeAdding {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
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
		if m.currentList != model.ListIn {
			m.currentList = model.ListIn
			m.cursor = 0
			return m, m.loadCurrentList
		}

	case "2":
		if m.currentList != model.ListSingleActions {
			m.currentList = model.ListSingleActions
			m.cursor = 0
			return m, m.loadCurrentList
		}

	case "tab":
		if m.currentList == model.ListIn {
			m.currentList = model.ListSingleActions
		} else {
			m.currentList = model.ListIn
		}
		m.cursor = 0
		return m, m.loadCurrentList

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

	if m.list == nil {
		v.SetContent("\n  Loading...")
		return v
	}

	var b strings.Builder

	// Tab bar.
	b.WriteString(m.renderTabBar())
	b.WriteString("\n\n")

	// Task list.
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

	// Input area (when adding).
	if m.mode == modeAdding {
		b.WriteString("\n")
		b.WriteString(inputPromptStyle.Render("  New task: "))
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

	taskCount := 0
	if m.list != nil {
		taskCount = len(m.list.Tasks)
	}

	if m.currentList == model.ListIn {
		return activeTabStyle.Render(fmt.Sprintf("%s(%d)", inboxLabel, taskCount)) +
			"  " + inactiveTabStyle.Render(actionsLabel)
	}
	return inactiveTabStyle.Render(inboxLabel) +
		"  " + activeTabStyle.Render(fmt.Sprintf("%s(%d)", actionsLabel, taskCount))
}

// helpText returns contextual help based on mode and current list.
func (m Model) helpText() string {
	if m.mode == modeAdding {
		return "enter: save  esc: cancel"
	}

	base := "j/k: navigate  g/G: top/bottom  tab: switch list"

	if m.currentList == model.ListIn {
		return "a: add  r: refile  s: someday  w: waiting  d: done  x: trash  " + base
	}
	return "s: someday  w: waiting  d: done  x: trash  " + base + "  q: quit"
}

// clampCursor ensures the cursor is within bounds.
func (m *Model) clampCursor() {
	if m.list == nil || len(m.list.Tasks) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(m.list.Tasks) {
		m.cursor = len(m.list.Tasks) - 1
	}
}
