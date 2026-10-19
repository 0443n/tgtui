package ui

import (
	"fmt"
	"strings"
	"time"

	tgc "github.com/0443n/tgtui/internal/tg"
	"github.com/charmbracelet/lipgloss"
)

// layout splits the screen into a left (list) and right (chat) panel.
func (m Model) layout() (leftW, rightW, panelH, msgH, maxBubble int) {
	leftW = 34
	if leftW > m.width/2 {
		leftW = m.width / 2
	}
	if leftW < 18 {
		leftW = 18
	}
	rightW = m.width - leftW
	if rightW < 24 {
		rightW = 24
		leftW = m.width - rightW
		if leftW < 8 {
			leftW = 8
		}
	}

	panelH = m.height - 1 // footer line
	if panelH < 4 {
		panelH = 4
	}
	msgH = panelH - 4 // body minus a blank line and the input line
	if msgH < 1 {
		msgH = 1
	}

	rightInner := rightW - 4
	maxBubble = rightInner - 10
	if maxBubble > 72 {
		maxBubble = 72
	}
	if maxBubble < 16 {
		maxBubble = 16
	}
	return
}

// innerWidth is the usable content width inside a panel of outer width w.
func innerWidth(w int) int {
	iw := w - 4
	if iw < 10 {
		iw = 10
	}
	return iw
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "starting..."
	}
	if m.err != nil {
		return stTitle.Render("tgtui") + "\n\n" +
			stError.Render(m.err.Error()) + "\n\n" +
			stDim.Render("q or ctrl+q to quit")
	}
	if m.showSettings {
		return m.settingsView()
	}
	if m.contactOpen {
		return m.contactView()
	}
	if m.searchOpen {
		return m.searchView()
	}

	leftW, rightW, panelH, msgH, maxBubble := m.layout()

	left := makePanel(m.leftTitle(), m.renderList(leftW, panelH-2), leftW, panelH, m.border(focusList))
	right := makePanel(m.rightTitle(), m.renderRight(rightW, msgH, maxBubble), rightW, panelH, m.border(focusInput))

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return body + "\n" + m.footer()
}

func (m Model) border(f focus) lipgloss.Color {
	if m.focus == f {
		return clrAccent
	}
	return clrBorder
}

func (m Model) leftTitle() string {
	if m.searching || m.filter != "" {
		cursor := ""
		if m.searching {
			cursor = "_"
		}
		return stAccent.Render("search ") + stFg.Render(sanitize(m.filter)) + stAccent.Render(cursor)
	}
	if len(m.folders) > 1 {
		f := m.folders[m.folderIdx]
		return stTitle.Render(sanitize(f.Title)) + stDim.Render(fmt.Sprintf(" %d/%d", m.folderIdx+1, len(m.folders)))
	}
	return stTitle.Render("tgtui")
}

func (m Model) rightTitle() string {
	if !m.openedOK {
		return stTitle.Render("no chat")
	}
	t := stTitle.Render(sanitize(m.openedChat.Title))
	if m.focus == focusSelect {
		return t + stDim.Render(" - ") + stAccent.Render("select")
	}
	if s := m.typingStatus(m.openedChat); s != "" {
		return t + stDim.Render(" - ") + stSuccess.Render(s)
	}
	if m.openedChat.Status != "" {
		style := stDim
		if m.openedChat.Online {
			style = stSuccess
		}
		return t + stDim.Render(" - ") + style.Render(m.openedChat.Status)
	}
	return t
}

// typingStatus returns a live typing line for a chat, or "". In a DM the title
// is already the person, so just "typing..."; groups name the typer.
func (m Model) typingStatus(ch tgc.Dialog) string {
	until, ok := m.typingUntil[ch.PeerID]
	if !ok || !time.Now().Before(until) {
		return ""
	}
	if name := m.typingName[ch.PeerID]; name != "" && !ch.IsUser {
		return name + " is typing..."
	}
	return "typing..."
}

func (m Model) footer() string {
	text, style := m.footerText()
	if m.width > 0 {
		text = truncate(text, m.width)
	}
	return style.Render(text)
}

// footerText picks the footer content: a transient flash, live playback
// controls, or the key hints (with proxy status appended).
func (m Model) footerText() (string, lipgloss.Style) {
	// Modal prompts win over a lingering flash so the user always sees them.
	if m.confirmDelete {
		return " delete: e everyone   m just me   esc cancel", stError
	}
	if m.forwarding {
		if m.searching {
			return " type to filter   enter forward here   esc cancel", stDim
		}
		return " forwarding: j/k pick   enter forward   / search   [ ] folder   esc cancel", stDim
	}
	if m.flash != "" {
		if m.flashErr {
			return " " + m.flash, stError
		}
		return " " + m.flash, stDim
	}
	if m.focus == focusSelect {
		return " j/k move   space mark   d delete   f forward   g/G ends   esc back", stDim
	}
	if m.searching {
		return " type to filter   enter select   esc cancel", stDim
	}
	if playing, label, speed := audio.status(); playing {
		s := " playing " + label
		if speed != 1 {
			s += " (" + speedLabel(speed) + "x)"
		}
		return s + "   ctrl+s stop   ctrl+x speed", stSuccess
	}
	var keys string
	switch {
	case m.focus == focusInput:
		keys = " enter send   up edit   ctrl+up reply   ctrl+p play   esc back"
	case len(m.folders) > 1:
		keys = " j/k move   enter open   [ ] folder   / search   i write   ? help   q quit"
	default:
		keys = " j/k move   enter open   / search   i write   ? help   q quit"
	}
	if m.openedOK && m.focus == focusList {
		keys += "   s find   v select"
		if m.openedChat.IsUser && !m.openedChat.IsSelf {
			keys += "   c contact"
		}
	}
	return keys + m.proxyTag(), stDim
}

// proxyTag is the proxy-status suffix for the footer, or "".
func (m Model) proxyTag() string {
	if m.proxyHost == "" {
		return ""
	}
	return "   proxy: " + m.proxyHost
}

// ── Left pane: chat list ────────────────────────────────────────────────────

func (m Model) renderList(w, bodyH int) string {
	cw := w - 4 // content width inside the panel
	if cw < 6 {
		cw = 6
	}
	chats := m.visible()
	if len(chats) == 0 {
		switch {
		case m.status != "" && len(m.chats) == 0:
			return stDim.Render(" " + m.status)
		case m.filter != "":
			return stDim.Render(" (no matches)")
		default:
			return stDim.Render(" (empty folder)")
		}
	}

	var rows []string
	selRow := 0
	for i, ch := range chats {
		marker := stDim.Render(" ")
		titleStyle := stBold
		previewStyle := stMid
		if i == m.selected {
			marker = stAccent.Render(">")
			titleStyle = stAccent
			previewStyle = stFg
			selRow = len(rows)
		}

		ts := ""
		if ch.Date > 0 {
			ts = stMid.Render(fmtTime(ch.Date))
		}
		unread := ""
		if ch.Unread > 0 {
			unread = " " + stAccent.Render(fmt.Sprintf("(%d)", ch.Unread))
		}
		dot := ""
		if ch.Online {
			dot = stSuccess.Render("* ")
		}
		pin := ""
		if ch.Pinned {
			pin = stDim.Render("* ") // pinned marker, kept next to the time
		}
		right := pin + ts
		avail := cw - lipgloss.Width(marker) - 2 - lipgloss.Width(dot) - lipgloss.Width(unread) - lipgloss.Width(right) - 1
		if avail < 1 {
			avail = 1
		}
		left := marker + "  " + dot + titleStyle.Render(truncate(ch.Title, avail)) + unread
		gap := cw - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 1 {
			gap = 1
		}
		rows = append(rows, left+strings.Repeat(" ", gap)+right)

		indent := strings.Repeat(" ", lipgloss.Width(marker)+2)
		if preview := strings.ReplaceAll(ch.Preview, "\n", " "); preview != "" {
			rows = append(rows, previewStyle.Render(indent+truncate(preview, cw-lipgloss.Width(indent))))
		}
		rows = append(rows, "")
	}

	// Window the rows so the selected chat stays visible.
	if bodyH > 0 && len(rows) > bodyH {
		top := selRow - bodyH/2
		if top < 0 {
			top = 0
		}
		if top > len(rows)-bodyH {
			top = len(rows) - bodyH
		}
		rows = rows[top : top+bodyH]
	}
	return strings.Join(rows, "\n")
}

// ── Right pane: open chat ───────────────────────────────────────────────────

func (m Model) renderRight(w, msgH, maxBubble int) string {
	if m.focus == focusSelect {
		return m.renderSelectPane(w, msgH)
	}
	var out []string
	if !m.openedOK {
		out = append(out, stDim.Render(" select a chat and press enter"))
	} else {
		lines := m.buildLines(maxBubble, innerWidth(w))
		total := len(lines)
		maxScroll := total - msgH
		if maxScroll < 0 {
			maxScroll = 0
		}
		scroll := m.scroll
		if scroll > maxScroll {
			scroll = maxScroll
		}
		switch {
		case scroll == 0 && total > msgH:
			out = lines[total-msgH:]
		case scroll == 0:
			out = lines
		default:
			end := total - scroll
			start := end - msgH
			if start < 0 {
				start = 0
			}
			out = lines[start:end]
		}
		if scroll > 0 {
			out = append(out, stDim.Render(fmt.Sprintf(" %d more above", scroll)))
		}
	}

	// Pad the message area, then a blank line and the input.
	for len(out) < msgH {
		out = append(out, "")
	}
	if len(out) > msgH {
		out = out[:msgH]
	}
	if m.openedOK && !m.openedChat.CanPost {
		out = append(out, "", stDim.Render(" you can't post in this channel"))
	} else {
		out = append(out, m.composeBanner(innerWidth(w)), m.input.View())
	}
	return strings.Join(out, "\n")
}

// renderSelectPane draws message select mode: one line per message with a
// cursor (>) and mark (x) column, windowed to keep the cursor on screen. It
// returns msgH message lines plus two footer-area lines, matching renderRight's
// height so the panel stays the same size.
func (m Model) renderSelectPane(w, msgH int) string {
	innerW := innerWidth(w)
	var rows []string
	for i, msg := range m.messages {
		cur, style := "  ", stFg
		if i == m.selCursor {
			cur, style = "> ", stAccent
		}
		mark := " "
		if msg.ID > 0 && m.marked[msg.ID] {
			mark = "x"
		}
		// Sanitize the text first so truncate's own sanitize finds no emoji and
		// leaves the marker-column spacing intact.
		text := strings.ReplaceAll(sanitize(msg.Text), "\n", " ")
		line := cur + mark + " " + fmtTime(msg.Date) + " " + m.selWho(msg) + ": " + text
		rows = append(rows, style.Render(truncate(line, innerW)))
	}
	if len(rows) == 0 {
		rows = []string{stDim.Render(" no messages")}
	}
	rows = windowRows(rows, m.selCursor, msgH)
	for len(rows) < msgH {
		rows = append(rows, "")
	}
	if len(rows) > msgH {
		rows = rows[:msgH]
	}
	return strings.Join(append(rows, "", m.selectStatus()), "\n")
}

// selectStatus is the pane's bottom line in select mode: the marked count.
func (m Model) selectStatus() string {
	n := 0
	for _, v := range m.marked {
		if v {
			n++
		}
	}
	if n > 0 {
		return stAccent.Render(fmt.Sprintf(" %d marked", n))
	}
	return stDim.Render(" select mode")
}

// selWho is the sender label for a message in select mode.
func (m Model) selWho(msg tgc.Message) string {
	if msg.Out || m.openedChat.IsSelf {
		return "you"
	}
	if msg.Sender != "" {
		return sanitize(msg.Sender)
	}
	return sanitize(m.openedChat.Title)
}

// windowRows returns the slice of rows of length h that keeps cursor visible,
// centred where possible.
func windowRows(rows []string, cursor, h int) []string {
	if h <= 0 || len(rows) <= h {
		return rows
	}
	top := cursor - h/2
	if top < 0 {
		top = 0
	}
	if top > len(rows)-h {
		top = len(rows) - h
	}
	return rows[top : top+h]
}

// composeBanner is the line above the input: an edit or reply hint, else blank.
func (m Model) composeBanner(width int) string {
	switch {
	case m.editing != 0:
		return stDim.Render(" editing (esc to cancel)")
	case m.msgSel >= 0 && m.msgSel < len(m.messages):
		t := m.messages[m.msgSel]
		who := t.Sender
		if t.Out {
			who = "you"
		}
		if who == "" {
			who = m.openedChat.Title
		}
		snippet := strings.ReplaceAll(t.Text, "\n", " ")
		return stDim.Render(truncate(" replying to "+who+": "+snippet, width))
	default:
		return ""
	}
}

// buildLines renders messages into display lines: date separators, sender
// groups with a "HH:MM name:" header, and indented wrapped bodies.
func (m Model) buildLines(maxBubble, innerW int) []string {
	var lines []string
	if m.loadingOlder {
		lines = append(lines, stDim.Render(" loading..."))
	}

	lastDate := ""
	for start := 0; start < len(m.messages); {
		if ts := msgTime(m.messages[start].Date); !ts.IsZero() {
			if date := ts.Format("2 Jan 2006"); date != lastDate {
				lastDate = date
				lines = append(lines, dateSep(ts, innerW))
			}
		}

		end := start + 1
		for end < len(m.messages) && sameGroup(m.messages[end-1], m.messages[end]) {
			end++
		}
		lines = append(lines, m.renderGroup(m.messages[start:end], maxBubble)...)
		if end < len(m.messages) {
			lines = append(lines, "")
		}
		start = end
	}
	return lines
}

func (m Model) renderGroup(group []tgc.Message, maxWidth int) []string {
	first := group[0]
	ts := fmtTime(first.Date)
	indent := strings.Repeat(" ", lipgloss.Width(" "+ts+"  "))

	header := " " + stDim.Render(ts) + "  " + m.senderLabel(first, maxWidth) + ":"
	if st := m.sendStatus(group[len(group)-1]); st != "" {
		header += " " + st
	}
	lines := []string{header, ""}
	for i, msg := range group {
		if msg.ReplyToID != 0 {
			lines = append(lines, stDim.Render(indent+replyQuote(msg, maxWidth)))
		}
		for _, ln := range wrap(msg.Text, maxWidth) {
			lines = append(lines, indent+ln)
		}
		if i < len(group)-1 {
			lines = append(lines, stDim.Render(indent+"."))
		}
	}
	return lines
}

// sendStatus marks one of our messages "[unread]" until the peer reads it,
// then shows nothing. Text-only, so it works on CLI-only terminals.
func (m Model) sendStatus(msg tgc.Message) string {
	if !msg.Out {
		return ""
	}
	if msg.ID == 0 || msg.ID > m.openedChat.ReadOutboxMaxID {
		return stDim.Render("[unread]")
	}
	return ""
}

// replyQuote is the dim "> sender: snippet" line shown above a reply.
func replyQuote(msg tgc.Message, width int) string {
	label := "> "
	switch {
	case msg.ReplyToSender != "" && msg.ReplyToText != "":
		label += msg.ReplyToSender + ": " + strings.ReplaceAll(msg.ReplyToText, "\n", " ")
	case msg.ReplyToText != "":
		label += strings.ReplaceAll(msg.ReplyToText, "\n", " ")
	default:
		label += "reply"
	}
	return truncate(label, width)
}

func (m Model) senderLabel(msg tgc.Message, width int) string {
	if msg.Out || m.openedChat.IsSelf {
		return stAccent.Render("you")
	}
	name := msg.Sender
	if name == "" && m.openedOK {
		name = m.openedChat.Title
	}
	return stPeer.Render(truncate(name, width))
}

// ── helpers ─────────────────────────────────────────────────────────────────

func sameGroup(a, b tgc.Message) bool {
	if a.Out != b.Out || a.Sender != b.Sender {
		return false
	}
	if a.Date == 0 || b.Date == 0 {
		return true
	}
	d := a.Date - b.Date
	if d < 0 {
		d = -d
	}
	return d <= 5*60
}

func dateSep(ts time.Time, width int) string {
	label := ts.Format("2 Jan 2006")
	now := time.Now()
	switch {
	case sameDay(ts, now):
		label = "Today"
	case sameDay(ts, now.AddDate(0, 0, -1)):
		label = "Yesterday"
	}
	total := width - len(label) - 2
	if total < 2 {
		total = 2
	}
	left := total / 2
	return stDim.Render(strings.Repeat("─", left) + " " + label + " " + strings.Repeat("─", total-left))
}

func fmtTime(ts int) string {
	if ts == 0 {
		return "     "
	}
	return time.Unix(int64(ts), 0).Format("15:04")
}

func msgTime(ts int) time.Time {
	if ts == 0 {
		return time.Time{}
	}
	return time.Unix(int64(ts), 0)
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// isEmoji reports whether r is an emoji or other pictographic rune whose
// terminal width is unreliable and would break the fixed-width layout.
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF: // pictographs, emoticons, transport, symbols
		return true
	case r >= 0x2600 && r <= 0x27BF: // misc symbols + dingbats
		return true
	case r >= 0x2B00 && r <= 0x2BFF: // misc symbols and arrows (stars, etc.)
		return true
	case r >= 0x2300 && r <= 0x23FF: // misc technical (watch, hourglass, keycaps)
		return true
	case r >= 0x1F1E6 && r <= 0x1F1FF: // regional indicators (flags)
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
		return true
	case r == 0x200D || r == 0x20E3: // zero-width joiner, combining keycap
		return true
	}
	return false
}

// sanitize drops emoji from display text so widths stay predictable. Text that
// was only emoji becomes "[emoji]" so it isn't shown blank.
func sanitize(s string) string {
	has := false
	for _, r := range s {
		if isEmoji(r) {
			has = true
			break
		}
	}
	if !has {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !isEmoji(r) {
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if out == "" {
		return "[emoji]"
	}
	return out
}

func wrap(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	words := strings.Fields(sanitize(s))
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := ""
	curW := 0
	for _, word := range words {
		ww := lipgloss.Width(word)
		switch {
		case cur == "":
			cur, curW = word, ww
		case curW+1+ww <= w:
			cur += " " + word
			curW += 1 + ww
		default:
			lines = append(lines, cur)
			cur, curW = word, ww
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}
