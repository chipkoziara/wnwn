package tui

import "charm.land/lipgloss/v2"

// Color palette.
var (
	colorPrimary   = lipgloss.Color("#7D56F4")
	colorSecondary = lipgloss.Color("#874BFD")
	colorMuted     = lipgloss.Color("#626262")
	colorSuccess   = lipgloss.Color("#73F59F")
	colorDanger    = lipgloss.Color("#FF6B6B")
	colorText      = lipgloss.Color("#FAFAFA")
	colorDimText   = lipgloss.Color("#999999")
)

// Styles used throughout the TUI.
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText).
			Background(colorPrimary).
			Padding(0, 1)

	taskStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	selectedTaskStyle = lipgloss.NewStyle().
				PaddingLeft(1).
				Foreground(colorPrimary).
				Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	checkboxOpenStyle = lipgloss.NewStyle().
				Foreground(colorDimText)

	checkboxDoneStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Strikethrough(true)

	checkboxCanceledStyle = lipgloss.NewStyle().
				Foreground(colorDanger).
				Strikethrough(true)

	tagStyle = lipgloss.NewStyle().
			Foreground(colorSecondary)

	deadlineStyle = lipgloss.NewStyle().
			Foreground(colorDanger)

	stateStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorDimText).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	inputPromptStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)
)
