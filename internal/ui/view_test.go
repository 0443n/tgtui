package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tgc "github.com/0443n/tgtui/internal/tg"
	"github.com/gotd/td/tg"
)

func sampleModel(w, h int) Model {
	m := New()
	m.width, m.height = w, h
	m.resizeInput()
	m.status = "ready"
	now := int(time.Now().Unix())
	for i := 0; i < 40; i++ {
		m.chats = append(m.chats, tgc.Dialog{
			Title:   fmt.Sprintf("Chat number %d", i),
			Peer:    &tg.InputPeerChat{ChatID: int64(i)},
			Unread:  i % 3,
			Date:    now,
			Preview: "last message preview here",
		})
	}
	m.selected = 6
	m.openedChat = m.chats[6]
	m.openedOK = true
	m.messages = []tgc.Message{
		{Date: now - 100, Sender: "Alice", Text: "hey there"},
		{Date: now - 90, Out: true, Text: "how are you doing today"},
		{Date: now - 80, Sender: "Alice", Text: "[voice]"},
		{Date: now - 70, Sender: "Alice", Text: strings.Repeat("wrap ", 30)},
	}
	return m
}

func TestTwoPaneRenders(t *testing.T) {
	m := sampleModel(90, 22)
	out := m.View()
	for _, want := range []string{"tgtui", "Chat number 6", "you"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q", want)
		}
	}
	if got := strings.Count(out, "\n") + 1; got != 22 {
		t.Fatalf("want 22 rows, got %d", got)
	}
}

func TestNoChatOpen(t *testing.T) {
	m := sampleModel(90, 22)
	m.openedOK = false
	if !strings.Contains(m.View(), "select a chat") {
		t.Fatal("missing empty-chat hint")
	}
}

func TestTinySizeNoPanic(t *testing.T) {
	for _, s := range [][2]int{{1, 1}, {10, 3}, {5, 5}, {40, 8}, {20, 2}} {
		m := sampleModel(s[0], s[1])
		_ = m.View()
		m.openedOK = false
		_ = m.View()
	}
}
