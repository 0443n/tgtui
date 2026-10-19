package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// searchView renders the in-chat message search overlay: a query line on top,
// then the matching messages (newest first) with a cursor to jump to one.
func (m Model) searchView() string {
	w := m.width
	h := m.height - 1
	if h < 6 {
		h = 6
	}
	inner := innerWidth(w)

	cursor := ""
	if m.searchTyping {
		cursor = "_"
	}
	lines := []string{
		"  " + stBold.Render("find: ") + stFg.Render(sanitize(m.chatQuery)) + stAccent.Render(cursor),
		"",
	}

	switch {
	case m.searchBusy:
		lines = append(lines, "  "+stDim.Render("searching..."))
	case m.searchErr != "":
		lines = append(lines, "  "+stDim.Render(m.searchErr))
	case len(m.hits) == 0:
		lines = append(lines, "  "+stDim.Render("type a query and press enter"))
	default:
		bodyH := h - 4 // panel body minus the query line and the blank line
		if bodyH < 1 {
			bodyH = 1
		}
		var rows []string
		for i, hit := range m.hits {
			cur, style := "  ", stFg
			if i == m.hitSel && !m.searchTyping {
				cur, style = "> ", stAccent
			}
			date := ""
			if t := msgTime(hit.Date); !t.IsZero() {
				date = t.Format("2 Jan 15:04")
			}
			text := strings.ReplaceAll(sanitize(hit.Text), "\n", " ")
			line := cur + date + " " + m.selWho(hit) + ": " + text
			rows = append(rows, style.Render(truncate(line, inner)))
		}
		lines = append(lines, windowRows(rows, m.hitSel, bodyH)...)
	}

	for i, ln := range lines {
		if lipgloss.Width(ln) > inner {
			lines[i] = truncate(ln, inner)
		}
	}

	title := truncate("search in "+m.openedChat.Title, inner-4)
	panel := makePanel(stTitle.Render(title), strings.Join(lines, "\n"), w, h, clrAccent)

	foot := " enter search   esc close"
	if !m.searchTyping && len(m.hits) > 0 {
		foot = " j/k move   enter jump   i edit   esc close"
	}
	return panel + "\n" + stDim.Render(foot)
}
