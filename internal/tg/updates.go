package tg

import (
	"strings"

	"github.com/gotd/td/tg"
)

// Event is a live update pushed to the UI.
type Event interface{ isEvent() }

// TypingEvent says someone is typing in a chat. ChatID is the dialog's peer
// id (the user id for a DM, the chat/channel id otherwise).
type TypingEvent struct {
	ChatID int64
	Name   string
}

// StatusEvent is a user's presence change.
type StatusEvent struct {
	UserID int64
	Status string
	Online bool
}

// MessageEvent is a new incoming or outgoing message.
type MessageEvent struct {
	ChatID   int64
	Incoming bool
	Msg      Message
}

// ReadEvent means a chat's inbox was read (often on another device); Unread is
// how many messages are still unread there.
type ReadEvent struct {
	ChatID int64
	Unread int
}

// OutboxReadEvent means the peer read our messages up to MaxID.
type OutboxReadEvent struct {
	ChatID int64
	MaxID  int
}

func (TypingEvent) isEvent()     {}
func (StatusEvent) isEvent()     {}
func (MessageEvent) isEvent()    {}
func (ReadEvent) isEvent()       {}
func (OutboxReadEvent) isEvent() {}

// emit sends an event, dropping it if the buffer is full. A typing "cancel"
// action is ignored; the UI expires typing on a timer.
func emit(ch chan Event, ev Event, action tg.SendMessageActionClass) {
	if _, cancel := action.(*tg.SendMessageCancelAction); cancel {
		return
	}
	select {
	case ch <- ev:
	default:
	}
}

// messageEvent builds an event from a new-message update.
func messageEvent(e tg.Entities, mc tg.MessageClass) (MessageEvent, bool) {
	mm, ok := mc.(*tg.Message)
	if !ok {
		return MessageEvent{}, false
	}
	text := plainText(mm)
	if text == "" {
		return MessageEvent{}, false
	}
	sender := ""
	if !mm.Out {
		if from, ok := mm.GetFromID(); ok {
			if u, ok := e.Users[peerID(from)]; ok {
				sender = userDisplay(u)
			}
		}
	}
	msg := Message{
		ID:        mm.ID,
		Date:      mm.Date,
		Sender:    sender,
		Out:       mm.Out,
		Text:      text,
		Media:     mm.Message == "",
		ReplyToID: replyMsgID(mm),
	}
	if doc, ok := voiceDoc(mm); ok {
		msg.Voice = true
		msg.Mime = doc.MimeType
		msg.doc = doc
	}
	return MessageEvent{
		ChatID:   peerID(mm.PeerID),
		Incoming: !mm.Out,
		Msg:      msg,
	}, true
}

// userDisplay is a user's shown name; the self user reads as Saved Messages.
func userDisplay(u *tg.User) string {
	if u.Self {
		return "Saved Messages"
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		name = u.Username
	}
	return name
}

func peerID(p tg.PeerClass) int64 {
	switch pp := p.(type) {
	case *tg.PeerUser:
		return pp.UserID
	case *tg.PeerChat:
		return pp.ChatID
	case *tg.PeerChannel:
		return pp.ChannelID
	}
	return 0
}

func typerName(e tg.Entities, id int64) string {
	if u, ok := e.Users[id]; ok {
		return userDisplay(u)
	}
	return ""
}
