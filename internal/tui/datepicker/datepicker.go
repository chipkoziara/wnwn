// Package datepicker provides a calendar-grid date (and optional time) picker
// component for Bubbletea v2 applications.
//
// # Usage
//
//  1. Embed a datepicker.Model in your parent model.
//  2. When the user activates the picker, call Open(initial) to set the
//     initial date and put the picker in the open state.
//  3. Forward all tea.Msg values to Model.Update() while IsOpen() is true.
//  4. After Update, call Result() to check whether the user confirmed or
//     cancelled:
//     - (time.Time, true, false) — confirmed date (with or without time)
//     - (zero,      false, true) — cancelled
//     - (zero,      false, false) — still open, no decision yet
//  5. Call View() to render the picker when it is open.
package datepicker

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Styles ──────────────────────────────────────────────────────────────────

var (
	colorPrimary  = lipgloss.Color("#7D56F4")
	colorMuted    = lipgloss.Color("#626262")
	colorDimText  = lipgloss.Color("#999999")
	colorText     = lipgloss.Color("#FAFAFA")
	colorDanger   = lipgloss.Color("#FF6B6B")
	colorSelected = lipgloss.Color("#7D56F4")

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText)

	dayHeaderStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(4).
			Align(lipgloss.Center)

	dayStyle = lipgloss.NewStyle().
			Width(4).
			Align(lipgloss.Center).
			Foreground(colorDimText)

	selectedDayStyle = lipgloss.NewStyle().
				Width(4).
				Align(lipgloss.Center).
				Foreground(colorText).
				Background(colorSelected).
				Bold(true)

	todayStyle = lipgloss.NewStyle().
			Width(4).
			Align(lipgloss.Center).
			Foreground(colorPrimary).
			Bold(true)

	navStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	timeToggleStyle = lipgloss.NewStyle().
			Foreground(colorDimText).
			Italic(true)

	confirmedStyle = lipgloss.NewStyle().
			Foreground(colorDanger)
)

// ── Model ────────────────────────────────────────────────────────────────────

// Model is the date picker component.
type Model struct {
	// current month/year being displayed
	year  int
	month time.Month

	// selected day within the displayed month
	selectedDay int

	// whether the time input section is visible
	showTime bool

	// text inputs for hour and minute
	hourInput   textinput.Model
	minuteInput textinput.Model
	timeFocus   int // 0 = hour, 1 = minute

	// internal state
	open      bool
	confirmed bool
	cancelled bool
	result    time.Time
}

// New returns a new Model, initially closed.
func New() Model {
	hi := textinput.New()
	hi.Placeholder = "HH"
	hi.CharLimit = 2
	hi.SetWidth(4)
	hi.Validate = validateHour

	mi := textinput.New()
	mi.Placeholder = "MM"
	mi.CharLimit = 2
	mi.SetWidth(4)
	mi.Validate = validateMinute

	return Model{
		hourInput:   hi,
		minuteInput: mi,
	}
}

func validateHour(s string) error {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return fmt.Errorf("digits only")
		}
	}
	if len(s) == 2 {
		n, _ := strconv.Atoi(s)
		if n > 23 {
			return fmt.Errorf("hour 0-23")
		}
	}
	return nil
}

func validateMinute(s string) error {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return fmt.Errorf("digits only")
		}
	}
	if len(s) == 2 {
		n, _ := strconv.Atoi(s)
		if n > 59 {
			return fmt.Errorf("minute 0-59")
		}
	}
	return nil
}

// Open initialises the picker with an existing time (or time.Time{} to default
// to today) and marks it open.
func (m *Model) Open(initial time.Time) tea.Cmd {
	base := initial
	if base.IsZero() {
		base = time.Now()
	}

	m.year = base.Year()
	m.month = base.Month()
	m.selectedDay = base.Day()

	m.showTime = !initial.IsZero() && (initial.Hour() != 0 || initial.Minute() != 0)
	m.confirmed = false
	m.cancelled = false
	m.open = true

	m.hourInput.Reset()
	m.minuteInput.Reset()

	if m.showTime {
		m.hourInput.SetValue(fmt.Sprintf("%02d", initial.Hour()))
		m.minuteInput.SetValue(fmt.Sprintf("%02d", initial.Minute()))
	}

	m.timeFocus = 0
	return m.hourInput.Focus()
}

// IsOpen reports whether the picker is currently active.
func (m Model) IsOpen() bool { return m.open }

// Result returns the picker outcome after an Update cycle:
//   - (t, true,  false) — user confirmed; t is the selected date/time
//   - (_, false, true)  — user cancelled
//   - (_, false, false) — still open, no decision yet
func (m Model) Result() (time.Time, bool, bool) {
	if m.confirmed {
		return m.result, true, false
	}
	if m.cancelled {
		return time.Time{}, false, true
	}
	return time.Time{}, false, false
}

// ── Update ───────────────────────────────────────────────────────────────────

// Update handles messages for the date picker. Returns the updated model and
// any commands.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.open {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Forward non-key messages to the focused time input.
	if m.showTime {
		var cmd tea.Cmd
		if m.timeFocus == 0 {
			m.hourInput, cmd = m.hourInput.Update(msg)
		} else {
			m.minuteInput, cmd = m.minuteInput.Update(msg)
		}
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// Time-input focus steals most keys when visible.
	if m.showTime && (m.timeFocus == 0 && m.hourInput.Focused() || m.timeFocus == 1 && m.minuteInput.Focused()) {
		switch msg.String() {
		case "esc":
			m.open = false
			m.cancelled = true
			return m, nil

		case "enter":
			// Move from hour → minute, or confirm from minute.
			if m.timeFocus == 0 {
				m.timeFocus = 1
				m.hourInput.Blur()
				return m, m.minuteInput.Focus()
			}
			return m.confirm()

		case "tab":
			if m.timeFocus == 0 {
				m.timeFocus = 1
				m.hourInput.Blur()
				return m, m.minuteInput.Focus()
			}
			m.timeFocus = 0
			m.minuteInput.Blur()
			return m, m.hourInput.Focus()

		case "shift+tab":
			if m.timeFocus == 1 {
				m.timeFocus = 0
				m.minuteInput.Blur()
				return m, m.hourInput.Focus()
			}

		case "up", "k":
			// Let arrow keys fall through to calendar when on time inputs
			// only if neither input is actively being edited (i.e. empty/full).
			// For simplicity we handle them in the calendar section below.

		default:
			var cmd tea.Cmd
			if m.timeFocus == 0 {
				m.hourInput, cmd = m.hourInput.Update(msg)
			} else {
				m.minuteInput, cmd = m.minuteInput.Update(msg)
			}
			return m, cmd
		}
	}

	switch msg.String() {
	case "esc":
		m.open = false
		m.cancelled = true

	case "enter":
		return m.confirm()

	case "t":
		m.showTime = !m.showTime
		if m.showTime {
			m.timeFocus = 0
			m.hourInput.Reset()
			m.minuteInput.Reset()
			return m, m.hourInput.Focus()
		}
		m.hourInput.Blur()
		m.minuteInput.Blur()

	case "left", "h":
		m.selectedDay--
		if m.selectedDay < 1 {
			m.month--
			if m.month < 1 {
				m.month = 12
				m.year--
			}
			m.selectedDay = daysInMonth(m.year, m.month)
		}

	case "right", "l":
		max := daysInMonth(m.year, m.month)
		m.selectedDay++
		if m.selectedDay > max {
			m.selectedDay = 1
			m.month++
			if m.month > 12 {
				m.month = 1
				m.year++
			}
		}

	case "up", "k":
		m.selectedDay -= 7
		if m.selectedDay < 1 {
			m.month--
			if m.month < 1 {
				m.month = 12
				m.year--
			}
			m.selectedDay += daysInMonth(m.year, m.month)
		}

	case "down", "j":
		max := daysInMonth(m.year, m.month)
		m.selectedDay += 7
		if m.selectedDay > max {
			m.selectedDay -= max
			m.month++
			if m.month > 12 {
				m.month = 1
				m.year++
			}
		}

	case "<", ",", "[":
		// Previous month.
		m.month--
		if m.month < 1 {
			m.month = 12
			m.year--
		}
		max := daysInMonth(m.year, m.month)
		if m.selectedDay > max {
			m.selectedDay = max
		}

	case ">", ".", "]":
		// Next month.
		m.month++
		if m.month > 12 {
			m.month = 1
			m.year++
		}
		max := daysInMonth(m.year, m.month)
		if m.selectedDay > max {
			m.selectedDay = max
		}
	}

	return m, nil
}

func (m Model) confirm() (Model, tea.Cmd) {
	hour, minute := 0, 0
	if m.showTime {
		if h, err := strconv.Atoi(m.hourInput.Value()); err == nil {
			hour = h
		}
		if min, err := strconv.Atoi(m.minuteInput.Value()); err == nil {
			minute = min
		}
	}
	m.result = time.Date(m.year, m.month, m.selectedDay, hour, minute, 0, 0, time.Local)
	m.confirmed = true
	m.open = false
	return m, nil
}

// ── View ─────────────────────────────────────────────────────────────────────

// View renders the date picker.
func (m Model) View() string {
	if !m.open {
		return ""
	}

	var b strings.Builder

	// Month / year header with nav hints.
	header := fmt.Sprintf("%s %d", m.month.String(), m.year)
	b.WriteString("  ")
	b.WriteString(navStyle.Render("<"))
	b.WriteString("  ")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("  ")
	b.WriteString(navStyle.Render(">"))
	b.WriteString("\n\n")

	// Day-of-week header row.
	days := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	b.WriteString("  ")
	for _, d := range days {
		b.WriteString(dayHeaderStyle.Render(d))
	}
	b.WriteString("\n")

	// Calendar grid.
	today := time.Now()
	firstDay := time.Date(m.year, m.month, 1, 0, 0, 0, 0, time.Local)
	startWd := int(firstDay.Weekday()) // 0=Sun
	totalDays := daysInMonth(m.year, m.month)

	cell := 0
	b.WriteString("  ")
	// Leading blanks.
	for ; cell < startWd; cell++ {
		b.WriteString(dayStyle.Render(""))
	}

	for day := 1; day <= totalDays; day++ {
		isSelected := day == m.selectedDay
		isToday := day == today.Day() && m.month == today.Month() && m.year == today.Year()

		var rendered string
		label := fmt.Sprintf("%d", day)
		switch {
		case isSelected:
			rendered = selectedDayStyle.Render(label)
		case isToday:
			rendered = todayStyle.Render(label)
		default:
			rendered = dayStyle.Render(label)
		}
		b.WriteString(rendered)
		cell++

		if cell%7 == 0 && day < totalDays {
			b.WriteString("\n  ")
		}
	}
	b.WriteString("\n")

	// Time toggle / input row.
	b.WriteString("\n")
	if !m.showTime {
		b.WriteString("  ")
		b.WriteString(timeToggleStyle.Render("t: add time"))
		b.WriteString("\n")
	} else {
		b.WriteString("  ")
		b.WriteString(labelStyle.Render("Time: "))
		b.WriteString(m.hourInput.View())
		b.WriteString(labelStyle.Render(" : "))
		b.WriteString(m.minuteInput.View())
		b.WriteString("  ")
		b.WriteString(timeToggleStyle.Render("(t: remove time)"))
		b.WriteString("\n")
	}

	// Selected date summary + help.
	b.WriteString("\n  ")
	summary := fmt.Sprintf("%s %d, %d", m.month.String(), m.selectedDay, m.year)
	if m.showTime {
		h := m.hourInput.Value()
		min := m.minuteInput.Value()
		if h == "" {
			h = "00"
		}
		if min == "" {
			min = "00"
		}
		summary += fmt.Sprintf(" at %s:%s", h, min)
	}
	b.WriteString(confirmedStyle.Render(summary))
	b.WriteString("\n")

	b.WriteString("\n  ")
	b.WriteString(timeToggleStyle.Render("arrows/hjkl: move  </> month  enter: confirm  esc: cancel"))
	b.WriteString("\n")

	return b.String()
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
}
