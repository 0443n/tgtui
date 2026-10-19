package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// contactView renders the full-screen contact/profile overlay for the open
// user chat: name, presence, username, phone, common chats, contact state, and
// bio, plus an inline rename that reuses the compose input.
func (m Model) contactView() string {
	w := m.width
	h := m.height - 1
	if h < 6 {
		h = 6
	}
	inner := innerWidth(w)

	row := func(k, v string) string {
		pad := 15 - len(k)
		if pad < 1 {
			pad = 1
		}
		return "  " + stBold.Render(k) + strings.Repeat(" ", pad) + stFg.Render(v)
	}

	var lines []string
	switch {
	case m.contactErr != "":
		lines = append(lines, "  "+stError.Render(m.contactErr))
	case m.contact == nil:
		lines = append(lines, "  "+stDim.Render("loading..."))
	default:
		p := m.contact
		lines = append(lines, "  "+stTitle.Render(sanitize(p.Name)))
		if p.Status != "" {
			lines = append(lines, "  "+stDim.Render(p.Status))
		}
		lines = append(lines, "")
		if p.Username != "" {
			lines = append(lines, row("username", "@"+p.Username))
		}
		if p.Phone != "" {
			lines = append(lines, row("phone", "+"+p.Phone))
		}
		if p.Common > 0 {
			lines = append(lines, row("common chats", fmt.Sprintf("%d", p.Common)))
		}
		state := "not a contact"
		if p.Contact {
			state = "in contacts"
		}
		if p.Blocked {
			state += ", blocked"
		}
		lines = append(lines, row("contact", state))
		if p.Bio != "" {
			lines = append(lines, "", stBold.Render("  bio"))
			for _, bl := range wrap(p.Bio, inner-2) {
				lines = append(lines, "  "+stFg.Render(bl))
			}
		}
	}

	for i, ln := range lines {
		if lipgloss.Width(ln) > inner {
			lines[i] = truncate(ln, inner)
		}
	}
	if m.renaming {
		lines = append(lines, "", stBold.Render("  new name:")+" "+m.input.View())
	}

	panel := makePanel(stTitle.Render("contact"), strings.Join(lines, "\n"), w, h, clrAccent)

	foot, footStyle := m.contactFooter()
	return panel + "\n" + footStyle.Render(foot)
}

// contactFooter picks the overlay's bottom line: a confirm prompt, the rename
// hint, or the list of actions (adapting labels to the contact's state).
func (m Model) contactFooter() (string, lipgloss.Style) {
	switch {
	case m.contactConfirm == "block":
		return " block this user? y / n", stError
	case m.contactConfirm == "unblock":
		return " unblock this user? y / n", stError
	case m.contactConfirm == "delete":
		return " delete from contacts? y / n", stError
	case m.renaming:
		return " enter save   esc cancel", stDim
	}
	rename, block := "r rename", "b block"
	if m.contact != nil && !m.contact.Contact {
		rename = "r save as contact"
	}
	if m.contact != nil && m.contact.Blocked {
		block = "b unblock"
	}
	parts := []string{rename, block}
	if m.contact != nil && m.contact.Contact {
		parts = append(parts, "d delete contact")
	}
	parts = append(parts, "esc close")
	return " " + strings.Join(parts, "   "), stDim
}
