package tui

import "github.com/charmbracelet/lipgloss"

// Shared styles used by every rel TUI screen.
var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	counterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	filterLabel    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	filterText     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231"))
	filterHint     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	cursorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	checkedStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	uncheckedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	activeItem     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231"))
	normalItem     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	matchStyle     = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("214"))
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	warnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	noteStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	cursorBlock    = lipgloss.NewStyle().Reverse(true)
)
