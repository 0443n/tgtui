package ui

import (
	"testing"

	tgc "github.com/0443n/tgtui/internal/tg"
)

// searchMsg results clear the busy flag, switch to result navigation, and load
// the hits.
func TestSearchMsgPopulates(t *testing.T) {
	m := New()
	m.searchOpen = true
	m.searchBusy = true
	m.searchTyping = true
	hits := []tgc.Message{{ID: 5, Text: "hi"}}
	out, _ := m.Update(searchMsg{hits: hits})
	m2 := out.(Model)
	if m2.searchBusy {
		t.Fatal("busy flag should clear")
	}
	if m2.searchTyping {
		t.Fatal("should switch out of typing once results arrive")
	}
	if len(m2.hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(m2.hits))
	}
}

func TestSearchNoMatches(t *testing.T) {
	m := New()
	m.searchOpen = true
	out, _ := m.Update(searchMsg{hits: nil})
	if out.(Model).searchErr != "no matches" {
		t.Fatalf("searchErr = %q, want no matches", out.(Model).searchErr)
	}
}

// Typing appends runes to the query.
func TestSearchTypingAppends(t *testing.T) {
	m := New()
	m.openedOK = true
	m.searchOpen = true
	m.searchTyping = true
	out, _ := m.handleSearchOverlayKey(key("h"))
	out, _ = out.(Model).handleSearchOverlayKey(key("i"))
	if q := out.(Model).chatQuery; q != "hi" {
		t.Fatalf("query = %q, want hi", q)
	}
}

// In result mode j/k move the hit cursor; enter closes the overlay to jump.
func TestSearchResultNav(t *testing.T) {
	m := New()
	m.openedOK = true
	m.openedChat = tgc.Dialog{PeerID: 42}
	m.searchOpen = true
	m.searchTyping = false
	m.hits = []tgc.Message{{ID: 1}, {ID: 2}, {ID: 3}}
	out, _ := m.handleSearchOverlayKey(key("j"))
	if out.(Model).hitSel != 1 {
		t.Fatalf("hitSel = %d, want 1", out.(Model).hitSel)
	}
	out, _ = out.(Model).handleSearchOverlayKey(key("enter"))
	if out.(Model).searchOpen {
		t.Fatal("enter should close the overlay to jump")
	}
}
