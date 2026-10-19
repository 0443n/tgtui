package ui

import "github.com/charmbracelet/lipgloss"

// Palette: soft dark-blue, matching the max3m look.
var (
	clrFg      = lipgloss.Color("#E5EEF8")
	clrDim     = lipgloss.Color("#526173")
	clrMid     = lipgloss.Color("#93A4B8")
	clrBorder  = lipgloss.Color("#253549")
	clrAccent  = lipgloss.Color("#58A9FF")
	clrPeer    = lipgloss.Color("#8BA9CB")
	clrRed     = lipgloss.Color("#DE7566")
	clrSuccess = lipgloss.Color("#85C98D")

	stFg      = lipgloss.NewStyle().Foreground(clrFg)
	stDim     = lipgloss.NewStyle().Foreground(clrDim)
	stMid     = lipgloss.NewStyle().Foreground(clrMid)
	stBold    = lipgloss.NewStyle().Foreground(clrFg).Bold(true)
	stTitle   = lipgloss.NewStyle().Foreground(clrAccent).Bold(true)
	stAccent  = lipgloss.NewStyle().Foreground(clrAccent).Bold(true)
	stPeer    = lipgloss.NewStyle().Foreground(clrPeer).Bold(true)
	stError   = lipgloss.NewStyle().Foreground(clrRed).Bold(true)
	stSuccess = lipgloss.NewStyle().Foreground(clrSuccess).Bold(true)
)
