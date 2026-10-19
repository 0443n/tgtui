package ui

import (
	"testing"

	tgc "github.com/0443n/tgtui/internal/tg"
	tea "github.com/charmbracelet/bubbletea"
)

// key builds a KeyMsg for a single key, so a test can drive the key handlers.
func key(s string) tea.KeyMsg {
	switch s {
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func selectModel() Model {
	m := New()
	m.openedOK = true
	m.openedChat = tgc.Dialog{PeerID: 42, Title: "Alice"}
	m.messages = []tgc.Message{
		{ID: 1, Sender: "Alice", Text: "one"},
		{ID: 2, Out: true, Text: "two"},
		{ID: 3, Sender: "Alice", Text: "three"},
		{ID: 0, Out: true, Text: "not sent yet"}, // optimistic, no server id
	}
	m.focus = focusSelect
	m.selCursor = 2
	m.marked = map[int]bool{}
	return m
}

// actionIDs uses the cursor when nothing is marked, and skips id 0.
func TestActionIDsCursor(t *testing.T) {
	m := selectModel()
	if got := m.actionIDs(); len(got) != 1 || got[0] != 3 {
		t.Fatalf("cursor action = %v, want [3]", got)
	}
	m.selCursor = 3 // the optimistic (id 0) message can't be acted on
	if got := m.actionIDs(); got != nil {
		t.Fatalf("id-0 action = %v, want nil", got)
	}
}

// Marked ids win over the cursor and come back in message order.
func TestActionIDsMarked(t *testing.T) {
	m := selectModel()
	m.marked = map[int]bool{3: true, 1: true}
	got := m.actionIDs()
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("marked action = %v, want [1 3]", got)
	}
}

// doDelete removes the chosen messages locally right away.
func TestDoDeleteRemovesLocally(t *testing.T) {
	m := selectModel()
	m.marked = map[int]bool{2: true}
	out, _ := m.doDelete(true)
	m2 := out.(Model)
	for _, mm := range m2.messages {
		if mm.ID == 2 {
			t.Fatalf("message 2 still present after delete")
		}
	}
	if m2.confirmDelete {
		t.Fatalf("confirmDelete should be cleared")
	}
	if len(m2.marked) != 0 {
		t.Fatalf("marks should be cleared, got %v", m2.marked)
	}
}

// Pressing f with a target moves into the destination-picker (forwarding) state.
func TestForwardEntersPicker(t *testing.T) {
	m := selectModel()
	out, _ := m.handleSelectKey(key("f"))
	m2 := out.(Model)
	if !m2.forwarding {
		t.Fatalf("f should start forwarding")
	}
	if m2.focus != focusList {
		t.Fatalf("forwarding should hand focus to the list, got %v", m2.focus)
	}
	if len(m2.forwardIDs) != 1 || m2.forwardIDs[0] != 3 {
		t.Fatalf("forwardIDs = %v, want [3]", m2.forwardIDs)
	}
}

// Space marks the current message and steps the cursor down.
func TestSpaceMarksAndAdvances(t *testing.T) {
	m := selectModel()
	m.selCursor = 0
	out, _ := m.handleSelectKey(key(" "))
	m2 := out.(Model)
	if !m2.marked[1] {
		t.Fatalf("space should mark message 1")
	}
	if m2.selCursor != 1 {
		t.Fatalf("cursor = %d, want 1 after marking", m2.selCursor)
	}
}
