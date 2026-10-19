package tg

import (
	"context"
	"errors"
	"strings"

	"github.com/gotd/td/tg"
)

// Profile is a user's details for the contact view.
type Profile struct {
	Name     string
	Username string
	Phone    string
	Bio      string
	Common   int // number of chats in common
	Status   string
	Blocked  bool
	Contact  bool // already in our contacts
}

// inputUser converts a user peer into an InputUser, or reports false for
// non-user peers (chats and channels have no profile).
func inputUser(peer tg.InputPeerClass) (*tg.InputUser, bool) {
	up, ok := peer.(*tg.InputPeerUser)
	if !ok {
		return nil, false
	}
	return &tg.InputUser{UserID: up.UserID, AccessHash: up.AccessHash}, true
}

// Profile fetches a user's full profile. It only works for user chats.
func (s *Session) Profile(ctx context.Context, peer tg.InputPeerClass) (Profile, error) {
	iu, ok := inputUser(peer)
	if !ok {
		return Profile{}, errors.New("not a user chat")
	}
	full, err := s.api.UsersGetFullUser(ctx, iu)
	if err != nil {
		return Profile{}, err
	}
	p := Profile{
		Bio:     full.FullUser.About,
		Common:  full.FullUser.CommonChatsCount,
		Blocked: full.FullUser.Blocked,
	}
	for _, u := range full.Users {
		uu, ok := u.(*tg.User)
		if !ok || uu.ID != iu.UserID {
			continue
		}
		p.Name = userDisplay(uu)
		p.Username = uu.Username
		p.Phone = uu.Phone
		p.Contact = uu.Contact
		p.Status, _ = userStatus(uu.Status)
		break
	}
	return p, nil
}

// Rename saves (or updates) the contact name for a user. When they are not a
// contact yet, this also adds them.
func (s *Session) Rename(ctx context.Context, peer tg.InputPeerClass, first, last string) error {
	iu, ok := inputUser(peer)
	if !ok {
		return errors.New("not a user chat")
	}
	if strings.TrimSpace(first) == "" {
		return errors.New("name can't be empty")
	}
	_, err := s.api.ContactsAddContact(ctx, &tg.ContactsAddContactRequest{
		ID:        iu,
		FirstName: first,
		LastName:  last,
	})
	return err
}

// SetBlocked blocks or unblocks a peer.
func (s *Session) SetBlocked(ctx context.Context, peer tg.InputPeerClass, block bool) error {
	if block {
		_, err := s.api.ContactsBlock(ctx, &tg.ContactsBlockRequest{ID: peer})
		return err
	}
	_, err := s.api.ContactsUnblock(ctx, &tg.ContactsUnblockRequest{ID: peer})
	return err
}

// DeleteContact removes a user from our contacts. They stay reachable; only the
// saved-contact entry (and its name) goes away.
func (s *Session) DeleteContact(ctx context.Context, peer tg.InputPeerClass) error {
	iu, ok := inputUser(peer)
	if !ok {
		return errors.New("not a user chat")
	}
	_, err := s.api.ContactsDeleteContacts(ctx, []tg.InputUserClass{iu})
	return err
}
