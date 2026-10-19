package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// makePanel draws a rounded box of width w and height h with the title set
// into the top border: ╭─ title ──────╮. title is already styled; border
// sets the frame color.
func makePanel(title, content string, w, h int, border lipgloss.Color) string {
	if h < 2 {
		h = 2 // at least a top and bottom border
	}
	inner := w - 4
	if inner < 10 {
		inner = 10
	}

	bs := lipgloss.NewStyle().Foreground(border)
	titleR := " " + title + " "
	fill := inner + 2 - 2 - lipgloss.Width(titleR)
	if fill < 0 {
		fill = 0
	}
	top := bs.Render("╭─") + titleR + bs.Render(strings.Repeat("─", fill)+"─") + bs.Render("╮")

	lines := strings.Split(content, "\n")
	body := h - 2
	if body < 0 {
		body = 0
	}
	for len(lines) < body {
		lines = append(lines, "")
	}
	if len(lines) > body {
		lines = lines[:body]
	}

	sideL := bs.Render("│ ")
	sideR := bs.Render(" │")
	rows := make([]string, 0, h)
	rows = append(rows, top)
	for _, line := range lines {
		pad := inner - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		rows = append(rows, sideL+line+strings.Repeat(" ", pad)+sideR)
	}
	rows = append(rows, bs.Render("╰"+strings.Repeat("─", inner+2)+"╯"))
	return strings.Join(rows, "\n")
}

// truncate shortens s to at most w cells, adding an ASCII "..." ellipsis. It
// drops emoji first and measures by display width, so the result never exceeds
// w cells regardless of the characters involved.
func truncate(s string, w int) string {
	s = sanitize(s)
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 3 {
		return strings.Repeat(".", w) // "", ".", "..", "..."
	}
	budget := w - 3
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > budget {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "..."
}
