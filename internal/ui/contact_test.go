package ui

import (
	"testing"

	tgc "github.com/0443n/tgtui/internal/tg"
)

func TestSplitName(t *testing.T) {
	cases := []struct{ in, first, last string }{
		{"Pavel Durov", "Pavel", "Durov"},
		{"Alice", "Alice", ""},
		{"  Jean  Luc  Picard ", "Jean", "Luc  Picard"},
		{"", "", ""},
	}
	for _, c := range cases {
		f, l := splitName(c.in)
		if f != c.first || l != c.last {
			t.Errorf("splitName(%q) = %q,%q want %q,%q", c.in, f, l, c.first, c.last)
		}
	}
}

// A successful rename updates the open chat and list titles.
func TestRenamedMsgUpdatesTitles(t *testing.T) {
	m := New()
	m.openedOK = true
	m.openedChat = tgc.Dialog{PeerID: 42, Title: "old", IsUser: true}
	m.chats = []tgc.Dialog{{PeerID: 42, Title: "old"}, {PeerID: 7, Title: "other"}}
	out, _ := m.Update(renamedMsg{name: "New Name"})
	m2 := out.(Model)
	if m2.openedChat.Title != "New Name" {
		t.Fatalf("open title = %q, want New Name", m2.openedChat.Title)
	}
	if m2.chats[0].Title != "New Name" || m2.chats[1].Title != "other" {
		t.Fatalf("list titles = %q,%q", m2.chats[0].Title, m2.chats[1].Title)
	}
}

// c only opens the contact overlay for a real (non-self) user chat.
func TestContactOpensForUsersOnly(t *testing.T) {
	m := New()
	m.openedOK = true
	m.openedChat = tgc.Dialog{PeerID: 5, IsBroadcast: true} // a channel, not a user
	out, _ := m.handleListKey(key("c"))
	if out.(Model).contactOpen {
		t.Fatalf("contact overlay should not open for a channel")
	}

	m.openedChat = tgc.Dialog{PeerID: 5, IsSelf: true, IsUser: true} // Saved Messages
	out, _ = m.handleListKey(key("c"))
	if out.(Model).contactOpen {
		t.Fatalf("contact overlay should not open for Saved Messages")
	}
}
