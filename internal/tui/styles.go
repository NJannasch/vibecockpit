package tui

import "github.com/charmbracelet/lipgloss"

var (
	purple    = lipgloss.Color("99")
	green     = lipgloss.Color("46")
	cyan      = lipgloss.Color("39")
	white     = lipgloss.Color("15")
	gray      = lipgloss.Color("242")
	darkGray  = lipgloss.Color("236")
	lightGray = lipgloss.Color("250")

	logoStyle = lipgloss.NewStyle().
			Foreground(purple).
			Bold(true)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(gray)

	headerBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(purple).
			Padding(0, 2).
			MarginBottom(1)

	columnHeaderStyle = lipgloss.NewStyle().
				Foreground(lightGray).
				Bold(true)

	selectedRowStyle = lipgloss.NewStyle().
				Background(darkGray).
				Bold(true)

	normalRowStyle = lipgloss.NewStyle()

	activeIndicator = lipgloss.NewStyle().
			Foreground(green).
			Render("●")

	cursorIndicator = lipgloss.NewStyle().
			Foreground(purple).
			Bold(true).
			Render("▸")

	dimText = lipgloss.NewStyle().
		Foreground(gray)

	filterPrompt = lipgloss.NewStyle().
			Foreground(cyan).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(gray)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lightGray).
			Bold(true)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(gray).
			MarginTop(1)

	newProjectStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cyan).
			Padding(1, 2).
			Width(60)

	settingsStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(purple).
			Padding(1, 2).
			Width(40)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)
)
