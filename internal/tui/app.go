// Package tui implements the Bubbletea terminal user interface for g-tuddy.
package tui

import (
	"fmt"
	"strings"

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
	svc    *service.Service
	store  *store.Store
	list   *model.TaskList
	cursor int
	mode   mode
	input  textinput.Model
	width  int
	height int
	err    error
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
		svc:   svc,
		store: s,
		input: ti,
	}
}

// Init loads the inbox on startup.
func (m Model) Init() tea.Cmd {
	return m.loadInbox
}

// loadInbox is a tea.Cmd that reads the inbox from disk.
func (m Model) loadInbox() tea.Msg {
	if err := m.store.Init(); err != nil {
		return errMsg{err}
	}
	list, err := m.store.ReadList(model.ListIn)
	if err != nil {
		return errMsg{err}
	}
	return listLoadedMsg{list}
}

// Messages
type listLoadedMsg struct{ list *model.TaskList }
type taskAddedMsg struct{ task *model.Task }
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
		// Reload the list to pick up the new task.
		return m, m.loadInbox

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyPressMsg:
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

// updateNormal handles keys in normal (browsing) mode.
func (m Model) updateNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

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

	case "a":
		m.mode = modeAdding
		m.input.Reset()
		cmd := m.input.Focus()
		return m, cmd

	case "d":
		// Mark done.
		if m.list != nil && len(m.list.Tasks) > 0 {
			task := m.list.Tasks[m.cursor]
			_ = m.svc.UpdateState(m.list.Type, task.ID, model.StateDone)
			return m, m.loadInbox
		}

	case "x":
		// Trash task.
		if m.list != nil && len(m.list.Tasks) > 0 {
			task := m.list.Tasks[m.cursor]
			_ = m.svc.TrashTask(m.list.Type, task.ID)
			return m, m.loadInbox
		}
	}

	return m, nil
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

	// Title bar.
	title := titleStyle.Render(fmt.Sprintf(" Inbox (%d)", len(m.list.Tasks)))
	b.WriteString(title)
	b.WriteString("\n\n")

	// Task list.
	if len(m.list.Tasks) == 0 {
		b.WriteString(taskStyle.Render("No tasks in the inbox. Press 'a' to add one."))
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

	// Status bar / help.
	b.WriteString("\n")
	if m.mode == modeAdding {
		b.WriteString(helpStyle.Render("  enter: save  esc: cancel"))
	} else {
		b.WriteString(helpStyle.Render("  a: add  d: done  x: trash  j/k: navigate  g/G: top/bottom  q: quit"))
	}
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
