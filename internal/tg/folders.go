package tg

import (
	"context"

	"github.com/gotd/td/tg"
)

// Folder is a chat folder (Telegram "dialog filter").
type Folder struct {
	ID    int
	Title string

	all     bool // "All" - the main list, excludes archived
	archive bool // "Archive" - archived chats only

	// Type flags (a normal folder includes chats matching these).
	contacts, nonContacts, groups, broadcasts, bots bool
	excludeArchived                                 bool
	// chatlist folders only ever contain their explicit peers.
	chatlist bool

	pinned, include, exclude map[int64]bool
}

// AllFolder is the main list: every non-archived chat.
func AllFolder() Folder { return Folder{Title: "All", all: true} }

// ArchiveFolder holds the archived chats.
func ArchiveFolder() Folder { return Folder{Title: "Archive", archive: true} }

// Contains reports whether a dialog belongs to this folder.
func (f Folder) Contains(d Dialog) bool {
	if f.all {
		return !d.Archived
	}
	if f.archive {
		return d.Archived
	}
	if f.exclude[d.PeerID] {
		return false
	}
	if f.pinned[d.PeerID] || f.include[d.PeerID] {
		return true
	}
	if f.chatlist {
		return false
	}
	if f.excludeArchived && d.Archived {
		return false
	}
	switch {
	case d.IsBot:
		return f.bots
	case d.IsUser && d.IsContact:
		return f.contacts
	case d.IsUser:
		return f.nonContacts
	case d.IsGroup:
		return f.groups
	case d.IsBroadcast:
		return f.broadcasts
	}
	return false
}

// Folders loads the user's custom chat folders (dialog filters). The "All"
// and "Archive" folders are added by the caller.
func (s *Session) Folders(ctx context.Context) ([]Folder, error) {
	res, err := s.api.MessagesGetDialogFilters(ctx)
	if err != nil {
		return nil, err
	}

	var out []Folder
	for _, fc := range res.Filters {
		switch f := fc.(type) {
		case *tg.DialogFilter:
			out = append(out, Folder{
				ID:              f.ID,
				Title:           f.Title.Text,
				contacts:        f.Contacts,
				nonContacts:     f.NonContacts,
				groups:          f.Groups,
				broadcasts:      f.Broadcasts,
				bots:            f.Bots,
				excludeArchived: f.ExcludeArchived,
				pinned:          s.peerIDSet(f.PinnedPeers),
				include:         s.peerIDSet(f.IncludePeers),
				exclude:         s.peerIDSet(f.ExcludePeers),
			})
		case *tg.DialogFilterChatlist:
			out = append(out, Folder{
				ID:       f.ID,
				Title:    f.Title.Text,
				chatlist: true,
				pinned:   s.peerIDSet(f.PinnedPeers),
				include:  s.peerIDSet(f.IncludePeers),
			})
		}
	}
	return out, nil
}

// peerIDSet collects the ids referenced by a list of input peers.
func (s *Session) peerIDSet(peers []tg.InputPeerClass) map[int64]bool {
	set := make(map[int64]bool, len(peers))
	for _, p := range peers {
		switch pp := p.(type) {
		case *tg.InputPeerUser:
			set[pp.UserID] = true
		case *tg.InputPeerChat:
			set[pp.ChatID] = true
		case *tg.InputPeerChannel:
			set[pp.ChannelID] = true
		case *tg.InputPeerSelf:
			set[s.selfID] = true
		}
	}
	return set
}
