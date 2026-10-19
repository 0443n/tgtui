package ui

import (
	"context"
	"testing"

	tgc "github.com/0443n/tgtui/internal/tg"
	"github.com/gotd/td/tg"
)

func TestFoldersArchive(t *testing.T) {
	m := New()
	m.chats = []tgc.Dialog{
		{Title: "A", PeerID: 1, IsUser: true},
		{Title: "B", PeerID: 2, IsUser: true, Archived: true},
	}
	m.rebuildFolders(nil)

	if len(m.folders) != 2 { // All + Archive
		t.Fatalf("want All+Archive, got %d folders", len(m.folders))
	}

	m.folderIdx = 0 // All excludes archived
	if v := m.visible(); len(v) != 1 || v[0].PeerID != 1 {
		t.Fatalf("All folder = %+v", v)
	}
	m.folderIdx = 1 // Archive only
	if v := m.visible(); len(v) != 1 || v[0].PeerID != 2 {
		t.Fatalf("Archive folder = %+v", v)
	}
}

func TestIncomingAppends(t *testing.T) {
	m := New()
	m.ctx = context.Background()
	m.chats = []tgc.Dialog{{Title: "A", PeerID: 1, Peer: &tg.InputPeerChat{ChatID: 1}}}
	m.openedChat = m.chats[0]
	m.openedOK = true

	out, _ := m.handleIncoming(IncomingMsg{ChatID: 1, Incoming: true, Msg: tgc.Message{ID: 5, Text: "hi"}})
	m2 := out.(Model)
	if len(m2.messages) != 1 || m2.messages[0].Text != "hi" {
		t.Fatalf("message not appended: %+v", m2.messages)
	}

	// A message to a different, unopened chat bumps its unread count.
	m2.chats = append(m2.chats, tgc.Dialog{Title: "B", PeerID: 2})
	out, _ = m2.handleIncoming(IncomingMsg{ChatID: 2, Incoming: true, Msg: tgc.Message{ID: 6, Text: "yo"}})
	m3 := out.(Model)
	if m3.chats[1].Unread != 1 {
		t.Fatalf("unread not bumped: %d", m3.chats[1].Unread)
	}
}

func TestReorderKeepsCursor(t *testing.T) {
	m := New()
	m.ctx = context.Background()
	m.chats = []tgc.Dialog{
		{Title: "C", PeerID: 3, Date: 300},
		{Title: "B", PeerID: 2, Date: 200},
		{Title: "A", PeerID: 1, Date: 100},
	}
	m.selected = 2 // cursor on A

	// A message to B (newest) should float B to the top.
	out, _ := m.handleIncoming(IncomingMsg{ChatID: 2, Incoming: true, Msg: tgc.Message{ID: 9, Date: 400, Text: "hi"}})
	m2 := out.(Model)

	if m2.chats[0].PeerID != 2 {
		t.Fatalf("B not moved to top: %+v", m2.chats)
	}
	if vis := m2.visible(); vis[m2.selected].PeerID != 1 {
		t.Fatalf("cursor left A: on peer %d", vis[m2.selected].PeerID)
	}
}

func TestOutgoingEchoReconciles(t *testing.T) {
	m := New()
	m.ctx = context.Background()
	m.chats = []tgc.Dialog{{Title: "A", PeerID: 1, Peer: &tg.InputPeerChat{ChatID: 1}}}
	m.openedChat = m.chats[0]
	m.openedOK = true

	// Own message typed on another device (not incoming) must still appear.
	out, _ := m.handleIncoming(IncomingMsg{ChatID: 1, Incoming: false, Msg: tgc.Message{ID: 10, Out: true, Text: "hi"}})
	m = out.(Model)
	if len(m.messages) != 1 {
		t.Fatalf("outgoing echo not appended: %d", len(m.messages))
	}

	// Local send (optimistic, id 0) then its echo (real id) must not duplicate.
	m.appendLive(tgc.Message{Out: true, Text: "yo"})
	m.appendLive(tgc.Message{ID: 11, Out: true, Text: "yo"})
	if len(m.messages) != 2 || m.messages[1].ID != 11 {
		t.Fatalf("reconcile failed: %+v", m.messages)
	}
}
