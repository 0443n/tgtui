package ui

import (
	"fmt"
	"os"
	"strings"

	tgc "github.com/0443n/tgtui/internal/tg"
	"github.com/charmbracelet/lipgloss"
)

// settingsKeys is the key reference shown at the bottom of the settings screen.
var settingsKeys = []string{
	"j/k, arrows   move            enter         open chat",
	"i, tab        write           [ ]           switch folder",
	"/             search list     s             find in open chat",
	"v             select messages space mark    d delete   f forward",
	"c             contact/profile r/b/d (in c)  rename/block/delete",
	"up (empty)    edit last msg   ctrl+up/dn    pick reply",
	"ctrl+p play   ctrl+s stop     ctrl+x speed",
	"?             this screen     ctrl+q        quit",
}

// settingsView renders the full-screen settings and status overlay. It is
// read-only: it reports the current config and what optional tools are present.
func (m Model) settingsView() string {
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

	conn := "connecting..."
	switch {
	case m.err != nil:
		conn = "error"
	case m.sess != nil:
		conn = "connected"
	}
	proxy := "none (direct)"
	if m.proxyHost != "" {
		proxy = m.proxyHost
	}
	notif := notifierName()
	if notif == "" {
		notif = "none (install notify-send)"
	}
	player := playerName()
	if player == "" {
		player = "none (install mpv or ffplay)"
	}

	lines := []string{
		"  " + stTitle.Render("tgtui") + stDim.Render(" "+tgc.Version),
		"",
		row("connection", conn),
		row("proxy", proxy),
		row("chats", fmt.Sprintf("%d loaded", len(m.chats))),
		row("notifications", notif),
		row("voice player", player),
		row("downloads", os.TempDir()),
		"",
		stDim.Render("  voice/video notes download once, cached as tgtui-* in the"),
		stDim.Render("  folder above and reused on replay. ctrl+p plays the newest"),
		stDim.Render("  one in the open chat."),
		"",
		stBold.Render("  keys"),
	}
	for _, k := range settingsKeys {
		lines = append(lines, stDim.Render("  "+k))
	}
	for i, ln := range lines {
		if lipgloss.Width(ln) > inner {
			lines[i] = truncate(ln, inner)
		}
	}

	panel := makePanel(stTitle.Render("settings"), strings.Join(lines, "\n"), w, h, clrAccent)
	return panel + "\n" + stDim.Render(" esc to close")
}
