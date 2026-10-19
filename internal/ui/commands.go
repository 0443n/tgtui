package ui

import (
	"context"
	"strings"

	tgc "github.com/0443n/tgtui/internal/tg"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gotd/td/tg"
)

// ConnectedMsg is sent once we have a live session.
type ConnectedMsg struct {
	Session *tgc.Session
	Ctx     context.Context
}

// FatalMsg is a connection or login error.
type FatalMsg struct{ Err error }

type dialogsMsg struct {
	dialogs []tgc.Dialog
	err     error
}

type foldersMsg struct {
	folders []tgc.Folder
	err     error
}

// TypingMsg is a live typing notification from a chat.
type TypingMsg struct {
	ChatID int64
	Name   string
}

// StatusMsg is a live presence change for a user.
type StatusMsg struct {
	UserID int64
	Status string
	Online bool
}

// IncomingMsg is a live new message.
type IncomingMsg struct {
	ChatID   int64
	Incoming bool
	Msg      tgc.Message
}

// ReadMsg is a live read-state change (usually from another device): the
// chat's unread count changed.
type ReadMsg struct {
	ChatID int64
	Unread int
}

// OutboxReadMsg reports that the peer read our messages up to MaxID.
type OutboxReadMsg struct {
	ChatID int64
	MaxID  int
}

// editedMsg is the result of editing one of our messages.
type editedMsg struct {
	id   int
	text string
	err  error
}

// playbackMsg reports a playback event: an error, or a speed change.
type playbackMsg struct {
	speed float64
	err   error
}

type historyMsg struct {
	chatID int64
	msgs   []tgc.Message
	err    error
}

// olderMsg carries a page of messages older than what is loaded.
type olderMsg struct {
	chatID int64
	msgs   []tgc.Message
	err    error
}

type sentMsg struct {
	text string
	err  error
}

// deletedMsg is the result of deleting messages.
type deletedMsg struct {
	n   int
	err error
}

// forwardedMsg is the result of forwarding messages to another chat.
type forwardedMsg struct {
	n     int
	title string
	err   error
}

// profileMsg carries a fetched contact profile.
type profileMsg struct {
	p   tgc.Profile
	err error
}

// renamedMsg is the result of saving a contact's name.
type renamedMsg struct {
	name string
	err  error
}

// blockedMsg is the result of blocking or unblocking a user.
type blockedMsg struct {
	blocked bool
	err     error
}

// contactDeletedMsg is the result of removing a user from contacts.
type contactDeletedMsg struct {
	err error
}

// searchMsg carries in-chat search results.
type searchMsg struct {
	hits []tgc.Message
	err  error
}

const pageSize = 40

func loadDialogs(ctx context.Context, s *tgc.Session) tea.Cmd {
	return func() tea.Msg {
		d, err := s.Dialogs(ctx, 200)
		return dialogsMsg{d, err}
	}
}

func loadFolders(ctx context.Context, s *tgc.Session) tea.Cmd {
	return func() tea.Msg {
		f, err := s.Folders(ctx)
		return foldersMsg{f, err}
	}
}

func loadHistory(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass, chatID int64) tea.Cmd {
	return func() tea.Msg {
		m, err := s.History(ctx, peer, pageSize, 0)
		return historyMsg{chatID, m, err}
	}
}

func loadOlder(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass, chatID int64, offsetID int) tea.Cmd {
	return func() tea.Msg {
		m, err := s.History(ctx, peer, pageSize, offsetID)
		return olderMsg{chatID, m, err}
	}
}

// searchChat runs an in-chat message search.
func searchChat(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass, query string) tea.Cmd {
	return func() tea.Msg {
		hits, err := s.Search(ctx, peer, query, 50)
		return searchMsg{hits: hits, err: err}
	}
}

// jumpTo loads a history page ending at offsetID, so a searched-for message
// shows at the bottom of the chat. It reuses historyMsg.
func jumpTo(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass, chatID int64, offsetID int) tea.Cmd {
	return func() tea.Msg {
		m, err := s.History(ctx, peer, pageSize, offsetID)
		return historyMsg{chatID, m, err}
	}
}

func sendMessage(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass, text string, replyTo int) tea.Cmd {
	return func() tea.Msg {
		err := s.Send(ctx, peer, text, replyTo)
		return sentMsg{text, err}
	}
}

func editMessage(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass, id int, text string) tea.Cmd {
	return func() tea.Msg {
		err := s.Edit(ctx, peer, id, text)
		return editedMsg{id, text, err}
	}
}

// playVoice downloads (or reuses a cached) voice/video note and plays it.
func playVoice(ctx context.Context, s *tgc.Session, chatID int64, msg tgc.Message, label string) tea.Cmd {
	return func() tea.Msg {
		path := voicePath(chatID, msg)
		if !fileReady(path) {
			if err := s.DownloadVoice(ctx, msg, path); err != nil {
				return playbackMsg{err: err}
			}
		}
		return playbackMsg{err: audio.play(path, label)}
	}
}

func stopVoice() tea.Cmd {
	return func() tea.Msg { audio.stop(); return playbackMsg{} }
}

func speedVoice() tea.Cmd {
	return func() tea.Msg { return playbackMsg{speed: audio.toggleSpeed()} }
}

func deleteMessages(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass, ids []int, revoke bool) tea.Cmd {
	n := len(ids)
	return func() tea.Msg {
		return deletedMsg{n: n, err: s.Delete(ctx, peer, ids, revoke)}
	}
}

func forwardMessages(ctx context.Context, s *tgc.Session, from, to tg.InputPeerClass, ids []int, title string) tea.Cmd {
	n := len(ids)
	return func() tea.Msg {
		return forwardedMsg{n: n, title: title, err: s.Forward(ctx, from, to, ids)}
	}
}

func loadProfile(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass) tea.Cmd {
	return func() tea.Msg {
		p, err := s.Profile(ctx, peer)
		return profileMsg{p: p, err: err}
	}
}

func renameContact(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass, first, last string) tea.Cmd {
	name := strings.TrimSpace(first + " " + last)
	return func() tea.Msg {
		return renamedMsg{name: name, err: s.Rename(ctx, peer, first, last)}
	}
}

func setBlocked(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass, block bool) tea.Cmd {
	return func() tea.Msg {
		return blockedMsg{blocked: block, err: s.SetBlocked(ctx, peer, block)}
	}
}

func deleteContact(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass) tea.Cmd {
	return func() tea.Msg {
		return contactDeletedMsg{err: s.DeleteContact(ctx, peer)}
	}
}

// markRead marks the chat read. Errors are not critical, so it sends no msg.
func markRead(ctx context.Context, s *tgc.Session, peer tg.InputPeerClass) tea.Cmd {
	return func() tea.Msg {
		_ = s.MarkRead(ctx, peer)
		return nil
	}
}
