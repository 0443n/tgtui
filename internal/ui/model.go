package ui

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	tgc "github.com/0443n/tgtui/internal/tg"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gotd/td/tg"
)

type focus int

const (
	focusList   focus = iota
	focusInput        // typing a message
	focusSelect       // picking messages to delete or forward
)

// Model is the whole app state.
type Model struct {
	width, height int

	ctx  context.Context
	sess *tgc.Session

	chats     []tgc.Dialog
	folders   []tgc.Folder
	folderIdx int
	selected  int // cursor into the visible (folder-filtered) list

	openedChat   tgc.Dialog
	openedOK     bool
	messages     []tgc.Message
	scroll       int // lines scrolled up from the newest message
	loadingOlder bool

	typingName  map[int64]string
	typingUntil map[int64]time.Time

	input   textinput.Model
	focus   focus
	msgSel  int // reply-target message index, -1 = none
	editing int // id of the message being edited, 0 = none

	searching bool   // typing into the chat-list search filter
	filter    string // chat-list search query, "" = no filter

	// Message select mode (focusSelect): a cursor over m.messages plus a set
	// of marked message ids to delete or forward.
	selCursor     int
	marked        map[int]bool
	confirmDelete bool // awaiting the e/m/esc choice for a delete

	// Forwarding: after picking messages, the list becomes a destination
	// picker; these hold what to forward and from where.
	forwarding  bool
	forwardIDs  []int
	forwardFrom tg.InputPeerClass

	showSettings bool // the settings/status overlay is up

	// Contact overlay: a user's profile, with an inline rename that reuses the
	// compose input.
	contactOpen    bool
	contact        *tgc.Profile
	contactErr     string
	renaming       bool
	contactConfirm string // pending confirm: "block", "unblock", or "delete"

	// In-chat message search overlay.
	searchOpen   bool
	searchTyping bool // editing the query vs navigating results
	searchBusy   bool // waiting on the server
	chatQuery    string
	searchErr    string
	hits         []tgc.Message
	hitSel       int

	proxyHost string // sanitised proxy host, "" if none

	status     string // connection state, shown while the list is empty
	flash      string // transient message shown in the footer
	flashErr   bool
	flashUntil time.Time
	err        error
}

// New builds the starting model.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "type a message..."
	ti.Prompt = "  "
	ti.PromptStyle = stDim
	ti.PlaceholderStyle = stDim
	ti.TextStyle = stFg
	ti.Cursor.Style = stAccent

	return Model{
		input:       ti,
		focus:       focusList,
		msgSel:      -1,
		marked:      map[int]bool{},
		status:      "connecting...",
		typingName:  map[int64]string{},
		typingUntil: map[int64]time.Time{},
	}
}

// WithProxy records the proxy so its status can be shown. Credentials are
// dropped; only host:port is kept.
func (m Model) WithProxy(raw string) Model {
	if raw == "" {
		return m
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		m.proxyHost = u.Host
	} else {
		m.proxyHost = "on"
	}
	return m
}

type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) Init() tea.Cmd { return tea.Batch(textinput.Blink, tick()) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeInput()
		return m, nil

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			return m, m.scrollUp(3)
		case tea.MouseButtonWheelDown:
			m.scrollDown(3)
		}
		return m, nil

	case tickMsg:
		if m.flash != "" && time.Now().After(m.flashUntil) {
			m.flash = ""
		}
		return m, tick()

	case ConnectedMsg:
		m.ctx, m.sess = msg.Ctx, msg.Session
		m.status = "loading chats..."
		return m, loadDialogs(m.ctx, m.sess)

	case FatalMsg:
		m.err = msg.Err
		m.status = "error"
		return m, nil

	case dialogsMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.chats = msg.dialogs
		m.status = "ready"
		m.rebuildFolders(nil)
		cmds := []tea.Cmd{loadFolders(m.ctx, m.sess)}
		if len(m.visible()) > 0 {
			var c tea.Cmd
			m, c = m.open(0, false)
			cmds = append(cmds, c)
		}
		return m, tea.Batch(cmds...)

	case foldersMsg:
		if msg.err == nil {
			m.rebuildFolders(msg.folders)
		}
		return m, nil

	case TypingMsg:
		m.typingName[msg.ChatID] = msg.Name
		m.typingUntil[msg.ChatID] = time.Now().Add(6 * time.Second)
		return m, nil

	case StatusMsg:
		for i := range m.chats {
			if m.chats[i].IsUser && !m.chats[i].IsSelf && m.chats[i].PeerID == msg.UserID {
				m.chats[i].Status = msg.Status
				m.chats[i].Online = msg.Online
			}
		}
		if m.openedOK && m.openedChat.PeerID == msg.UserID && !m.openedChat.IsSelf {
			m.openedChat.Status = msg.Status
			m.openedChat.Online = msg.Online
		}
		return m, nil

	case IncomingMsg:
		return m.handleIncoming(msg)

	case ReadMsg:
		for i := range m.chats {
			if m.chats[i].PeerID == msg.ChatID {
				m.chats[i].Unread = msg.Unread
			}
		}
		if m.openedOK && m.openedChat.PeerID == msg.ChatID {
			m.openedChat.Unread = msg.Unread
		}
		return m, nil

	case OutboxReadMsg:
		for i := range m.chats {
			if m.chats[i].PeerID == msg.ChatID && msg.MaxID > m.chats[i].ReadOutboxMaxID {
				m.chats[i].ReadOutboxMaxID = msg.MaxID
			}
		}
		if m.openedOK && m.openedChat.PeerID == msg.ChatID && msg.MaxID > m.openedChat.ReadOutboxMaxID {
			m.openedChat.ReadOutboxMaxID = msg.MaxID
		}
		return m, nil

	case editedMsg:
		if msg.err != nil {
			m.setFlash("edit failed: "+firstLine(msg.err.Error()), true)
			return m, nil
		}
		for i := range m.messages {
			if m.messages[i].ID == msg.id {
				m.messages[i].Text = msg.text
				m.messages[i].Media = false
			}
		}
		if m.openedOK && len(m.messages) > 0 && m.messages[len(m.messages)-1].ID == msg.id {
			m.bumpPreview(m.openedChat.PeerID, msg.text, 0, false)
		}
		return m, nil

	case playbackMsg:
		switch {
		case msg.err != nil:
			m.setFlash("cannot play: "+firstLine(msg.err.Error()), true)
		case msg.speed != 0:
			m.setFlash("speed "+speedLabel(msg.speed)+"x", false)
		}
		return m, nil

	case historyMsg:
		if msg.err != nil {
			m.setFlash(firstLine(msg.err.Error()), true)
			return m, nil
		}
		if !m.openedOK || msg.chatID != m.openedChat.PeerID {
			return m, nil // stale: the user moved on to another chat
		}
		m.messages = msg.msgs
		m.scroll = 0
		return m, nil

	case olderMsg:
		m.loadingOlder = false
		if msg.err != nil {
			m.setFlash(firstLine(msg.err.Error()), true)
			return m, nil
		}
		if !m.openedOK || msg.chatID != m.openedChat.PeerID {
			return m, nil
		}
		m.prependOlder(msg.msgs)
		return m, nil

	case sentMsg:
		// The optimistic copy is added on submit; here we only surface errors.
		if msg.err != nil {
			m.setFlash("send failed: "+firstLine(msg.err.Error()), true)
		}
		return m, nil

	case deletedMsg:
		if msg.err != nil {
			m.setFlash("delete failed: "+firstLine(msg.err.Error()), true)
		} else {
			m.setFlash(fmt.Sprintf("deleted %d", msg.n), false)
		}
		return m, nil

	case forwardedMsg:
		if msg.err != nil {
			m.setFlash("forward failed: "+firstLine(msg.err.Error()), true)
		} else {
			m.setFlash(fmt.Sprintf("forwarded %d to %s", msg.n, sanitize(msg.title)), false)
		}
		return m, nil

	case profileMsg:
		if msg.err != nil {
			m.contactErr = firstLine(msg.err.Error())
		} else {
			p := msg.p
			m.contact = &p
		}
		return m, nil

	case renamedMsg:
		if msg.err != nil {
			m.setFlash("rename failed: "+firstLine(msg.err.Error()), true)
			return m, nil
		}
		if msg.name != "" {
			m.openedChat.Title = msg.name
			for i := range m.chats {
				if m.chats[i].PeerID == m.openedChat.PeerID {
					m.chats[i].Title = msg.name
				}
			}
		}
		m.setFlash("saved", false)
		if m.contactOpen {
			return m, loadProfile(m.ctx, m.sess, m.openedChat.Peer)
		}
		return m, nil

	case blockedMsg:
		if msg.err != nil {
			m.setFlash("block failed: "+firstLine(msg.err.Error()), true)
			return m, nil
		}
		if m.contact != nil {
			m.contact.Blocked = msg.blocked
		}
		if msg.blocked {
			m.setFlash("blocked", false)
		} else {
			m.setFlash("unblocked", false)
		}
		return m, nil

	case contactDeletedMsg:
		if msg.err != nil {
			m.setFlash("delete failed: "+firstLine(msg.err.Error()), true)
			return m, nil
		}
		m.setFlash("contact deleted", false)
		if m.contactOpen {
			return m, loadProfile(m.ctx, m.sess, m.openedChat.Peer)
		}
		return m, nil

	case searchMsg:
		m.searchBusy = false
		if msg.err != nil {
			m.searchErr = firstLine(msg.err.Error())
			return m, nil
		}
		m.hits = msg.hits
		m.hitSel = 0
		m.searchTyping = false
		if len(m.hits) == 0 {
			m.searchErr = "no matches"
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.focus == focusInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The settings overlay swallows keys: ctrl+q still quits, anything else
	// closes it.
	if m.showSettings {
		if msg.String() == "ctrl+q" {
			return m, tea.Quit
		}
		m.showSettings = false
		return m, nil
	}

	// The contact overlay is modal too, but handles its own keys (rename etc).
	if m.contactOpen {
		return m.handleContactKey(msg)
	}

	// So is the in-chat search overlay.
	if m.searchOpen {
		return m.handleSearchOverlayKey(msg)
	}

	// Scrolling and quit work in either focus.
	switch msg.String() {
	case "ctrl+q":
		return m, tea.Quit
	case "pgup", "shift+up":
		return m, m.scrollUp(3)
	case "pgdown", "shift+down":
		m.scrollDown(3)
		return m, nil
	// Audio controls use ctrl so they work while typing in a chat too.
	case "ctrl+p":
		return m.play()
	case "ctrl+s":
		return m, stopVoice()
	case "ctrl+x":
		return m, speedVoice()
	}

	switch m.focus {
	case focusInput:
		return m.handleInputKey(msg)
	case focusSelect:
		return m.handleSelectKey(msg)
	default:
		return m.handleListKey(msg)
	}
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searching {
		return m.handleSearchKey(msg)
	}
	if m.forwarding {
		return m.handleForwardKey(msg)
	}
	vis := m.visible()
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "/":
		m.searching = true
		m.filter = ""
		m.selected = 0
		return m, nil
	case "?":
		m.showSettings = true
		return m, nil
	case "v":
		if m.openedOK && len(m.messages) > 0 {
			m.focus = focusSelect
			m.selCursor = len(m.messages) - 1
			m.marked = map[int]bool{}
			m.confirmDelete = false
		}
		return m, nil
	case "c":
		if m.openedOK && m.openedChat.IsUser && !m.openedChat.IsSelf {
			m.contactOpen = true
			m.contact = nil
			m.contactErr = ""
			return m, loadProfile(m.ctx, m.sess, m.openedChat.Peer)
		}
		return m, nil
	case "s":
		if m.openedOK {
			m.searchOpen = true
			m.searchTyping = true
			m.searchBusy = false
			m.chatQuery = ""
			m.searchErr = ""
			m.hits = nil
			m.hitSel = 0
		}
		return m, nil
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.selected = 0
		}
		return m, nil
	case "j", "down":
		if m.selected < len(vis)-1 {
			m.selected++
			return m.preview(m.selected)
		}
		return m, nil
	case "k", "up":
		if m.selected > 0 {
			m.selected--
			return m.preview(m.selected)
		}
		return m, nil
	case "]":
		return m.previewSelected(m.switchFolder(1))
	case "[":
		return m.previewSelected(m.switchFolder(-1))
	case "enter":
		if len(vis) == 0 {
			return m, nil
		}
		return m.open(m.selected, true)
	case "i", "tab":
		if len(vis) == 0 {
			return m, nil
		}
		m, cmd := m.open(m.selected, true)
		if !m.openedChat.CanPost {
			m.setFlash("you can't post in this channel", false)
			return m, cmd
		}
		m.focus = focusInput
		return m, tea.Batch(cmd, m.input.Focus())
	}
	return m, nil
}

// handleSearchKey drives the chat-list search filter while the user is typing
// it. Enter keeps the filter and opens the top match; esc drops it.
func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searching = false
		m.filter = ""
		m.selected = 0
		return m, nil
	case "enter":
		m.searching = false
		if len(m.visible()) == 0 {
			return m, nil
		}
		if m.forwarding {
			return m.doForward(m.selected)
		}
		return m.open(m.selected, false)
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
		m.selected = 0
		return m, nil
	}
	if (msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace) && !msg.Alt {
		m.filter += string(msg.Runes)
		m.selected = 0
	}
	return m, nil
}

// handleSelectKey drives message select mode: a cursor over the open chat's
// messages, space to mark, d to delete, f to forward.
func (m Model) handleSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirmDelete {
		switch msg.String() {
		case "e":
			return m.doDelete(true)
		case "m":
			return m.doDelete(false)
		case "esc":
			m.confirmDelete = false
		}
		return m, nil
	}
	switch msg.String() {
	case "esc", "v":
		m.focus = focusList
		m.marked = map[int]bool{}
		return m, nil
	case "j", "down":
		if m.selCursor < len(m.messages)-1 {
			m.selCursor++
		}
		return m, nil
	case "k", "up":
		if m.selCursor > 0 {
			m.selCursor--
		}
		return m, nil
	case "g":
		m.selCursor = 0
		return m, nil
	case "G":
		m.selCursor = len(m.messages) - 1
		return m, nil
	case " ": // space: mark the current message, then step down
		if m.selCursor >= 0 && m.selCursor < len(m.messages) {
			if id := m.messages[m.selCursor].ID; id > 0 {
				m.marked[id] = !m.marked[id]
			}
		}
		if m.selCursor < len(m.messages)-1 {
			m.selCursor++
		}
		return m, nil
	case "d":
		if len(m.actionIDs()) == 0 {
			m.setFlash("nothing to delete (message not sent yet?)", false)
			return m, nil
		}
		m.confirmDelete = true
		return m, nil
	case "f":
		ids := m.actionIDs()
		if len(ids) == 0 {
			m.setFlash("nothing to forward (message not sent yet?)", false)
			return m, nil
		}
		m.forwardIDs = ids
		m.forwardFrom = m.openedChat.Peer
		m.forwarding = true
		m.focus = focusList
		m.filter = ""
		m.searching = false
		return m, nil
	}
	return m, nil
}

// handleForwardKey navigates the destination picker: the chat list, where enter
// forwards to the highlighted chat instead of opening it.
func (m Model) handleForwardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	vis := m.visible()
	switch msg.String() {
	case "esc":
		m.forwarding = false
		m.forwardIDs = nil
		m.forwardFrom = nil
		m.setFlash("forward cancelled", false)
		return m, nil
	case "enter":
		return m.doForward(m.selected)
	case "/":
		m.searching = true
		m.filter = ""
		m.selected = 0
		return m, nil
	case "j", "down":
		if m.selected < len(vis)-1 {
			m.selected++
		}
		return m, nil
	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}
		return m, nil
	case "]":
		return m.switchFolder(1), nil
	case "[":
		return m.switchFolder(-1), nil
	}
	return m, nil
}

// handleContactKey drives the contact overlay: r renames (reusing the compose
// input), esc/c/q closes it. ctrl+q still quits.
func (m Model) handleContactKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+q" {
		return m, tea.Quit
	}
	if m.renaming {
		switch msg.String() {
		case "enter":
			first, last := splitName(m.input.Value())
			m.renaming = false
			m.input.Reset()
			m.input.Blur()
			if first == "" {
				m.setFlash("name can't be empty", false)
				return m, nil
			}
			return m, renameContact(m.ctx, m.sess, m.openedChat.Peer, first, last)
		case "esc":
			m.renaming = false
			m.input.Reset()
			m.input.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	if m.contactConfirm != "" {
		peer := m.openedChat.Peer
		action := m.contactConfirm
		switch msg.String() {
		case "y":
			m.contactConfirm = ""
			switch action {
			case "block":
				return m, setBlocked(m.ctx, m.sess, peer, true)
			case "unblock":
				return m, setBlocked(m.ctx, m.sess, peer, false)
			case "delete":
				return m, deleteContact(m.ctx, m.sess, peer)
			}
		case "n", "esc":
			m.contactConfirm = ""
		}
		return m, nil
	}
	switch msg.String() {
	case "esc", "c", "q":
		m.contactOpen = false
		m.contact = nil
		m.contactErr = ""
		m.input.Reset()
		m.input.Blur()
		return m, nil
	case "r":
		cur := ""
		if m.contact != nil {
			cur = m.contact.Name
		}
		m.renaming = true
		m.input.SetValue(cur)
		m.input.CursorEnd()
		return m, m.input.Focus()
	case "b":
		if m.contact != nil && m.contact.Blocked {
			m.contactConfirm = "unblock"
		} else {
			m.contactConfirm = "block"
		}
		return m, nil
	case "d":
		if m.contact != nil && m.contact.Contact {
			m.contactConfirm = "delete"
		} else {
			m.setFlash("not a contact", false)
		}
		return m, nil
	}
	return m, nil
}

// splitName splits a full name into first and last on the first space.
func splitName(s string) (first, last string) {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// handleSearchOverlayKey drives in-chat search: type a query and enter to run
// it, then j/k over the hits and enter to jump to one in the history.
func (m Model) handleSearchOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+q" {
		return m, tea.Quit
	}
	if m.searchTyping {
		switch msg.String() {
		case "esc":
			m.searchOpen = false
			return m, nil
		case "enter":
			q := strings.TrimSpace(m.chatQuery)
			if q == "" {
				return m, nil
			}
			m.searchBusy = true
			m.searchErr = ""
			return m, searchChat(m.ctx, m.sess, m.openedChat.Peer, q)
		case "backspace":
			if r := []rune(m.chatQuery); len(r) > 0 {
				m.chatQuery = string(r[:len(r)-1])
			}
			return m, nil
		}
		if (msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace) && !msg.Alt {
			m.chatQuery += string(msg.Runes)
		}
		return m, nil
	}
	switch msg.String() {
	case "esc", "s":
		m.searchOpen = false
		return m, nil
	case "i", "/":
		m.searchTyping = true
		return m, nil
	case "j", "down":
		if m.hitSel < len(m.hits)-1 {
			m.hitSel++
		}
		return m, nil
	case "k", "up":
		if m.hitSel > 0 {
			m.hitSel--
		}
		return m, nil
	case "enter":
		if len(m.hits) > 0 && m.hitSel < len(m.hits) {
			hit := m.hits[m.hitSel]
			m.searchOpen = false
			m.setFlash("jumped to match", false)
			return m, jumpTo(m.ctx, m.sess, m.openedChat.Peer, m.openedChat.PeerID, hit.ID+1)
		}
		return m, nil
	}
	return m, nil
}

// actionIDs returns the message ids to act on: the marked set (in order) if any
// are marked, otherwise the message under the cursor. Optimistic (id 0)
// messages are skipped since the server can't act on them.
func (m Model) actionIDs() []int {
	var ids []int
	if len(m.marked) > 0 {
		for _, mm := range m.messages {
			if mm.ID > 0 && m.marked[mm.ID] {
				ids = append(ids, mm.ID)
			}
		}
		if len(ids) > 0 {
			return ids
		}
	}
	if m.selCursor >= 0 && m.selCursor < len(m.messages) {
		if id := m.messages[m.selCursor].ID; id > 0 {
			return []int{id}
		}
	}
	return nil
}

// doDelete removes the chosen messages locally and on the server. revoke=true
// deletes for everyone; false only on our side.
func (m Model) doDelete(revoke bool) (tea.Model, tea.Cmd) {
	ids := m.actionIDs()
	m.confirmDelete = false
	if len(ids) == 0 {
		return m, nil
	}
	peer := m.openedChat.Peer
	del := make(map[int]bool, len(ids))
	for _, id := range ids {
		del[id] = true
	}
	var kept []tgc.Message
	for _, mm := range m.messages {
		if mm.ID > 0 && del[mm.ID] {
			continue
		}
		kept = append(kept, mm)
	}
	m.messages = kept
	m.marked = map[int]bool{}
	if m.selCursor >= len(m.messages) {
		m.selCursor = len(m.messages) - 1
	}
	if len(m.messages) == 0 {
		m.focus = focusList
	}
	// Refresh the list preview if we deleted the newest message.
	if m.openedOK {
		last := ""
		if n := len(m.messages); n > 0 {
			last = m.messages[n-1].Text
		}
		m.bumpPreview(m.openedChat.PeerID, last, 0, false)
	}
	return m, deleteMessages(m.ctx, m.sess, peer, ids, revoke)
}

// doForward sends the pending messages to the chat at idx in the visible list.
func (m Model) doForward(idx int) (tea.Model, tea.Cmd) {
	vis := m.visible()
	if idx < 0 || idx >= len(vis) {
		return m, nil
	}
	to := vis[idx]
	ids := m.forwardIDs
	from := m.forwardFrom
	m.forwarding = false
	m.forwardIDs = nil
	m.forwardFrom = nil
	m.filter = ""
	m.searching = false
	return m, forwardMessages(m.ctx, m.sess, from, to.Peer, ids, to.Title)
}

// preview loads a chat into the right pane without marking it read.
func (m Model) preview(idx int) (tea.Model, tea.Cmd) {
	return m.open(idx, false)
}

// previewSelected shows the current selection after a folder switch.
func (m Model) previewSelected(m2 Model) (tea.Model, tea.Cmd) {
	if len(m2.visible()) == 0 {
		m2.openedOK = false
		return m2, nil
	}
	return m2.open(m2.selected, false)
}

func (m Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel edit or reply mode first; leave the input only once neither is on.
		switch {
		case m.editing != 0:
			m.editing = 0
			m.input.Reset()
		case m.msgSel >= 0:
			m.msgSel = -1
		default:
			m.focus = focusList
			m.input.Blur()
		}
		return m, nil
	case "up": // empty input: edit your last message
		if m.input.Value() == "" && m.editing == 0 {
			m.startEdit()
			return m, nil
		}
	case "ctrl+up":
		m.moveReplyTarget(-1)
		return m, nil
	case "ctrl+down":
		m.moveReplyTarget(1)
		return m, nil
	case "enter":
		return m.submitInput()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// submitInput sends, replies, or applies an edit, depending on the mode.
func (m Model) submitInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	peer := m.openedPeer()
	if peer == nil || !m.openedChat.CanPost || text == "" {
		return m, nil
	}
	if m.editing != 0 {
		id := m.editing
		m.editing = 0
		m.input.Reset()
		return m, editMessage(m.ctx, m.sess, peer, id, text)
	}
	// Build the optimistic copy now, carrying the reply so it shows a quote
	// immediately instead of appearing as a plain message until reload.
	opt := tgc.Message{Out: true, Text: text, Date: int(time.Now().Unix())}
	replyTo := 0
	if m.msgSel >= 0 && m.msgSel < len(m.messages) {
		t := m.messages[m.msgSel]
		replyTo = t.ID
		opt.ReplyToID = t.ID
		opt.ReplyToText = t.Text
		if t.Out {
			opt.ReplyToSender = "you"
		} else {
			opt.ReplyToSender = t.Sender
		}
	}
	m.msgSel = -1
	m.input.Reset()
	m.appendLive(opt)
	m.scroll = 0
	if m.openedOK {
		m.bumpPreview(m.openedChat.PeerID, text, opt.Date, false)
		m.reorder()
	}
	return m, sendMessage(m.ctx, m.sess, peer, text, replyTo)
}

// startEdit loads our most recent editable message into the input.
func (m *Model) startEdit() {
	for i := len(m.messages) - 1; i >= 0; i-- {
		mm := m.messages[i]
		if mm.Out && mm.ID > 0 && !mm.Media {
			m.editing = mm.ID
			m.msgSel = -1
			m.input.SetValue(mm.Text)
			m.input.CursorEnd()
			return
		}
	}
	m.setFlash("no message of yours to edit", false)
}

// moveReplyTarget walks the reply cursor: from none, up (-1) targets the newest;
// further up goes older, down (+1) newer, and past the newest cancels the reply.
func (m *Model) moveReplyTarget(dir int) {
	if len(m.messages) == 0 {
		return
	}
	m.editing = 0
	if m.msgSel < 0 {
		if dir < 0 {
			m.msgSel = len(m.messages) - 1
		}
		return
	}
	m.msgSel += dir
	switch {
	case m.msgSel < 0:
		m.msgSel = 0
	case m.msgSel >= len(m.messages):
		m.msgSel = -1 // past the newest: cancel reply
	}
}

// play starts playback of the newest voice/video note in the open chat. It
// only works while a chat is open.
func (m Model) play() (tea.Model, tea.Cmd) {
	if !m.openedOK {
		m.setFlash("open a chat to listen", false)
		return m, nil
	}
	for i := len(m.messages) - 1; i >= 0; i-- {
		if mm := m.messages[i]; mm.Voice {
			label := strings.Trim(mm.Text, "[]")
			return m, playVoice(m.ctx, m.sess, m.openedChat.PeerID, mm, label)
		}
	}
	m.setFlash("no voice message to play", false)
	return m, nil
}

// rebuildFolders assembles the folder bar: All, then Archive if there are any
// archived chats, then the user's custom folders.
func (m *Model) rebuildFolders(custom []tgc.Folder) {
	folders := []tgc.Folder{tgc.AllFolder()}
	for _, d := range m.chats {
		if d.Archived {
			folders = append(folders, tgc.ArchiveFolder())
			break
		}
	}
	folders = append(folders, custom...)
	m.folders = folders
	if m.folderIdx >= len(m.folders) {
		m.folderIdx = 0
	}
}

// handleIncoming applies a live new message to the open chat and the list.
// It handles both incoming messages and our own messages echoed from other
// devices.
func (m Model) handleIncoming(msg IncomingMsg) (tea.Model, tea.Cmd) {
	delete(m.typingUntil, msg.ChatID)

	// Saved Messages is yourself, so such a message is never truly "incoming".
	if m.isSelfChat(msg.ChatID) {
		msg.Incoming = false
		msg.Msg.Out = true
	}

	open := m.openedOK && m.openedChat.PeerID == msg.ChatID
	var cmd tea.Cmd
	if open {
		m.appendLive(msg.Msg)
		m.scroll = 0
		if msg.Incoming {
			cmd = markRead(m.ctx, m.sess, m.openedChat.Peer)
		}
	} else if msg.Incoming {
		cmd = notifyCmd(m.notifyText(msg.ChatID, msg.Msg))
	}

	m.bumpPreview(msg.ChatID, msg.Msg.Text, msg.Msg.Date, msg.Incoming && !open)
	m.reorder()
	return m, cmd
}

// notifyText builds a notification title and body for an incoming message.
func (m Model) notifyText(chatID int64, msg tgc.Message) (title, body string) {
	title = msg.Sender
	for _, d := range m.chats {
		if d.PeerID == chatID {
			title = d.Title
			break
		}
	}
	body = msg.Text
	if msg.Sender != "" && msg.Sender != title {
		body = msg.Sender + ": " + msg.Text
	}
	return title, body
}

// isSelfChat reports whether a peer id is the Saved Messages chat.
func (m Model) isSelfChat(chatID int64) bool {
	for _, d := range m.chats {
		if d.PeerID == chatID {
			return d.IsSelf
		}
	}
	return false
}

// bumpPreview refreshes a chat's last-message preview and date, and optionally
// bumps its unread count.
func (m *Model) bumpPreview(chatID int64, text string, date int, addUnread bool) {
	for i := range m.chats {
		if m.chats[i].PeerID != chatID {
			continue
		}
		if text != "" {
			m.chats[i].Preview = text
		}
		if date > 0 {
			m.chats[i].Date = date
		}
		if addUnread {
			m.chats[i].Unread++
		}
	}
}

// reorder sorts the chat list newest-first (pinned chats stay on top) while
// keeping the cursor on the same chat so it doesn't jump.
func (m *Model) reorder() {
	var selPeer int64
	if vis := m.visible(); m.selected >= 0 && m.selected < len(vis) {
		selPeer = vis[m.selected].PeerID
	}

	sort.SliceStable(m.chats, func(i, j int) bool {
		pi, pj := m.chats[i].Pinned, m.chats[j].Pinned
		if pi != pj {
			return pi // pinned chats stay above the rest
		}
		if pi { // both pinned: keep their existing order
			return false
		}
		return m.chats[i].Date > m.chats[j].Date
	})

	if selPeer != 0 {
		for i, d := range m.visible() {
			if d.PeerID == selPeer {
				m.selected = i
				break
			}
		}
	}
}

// appendLive adds a live message, skipping duplicates. An outgoing message
// reconciles the optimistic (id 0) copy left by a local send.
func (m *Model) appendLive(msg tgc.Message) {
	if hasMessageID(m.messages, msg.ID) {
		return
	}
	if msg.Out {
		// Reconcile the optimistic (id 0) copy from a local send with its real
		// echo, whichever arrives first, so the message isn't shown twice.
		for i := range m.messages {
			mm := m.messages[i]
			if !mm.Out || mm.Text != msg.Text {
				continue
			}
			if mm.ID == 0 && msg.ID != 0 {
				m.messages[i] = msg // real echo replaces the optimistic copy
				return
			}
			if mm.ID != 0 && msg.ID == 0 {
				return // echo already here; drop the optimistic copy
			}
		}
	}
	m.messages = append(m.messages, msg)
}

func hasMessageID(msgs []tgc.Message, id int) bool {
	if id == 0 {
		return false
	}
	for _, mm := range msgs {
		if mm.ID == id {
			return true
		}
	}
	return false
}

// switchFolder moves to the next/previous folder and resets the cursor.
func (m Model) switchFolder(step int) Model {
	if len(m.folders) <= 1 {
		return m
	}
	m.folderIdx = (m.folderIdx + step + len(m.folders)) % len(m.folders)
	m.selected = 0
	m.filter = "" // a filter from one folder shouldn't bleed into the next
	return m
}

// visible returns the dialogs in the active folder, narrowed by the search
// filter when one is set.
func (m Model) visible() []tgc.Dialog {
	base := m.chats
	if len(m.folders) > 0 && m.folderIdx < len(m.folders) {
		f := m.folders[m.folderIdx]
		base = nil
		for _, d := range m.chats {
			if f.Contains(d) {
				base = append(base, d)
			}
		}
	}
	if m.filter == "" {
		return base
	}
	q := strings.ToLower(m.filter)
	var out []tgc.Dialog
	for _, d := range base {
		if strings.Contains(strings.ToLower(sanitize(d.Title)), q) {
			out = append(out, d)
		}
	}
	return out
}

// open shows a chat in the right pane. It loads history when the chat changes;
// markReadNow additionally clears its unread badge on the server.
func (m Model) open(idx int, markReadNow bool) (Model, tea.Cmd) {
	vis := m.visible()
	if idx < 0 || idx >= len(vis) {
		return m, nil
	}
	d := vis[idx]

	var cmds []tea.Cmd
	if !m.openedOK || m.openedChat.PeerID != d.PeerID {
		m.openedChat = d
		m.openedOK = true
		m.messages = nil
		m.scroll = 0
		m.status = ""
		m.msgSel = -1
		m.editing = 0
		m.input.Reset()
		cmds = append(cmds, loadHistory(m.ctx, m.sess, d.Peer, d.PeerID))
	}
	if markReadNow {
		for i := range m.chats {
			if m.chats[i].PeerID == d.PeerID {
				m.chats[i].Unread = 0
			}
		}
		m.openedChat.Unread = 0
		cmds = append(cmds, markRead(m.ctx, m.sess, d.Peer))
	}
	return m, tea.Batch(cmds...)
}

// scrollUp moves toward older messages, paging in more at the top.
func (m *Model) scrollUp(n int) tea.Cmd {
	if !m.openedOK {
		return nil
	}
	m.scroll += n
	_, rightW, _, msgH, maxBubble := m.layout()
	maxScroll := len(m.buildLines(maxBubble, innerWidth(rightW))) - msgH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll >= maxScroll {
		m.scroll = maxScroll
		return m.startLoadOlder()
	}
	return nil
}

func (m *Model) scrollDown(n int) {
	m.scroll -= n
	if m.scroll < 0 {
		m.scroll = 0
	}
}

// startLoadOlder fetches a page older than the current oldest message.
func (m *Model) startLoadOlder() tea.Cmd {
	if m.loadingOlder || len(m.messages) == 0 || !m.openedOK {
		return nil
	}
	oldest := m.messages[0].ID
	if oldest <= 0 {
		return nil
	}
	m.loadingOlder = true
	return loadOlder(m.ctx, m.sess, m.openedChat.Peer, m.openedChat.PeerID, oldest)
}

// prependOlder adds a page of older messages, skipping any already present.
func (m *Model) prependOlder(page []tgc.Message) {
	seen := make(map[int]bool, len(m.messages))
	for _, mm := range m.messages {
		seen[mm.ID] = true
	}
	var older []tgc.Message
	for _, mm := range page {
		if mm.ID == 0 || !seen[mm.ID] {
			older = append(older, mm)
		}
	}
	m.messages = append(older, m.messages...)
}

// resizeInput sets the compose field width to fit the right pane.
func (m *Model) resizeInput() {
	_, rightW, _, _, _ := m.layout()
	w := rightW - 6
	if w < 4 {
		w = 4
	}
	m.input.Width = w
}

// openedPeer returns the open chat's peer, or nil.
func (m Model) openedPeer() tg.InputPeerClass {
	if !m.openedOK {
		return nil
	}
	return m.openedChat.Peer
}

// setFlash shows a short-lived message in the footer.
func (m *Model) setFlash(text string, isErr bool) {
	m.flash = text
	m.flashErr = isErr
	m.flashUntil = time.Now().Add(4 * time.Second)
}

// firstLine trims an error to its first line for a compact flash message.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
