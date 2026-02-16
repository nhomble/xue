package main

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorPrimary   = lipgloss.Color("#7C3AED") // violet
	colorSecondary = lipgloss.Color("#6366F1") // indigo
	colorSuccess   = lipgloss.Color("#10B981") // green
	colorWarning   = lipgloss.Color("#F59E0B") // amber
	colorDanger    = lipgloss.Color("#EF4444") // red
	colorMuted     = lipgloss.Color("#6B7280") // gray
	colorText      = lipgloss.Color("#E5E7EB") // light gray
	colorBg        = lipgloss.Color("#111827") // dark bg
	colorBgPanel   = lipgloss.Color("#1F2937") // panel bg
	colorBorder    = lipgloss.Color("#374151") // border

	// Layout styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			PaddingLeft(1)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	activePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(0, 1)

	listItemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	selectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(1).
				Foreground(colorPrimary).
				Bold(true)

	statusRunning = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)

	statusExited = lipgloss.NewStyle().
			Foreground(colorMuted)

	statusStopped = lipgloss.NewStyle().
			Foreground(colorWarning)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			PaddingLeft(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText).
			PaddingLeft(1).
			PaddingBottom(1)

	inputLabelStyle = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorDanger).
			Bold(true)

	tagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E5E7EB")).
			Background(lipgloss.Color("#374151")).
			Padding(0, 1)

	metaLabelStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Bold(true)
)

func threadStatusStyle(status ThreadStatus) lipgloss.Style {
	switch status {
	case ThreadRunning:
		return statusRunning
	case ThreadExited:
		return statusExited
	case ThreadStopped:
		return statusStopped
	default:
		return mutedStyle
	}
}
