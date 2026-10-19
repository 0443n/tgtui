package ui

import (
	"testing"

	tgc "github.com/0443n/tgtui/internal/tg"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestStartEditLoadsLastOwn(t *testing.T) {
	m := New()
	m.messages = []tgc.Message{
		{ID: 1, Out: true, Text: "first"},
		{ID: 2, Sender: "Alice", Text: "hi"},
		{ID: 3, Out: true, Text: "last mine"},
		{ID: 4, Out: true, Media: true, Text: "[voice]"}, // media: skipped
	}
	m.startEdit()
	if m.editing != 3 || m.input.Value() != "last mine" {
		t.Fatalf("editing=%d input=%q", m.editing, m.input.Value())
	}
}

func TestMoveReplyTargetWalks(t *testing.T) {
	m := New()
	m.messages = []tgc.Message{{ID: 1}, {ID: 2}, {ID: 3}}
	m.moveReplyTarget(-1) // none -> newest
	if m.msgSel != 2 {
		t.Fatalf("first up = %d, want 2", m.msgSel)
	}
	m.moveReplyTarget(-1) // older
	if m.msgSel != 1 {
		t.Fatalf("second up = %d, want 1", m.msgSel)
	}
	m.moveReplyTarget(1) // newer -> 2
	m.moveReplyTarget(1) // past newest -> cancel
	if m.msgSel != -1 {
		t.Fatalf("past newest = %d, want -1", m.msgSel)
	}
}

func TestSendStatus(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii) // Render is identity, no ANSI
	m := New()
	m.openedChat = tgc.Dialog{ReadOutboxMaxID: 100}
	m.openedOK = true
	if got := m.sendStatus(tgc.Message{ID: 100, Out: true}); got != "" {
		t.Fatalf("read status = %q, want empty", got)
	}
	if got := m.sendStatus(tgc.Message{ID: 101, Out: true}); got != "[unread]" {
		t.Fatalf("unread status = %q, want [unread]", got)
	}
	if got := m.sendStatus(tgc.Message{ID: 5, Out: false}); got != "" {
		t.Fatalf("incoming status = %q, want empty", got)
	}
}

func TestEditedMsgUpdatesText(t *testing.T) {
	m := New()
	m.openedChat = tgc.Dialog{PeerID: 1}
	m.openedOK = true
	m.messages = []tgc.Message{{ID: 7, Out: true, Text: "old"}}
	out, _ := m.Update(editedMsg{id: 7, text: "new"})
	if m2 := out.(Model); m2.messages[0].Text != "new" {
		t.Fatalf("text = %q, want new", m2.messages[0].Text)
	}
}
