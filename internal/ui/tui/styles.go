package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorGreen  = lipgloss.Color("#22c55e")
	colorRed    = lipgloss.Color("#ef4444")
	colorYellow = lipgloss.Color("#eab308")
	colorBlue   = lipgloss.Color("#3b82f6")
	colorDim    = lipgloss.Color("#6b7280")
	colorWhite  = lipgloss.Color("#f9fafb")

	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWhite)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBlue).
			MarginTop(1)

	readyStyle = lipgloss.NewStyle().
			Foreground(colorGreen)

	failedStyle = lipgloss.NewStyle().
			Foreground(colorRed)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	dimStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	activeStyle = lipgloss.NewStyle().
			Foreground(colorWhite).
			Bold(true)

	progressBarFull  = lipgloss.NewStyle().Foreground(colorGreen)
	progressBarEmpty = lipgloss.NewStyle().Foreground(colorDim)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			MarginTop(1)
)

const (
	checkMark  = "[OK]"
	crossMark  = "[!!]"
	spinner    = "[..]"
	pending    = "[  ]"
	warnMark   = "[??]"
)
