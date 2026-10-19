// Package tg wraps the gotd Telegram client for tgtui.
package tg

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/0443n/tgtui/internal/config"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"golang.org/x/net/proxy"
)

// Version is reported to Telegram as the app version and shown in settings.
// Release builds set it from the git tag via -ldflags -X.
var Version = "0.1.0"

// ErrNotAuthorized means there is no valid session yet.
var ErrNotAuthorized = errors.New("not logged in: run `tgtui auth`")

// Client is a thin wrapper over telegram.Client.
type Client struct {
	client *telegram.Client
	gaps   *updates.Manager
	events chan Event
}

// Events streams live updates (typing, status, incoming messages). Read it
// from a goroutine.
func (c *Client) Events() <-chan Event { return c.events }

// Session gives access to the API once we are connected and logged in.
type Session struct {
	api    *tg.Client
	selfID int64
}

// New builds a client from config.
func New(cfg config.Config) (*Client, error) {
	events := make(chan Event, 128)

	dispatcher := tg.NewUpdateDispatcher()
	dispatcher.OnUserTyping(func(_ context.Context, e tg.Entities, u *tg.UpdateUserTyping) error {
		emit(events, TypingEvent{ChatID: u.UserID, Name: typerName(e, u.UserID)}, u.Action)
		return nil
	})
	dispatcher.OnChatUserTyping(func(_ context.Context, e tg.Entities, u *tg.UpdateChatUserTyping) error {
		emit(events, TypingEvent{ChatID: u.ChatID, Name: typerName(e, peerID(u.FromID))}, u.Action)
		return nil
	})
	dispatcher.OnChannelUserTyping(func(_ context.Context, e tg.Entities, u *tg.UpdateChannelUserTyping) error {
		emit(events, TypingEvent{ChatID: u.ChannelID, Name: typerName(e, peerID(u.FromID))}, u.Action)
		return nil
	})
	dispatcher.OnUserStatus(func(_ context.Context, _ tg.Entities, u *tg.UpdateUserStatus) error {
		status, online := userStatus(u.Status)
		emit(events, StatusEvent{UserID: u.UserID, Status: status, Online: online}, nil)
		return nil
	})
	dispatcher.OnNewMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		if ev, ok := messageEvent(e, u.Message); ok {
			emit(events, ev, nil)
		}
		return nil
	})
	dispatcher.OnNewChannelMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		if ev, ok := messageEvent(e, u.Message); ok {
			emit(events, ev, nil)
		}
		return nil
	})
	dispatcher.OnReadHistoryInbox(func(_ context.Context, _ tg.Entities, u *tg.UpdateReadHistoryInbox) error {
		emit(events, ReadEvent{ChatID: peerID(u.Peer), Unread: u.StillUnreadCount}, nil)
		return nil
	})
	dispatcher.OnReadChannelInbox(func(_ context.Context, _ tg.Entities, u *tg.UpdateReadChannelInbox) error {
		emit(events, ReadEvent{ChatID: u.ChannelID, Unread: u.StillUnreadCount}, nil)
		return nil
	})
	dispatcher.OnReadHistoryOutbox(func(_ context.Context, _ tg.Entities, u *tg.UpdateReadHistoryOutbox) error {
		emit(events, OutboxReadEvent{ChatID: peerID(u.Peer), MaxID: u.MaxID}, nil)
		return nil
	})
	dispatcher.OnReadChannelOutbox(func(_ context.Context, _ tg.Entities, u *tg.UpdateReadChannelOutbox) error {
		emit(events, OutboxReadEvent{ChatID: u.ChannelID, MaxID: u.MaxID}, nil)
		return nil
	})

	gaps := updates.New(updates.Config{Handler: dispatcher})

	opts := telegram.Options{
		SessionStorage: newSessionStorage("session", cfg.SessionFile),
		UpdateHandler:  gaps,
		// Identify as tgtui in Telegram's session list instead of the gotd
		// defaults ("go1.26.5-..." device, gotd's version as the app).
		Device: telegram.DeviceConfig{
			DeviceModel:   "tgtui",
			SystemVersion: runtime.GOOS + "/" + runtime.GOARCH,
			AppVersion:    Version,
		},
	}

	if cfg.Proxy != "" {
		resolver, err := proxyResolver(cfg.Proxy)
		if err != nil {
			return nil, err
		}
		opts.Resolver = resolver
	}

	return &Client{
		client: telegram.NewClient(cfg.APIID, cfg.APIHash, opts),
		gaps:   gaps,
		events: events,
	}, nil
}

// proxyResolver routes Telegram traffic through a SOCKS5 proxy.
func proxyResolver(raw string) (dcs.Resolver, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("bad proxy url: %w", err)
	}
	if u.Scheme != "socks5" && u.Scheme != "socks5h" {
		return nil, fmt.Errorf("proxy scheme %q not supported (use socks5)", u.Scheme)
	}

	var auth *proxy.Auth
	if u.User != nil {
		pw, _ := u.User.Password()
		auth = &proxy.Auth{User: u.User.Username(), Password: pw}
	}

	d, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("build proxy dialer: %w", err)
	}
	ctxDial, ok := d.(proxy.ContextDialer)
	if !ok {
		return nil, errors.New("proxy dialer has no context support")
	}

	return dcs.Plain(dcs.PlainOptions{Dial: ctxDial.DialContext}), nil
}

// Run connects, checks the login, then runs the update manager, calling ready
// with a live Session once updates are flowing. It blocks until ctx is done.
// Call it from a goroutine.
func (c *Client) Run(ctx context.Context, ready func(*Session)) error {
	return c.client.Run(ctx, func(ctx context.Context) error {
		status, err := c.client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !status.Authorized {
			return ErrNotAuthorized
		}
		var selfID int64
		if status.User != nil {
			selfID = status.User.ID
		}
		sess := &Session{api: c.client.API(), selfID: selfID}
		return c.gaps.Run(ctx, c.client.API(), selfID, updates.AuthOptions{
			OnStart: func(context.Context) { ready(sess) },
		})
	})
}

// Dialog is one chat in the list.
type Dialog struct {
	Title    string
	Kind     string // user, chat or channel
	Peer     tg.InputPeerClass
	PeerID   int64
	Unread   int
	Date     int    // unix time of the last message
	Preview  string // last message text or a [type] tag
	Status   string // user presence, e.g. "online" or "last seen recently"
	Online   bool
	Archived bool
	Pinned   bool
	IsSelf   bool // the Saved Messages chat
	CanPost  bool // we may send here (false for channels we only follow)

	ReadOutboxMaxID int // peer has read our messages up to this id

	// Classification, used to match chat folders.
	IsUser, IsBot, IsContact, IsGroup, IsBroadcast bool
}

// Dialogs loads chats from the main list and the archive, paging until each
// is exhausted or maxPerFolder is reached. Results are deduped by peer (the
// main list wins), so a chat pinned in both folders shows only once.
func (s *Session) Dialogs(ctx context.Context, maxPerFolder int) ([]Dialog, error) {
	main, err := s.dialogsInFolder(ctx, 0, maxPerFolder)
	if err != nil {
		return nil, err
	}
	// The archive is optional; ignore its errors so a working main list shows.
	archived, _ := s.dialogsInFolder(ctx, 1, maxPerFolder)
	return mergeDialogs(main, archived), nil
}

// mergeDialogs dedups the main and archive lists by peer, keeping the first
// (main) copy's data. Whether a chat is archived comes from its own folder_id
// (read in parseDialogPage), which is the reliable signal: the main query
// returns archived chats too, each stamped folder_id 1, while pinned and main
// chats carry no folder id. The flag is OR'd across a peer's copies in case one
// response omitted it.
func mergeDialogs(main, archived []Dialog) []Dialog {
	idx := make(map[int64]int)
	var out []Dialog
	for _, d := range append(append([]Dialog{}, main...), archived...) {
		if d.PeerID == 0 {
			continue
		}
		if i, ok := idx[d.PeerID]; ok {
			out[i].Archived = out[i].Archived || d.Archived
			continue
		}
		idx[d.PeerID] = len(out)
		out = append(out, d)
	}
	return out
}

// dialogsInFolder pages through one folder (0 = main, 1 = archive).
func (s *Session) dialogsInFolder(ctx context.Context, folderID, max int) ([]Dialog, error) {
	var out []Dialog
	offDate, offID := 0, 0
	var offPeer tg.InputPeerClass = &tg.InputPeerEmpty{}

	for len(out) < max {
		req := &tg.MessagesGetDialogsRequest{
			OffsetDate: offDate,
			OffsetID:   offID,
			OffsetPeer: offPeer,
			Limit:      100,
		}
		// Only the archive needs an explicit folder id; omitting it for the
		// main list keeps pinned chats in the response.
		if folderID != 0 {
			req.SetFolderID(folderID)
		}
		res, err := s.api.MessagesGetDialogs(ctx, req)
		if err != nil {
			return out, err
		}
		page, off, more := parseDialogPage(res)
		out = append(out, page...)
		if !more || off.peer == nil || len(page) == 0 {
			break
		}
		offDate, offID, offPeer = off.date, off.id, off.peer
	}
	return out, nil
}

type dialogOffset struct {
	date, id int
	peer     tg.InputPeerClass
}

// parseDialogPage turns one getDialogs response into dialogs plus the offset
// for the next page and whether more pages may follow.
func parseDialogPage(res tg.MessagesDialogsClass) ([]Dialog, dialogOffset, bool) {
	var (
		dialogs  []tg.DialogClass
		users    []tg.UserClass
		chats    []tg.ChatClass
		messages []tg.MessageClass
		more     bool
	)
	switch d := res.(type) {
	case *tg.MessagesDialogs:
		dialogs, users, chats, messages = d.Dialogs, d.Users, d.Chats, d.Messages
	case *tg.MessagesDialogsSlice:
		dialogs, users, chats, messages = d.Dialogs, d.Users, d.Chats, d.Messages
		more = true // a slice means there may be more pages
	default:
		return nil, dialogOffset{}, false
	}

	// Build lookup tables for users and chats.
	userByID := map[int64]Dialog{}
	for _, u := range users {
		uu, ok := u.(*tg.User)
		if !ok {
			continue
		}
		status, online := userStatus(uu.Status)
		if uu.Self {
			// Saved Messages is yourself; a presence line there is noise.
			status, online = "", false
		}
		userByID[uu.ID] = Dialog{
			Title:     userDisplay(uu),
			Kind:      "user",
			Peer:      &tg.InputPeerUser{UserID: uu.ID, AccessHash: uu.AccessHash},
			PeerID:    uu.ID,
			Status:    status,
			Online:    online,
			IsSelf:    uu.Self,
			CanPost:   true,
			IsUser:    true,
			IsBot:     uu.Bot,
			IsContact: uu.Contact,
		}
	}
	chatByID := map[int64]Dialog{}
	for _, c := range chats {
		switch cc := c.(type) {
		case *tg.Chat:
			chatByID[cc.ID] = Dialog{
				Title: cc.Title, Kind: "chat", PeerID: cc.ID, IsGroup: true, CanPost: true,
				Peer: &tg.InputPeerChat{ChatID: cc.ID},
			}
		case *tg.Channel:
			d := Dialog{
				Title: cc.Title, Kind: "channel", PeerID: cc.ID, CanPost: true,
				Peer: &tg.InputPeerChannel{ChannelID: cc.ID, AccessHash: cc.AccessHash},
			}
			if cc.Megagroup {
				d.IsGroup = true
			} else {
				// A broadcast channel is post-only for its owner/admins.
				d.IsBroadcast = true
				d.CanPost = cc.Creator
				if ar, ok := cc.GetAdminRights(); ok && ar.PostMessages {
					d.CanPost = true
				}
			}
			chatByID[cc.ID] = d
		}
	}

	msgByID := map[int]*tg.Message{}
	for _, mc := range messages {
		if mm, ok := mc.(*tg.Message); ok {
			msgByID[mm.ID] = mm
		}
	}

	var out []Dialog
	var off dialogOffset
	for _, dc := range dialogs {
		dd, ok := dc.(*tg.Dialog)
		if !ok {
			continue
		}
		var d Dialog
		switch p := dd.Peer.(type) {
		case *tg.PeerUser:
			d = userByID[p.UserID]
		case *tg.PeerChat:
			d = chatByID[p.ChatID]
		case *tg.PeerChannel:
			d = chatByID[p.ChannelID]
		default:
			continue
		}
		d.Unread = dd.UnreadCount
		d.Pinned = dd.Pinned
		d.ReadOutboxMaxID = dd.ReadOutboxMaxID
		if fid, ok := dd.GetFolderID(); ok && fid == 1 {
			d.Archived = true
		}
		off = dialogOffset{id: dd.TopMessage, peer: d.Peer}
		if mm := msgByID[dd.TopMessage]; mm != nil {
			d.Date = mm.Date
			d.Preview = plainText(mm)
			off.date = mm.Date
		}
		out = append(out, d)
	}
	return out, off, more
}

// userStatus turns a Telegram presence into a short label plus an online flag.
func userStatus(st tg.UserStatusClass) (string, bool) {
	switch s := st.(type) {
	case *tg.UserStatusOnline:
		return "online", true
	case *tg.UserStatusOffline:
		return "last seen " + lastSeen(s.WasOnline), false
	case *tg.UserStatusRecently:
		return "last seen recently", false
	case *tg.UserStatusLastWeek:
		return "last seen within a week", false
	case *tg.UserStatusLastMonth:
		return "last seen within a month", false
	default:
		return "", false
	}
}

func lastSeen(ts int) string {
	t := time.Unix(int64(ts), 0)
	now := time.Now()
	switch {
	case t.Year() == now.Year() && t.YearDay() == now.YearDay():
		return "at " + t.Format("15:04")
	case t.Year() == now.Year() && t.YearDay() == now.YearDay()-1:
		return "yesterday"
	default:
		return "on " + t.Format("2 Jan")
	}
}

// plainText returns a message's text, or a [type] tag for media.
func plainText(mm *tg.Message) string {
	if mm.Message != "" {
		return mm.Message
	}
	if media, ok := mm.GetMedia(); ok {
		return mediaLabel(media)
	}
	return ""
}

// Message is one line of history.
type Message struct {
	ID     int
	Date   int    // unix time
	Sender string // display name; empty means the chat's peer
	Out    bool   // true if we sent it
	Text   string
	Media  bool // Text is a [type] tag, not real text (so not editable)

	ReplyToID     int    // id of the message this one replies to (0 = none)
	ReplyToSender string // resolved from the same page, if present
	ReplyToText   string // resolved snippet, or "" if not on the page

	Voice bool         // voice message or video note, playable as audio
	Mime  string       // document mime type, used to pick a file extension
	doc   *tg.Document // set for Voice; used to download the file
}

// voiceDoc returns the document for a voice message or video note.
func voiceDoc(mm *tg.Message) (*tg.Document, bool) {
	media, ok := mm.GetMedia()
	if !ok {
		return nil, false
	}
	md, ok := media.(*tg.MessageMediaDocument)
	if !ok || (!md.Voice && !md.Round) {
		return nil, false
	}
	doc, ok := md.Document.(*tg.Document)
	return doc, ok
}

// replyMsgID is the id of the message mm replies to, or 0.
func replyMsgID(mm *tg.Message) int {
	rt, ok := mm.GetReplyTo()
	if !ok {
		return 0
	}
	h, ok := rt.(*tg.MessageReplyHeader)
	if !ok {
		return 0
	}
	id, _ := h.GetReplyToMsgID()
	return id
}

// History loads up to limit messages, oldest first. Pass offsetID = 0 for the
// newest page, or an existing message id to page backwards into older history.
func (s *Session) History(ctx context.Context, peer tg.InputPeerClass, limit, offsetID int) ([]Message, error) {
	res, err := s.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:     peer,
		Limit:    limit,
		OffsetID: offsetID,
	})
	if err != nil {
		return nil, err
	}
	return parseHistory(res), nil
}

// Search finds messages in a chat whose text matches query, newest first.
func (s *Session) Search(ctx context.Context, peer tg.InputPeerClass, query string, limit int) ([]Message, error) {
	res, err := s.api.MessagesSearch(ctx, &tg.MessagesSearchRequest{
		Peer:   peer,
		Q:      query,
		Filter: &tg.InputMessagesFilterEmpty{},
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}
	hits := parseHistory(res)
	// parseHistory returns oldest first; search hits read better newest first.
	for i, j := 0, len(hits)-1; i < j; i, j = i+1, j-1 {
		hits[i], hits[j] = hits[j], hits[i]
	}
	return hits, nil
}

// parseHistory turns a messages response (getHistory or search) into our
// Message list, oldest first, resolving sender names and reply snippets from
// the same page.
func parseHistory(res tg.MessagesMessagesClass) []Message {
	var (
		raw   []tg.MessageClass
		users []tg.UserClass
		chats []tg.ChatClass
	)
	switch m := res.(type) {
	case *tg.MessagesMessages:
		raw, users, chats = m.Messages, m.Users, m.Chats
	case *tg.MessagesMessagesSlice:
		raw, users, chats = m.Messages, m.Users, m.Chats
	case *tg.MessagesChannelMessages:
		raw, users, chats = m.Messages, m.Users, m.Chats
	default:
		return nil
	}

	nameByUser := map[int64]string{}
	for _, u := range users {
		if uu, ok := u.(*tg.User); ok {
			name := strings.TrimSpace(uu.FirstName + " " + uu.LastName)
			if name == "" {
				name = uu.Username
			}
			nameByUser[uu.ID] = name
		}
	}
	nameByChannel := map[int64]string{}
	for _, c := range chats {
		if cc, ok := c.(*tg.Channel); ok {
			nameByChannel[cc.ID] = cc.Title
		}
	}

	// Index the page by id so a reply can quote its target, if it is present.
	rawByID := make(map[int]*tg.Message, len(raw))
	for _, r := range raw {
		if mm, ok := r.(*tg.Message); ok {
			rawByID[mm.ID] = mm
		}
	}
	sender := func(mm *tg.Message) string {
		if mm.Out {
			return "you"
		}
		if from, ok := mm.GetFromID(); ok {
			switch p := from.(type) {
			case *tg.PeerUser:
				return nameByUser[p.UserID]
			case *tg.PeerChannel:
				return nameByChannel[p.ChannelID]
			}
		}
		return ""
	}

	// Telegram returns newest first; flip to chronological order.
	out := make([]Message, 0, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		mm, ok := raw[i].(*tg.Message)
		if !ok {
			continue
		}
		text := plainText(mm)
		if text == "" {
			continue
		}
		msg := Message{
			ID:    mm.ID,
			Date:  mm.Date,
			Out:   mm.Out,
			Text:  text,
			Media: mm.Message == "",
		}
		if !mm.Out {
			msg.Sender = sender(mm)
		}
		if doc, ok := voiceDoc(mm); ok {
			msg.Voice = true
			msg.Mime = doc.MimeType
			msg.doc = doc
		}
		if rid := replyMsgID(mm); rid != 0 {
			msg.ReplyToID = rid
			if tgt := rawByID[rid]; tgt != nil {
				msg.ReplyToSender = sender(tgt)
				msg.ReplyToText = plainText(tgt)
			}
		}
		out = append(out, msg)
	}
	return out
}

// mediaLabel returns a short [type] tag for a non-text message.
func mediaLabel(media tg.MessageMediaClass) string {
	switch m := media.(type) {
	case *tg.MessageMediaEmpty:
		return ""
	case *tg.MessageMediaPhoto:
		return "[photo]"
	case *tg.MessageMediaDocument:
		switch {
		case m.Voice:
			return "[voice]"
		case m.Round:
			return "[video note]"
		case m.Video:
			return "[video]"
		}
		return documentLabel(m)
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive:
		return "[location]"
	case *tg.MessageMediaVenue:
		return "[venue]"
	case *tg.MessageMediaContact:
		return "[contact]"
	case *tg.MessageMediaPoll:
		return "[poll]"
	case *tg.MessageMediaDice:
		return "[dice]"
	case *tg.MessageMediaGame:
		return "[game]"
	case *tg.MessageMediaInvoice:
		return "[invoice]"
	case *tg.MessageMediaStory:
		return "[story]"
	case *tg.MessageMediaWebPage:
		return "" // link preview; the message text carries the link
	default:
		return "[media]"
	}
}

// documentLabel names a document by its attributes.
func documentLabel(m *tg.MessageMediaDocument) string {
	doc, ok := m.Document.(*tg.Document)
	if !ok {
		return "[file]"
	}
	name := ""
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeSticker:
			return "[sticker]"
		case *tg.DocumentAttributeAnimated:
			return "[gif]"
		case *tg.DocumentAttributeAudio:
			return "[audio]"
		case *tg.DocumentAttributeFilename:
			name = a.FileName
		}
	}
	if name != "" {
		return "[file: " + name + "]"
	}
	return "[file]"
}

// MarkRead marks the whole chat as read on the server, so other Telegram
// apps stop showing it unread.
func (s *Session) MarkRead(ctx context.Context, peer tg.InputPeerClass) error {
	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		_, err := s.api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
			Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
			MaxID:   0,
		})
		return err
	}
	_, err := s.api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{
		Peer:  peer,
		MaxID: 0,
	})
	return err
}

// Send posts a text message to peer. Pass replyTo != 0 to reply to a message.
func (s *Session) Send(ctx context.Context, peer tg.InputPeerClass, text string, replyTo int) error {
	req := &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  text,
		RandomID: randomID(),
	}
	if replyTo != 0 {
		req.SetReplyTo(&tg.InputReplyToMessage{ReplyToMsgID: replyTo})
	}
	_, err := s.api.MessagesSendMessage(ctx, req)
	return err
}

// Edit replaces the text of one of our messages.
func (s *Session) Edit(ctx context.Context, peer tg.InputPeerClass, id int, text string) error {
	_, err := s.api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:    peer,
		ID:      id,
		Message: text,
	})
	return err
}

// Delete removes messages by id. For channels the delete is always for
// everyone; for users and small groups, revoke deletes for everyone (within
// Telegram's time limit) while revoke=false only deletes on our side.
func (s *Session) Delete(ctx context.Context, peer tg.InputPeerClass, ids []int, revoke bool) error {
	if len(ids) == 0 {
		return nil
	}
	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		_, err := s.api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
			ID:      ids,
		})
		return err
	}
	_, err := s.api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
		Revoke: revoke,
		ID:     ids,
	})
	return err
}

// Forward copies messages (by id) from one chat to another.
func (s *Session) Forward(ctx context.Context, from, to tg.InputPeerClass, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	rnd := make([]int64, len(ids))
	for i := range rnd {
		rnd[i] = randomID()
	}
	_, err := s.api.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer: from,
		ToPeer:   to,
		ID:       ids,
		RandomID: rnd,
	})
	return err
}

// DownloadVoice saves a voice message or video note to path.
func (s *Session) DownloadVoice(ctx context.Context, msg Message, path string) error {
	if msg.doc == nil {
		return errors.New("message has no audio to play")
	}
	_, err := downloader.NewDownloader().
		Download(s.api, msg.doc.AsInputDocumentFileLocation("")).
		ToPath(ctx, path)
	return err
}
