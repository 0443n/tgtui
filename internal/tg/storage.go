package tg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/gotd/td/session"
	"github.com/zalando/go-keyring"
)

const keyringService = "tgtui"

// keyringTimeout bounds a Secret Service call, which can block forever on a
// locked wallet waiting for an unlock prompt nobody sees.
const keyringTimeout = 3 * time.Second

var errKeyringTimeout = errors.New("keyring did not answer")

// sessionStorage keeps the Telegram session (a full-account secret) in the
// OS keyring. When no keyring is available (e.g. headless box), it falls
// back to a 0600 file. It satisfies gotd's session.Storage.
type sessionStorage struct {
	user     string // keyring account key
	fallback string // file used when the keyring is unavailable
	hung     atomic.Bool
}

func newSessionStorage(user, fallback string) *sessionStorage {
	return &sessionStorage{user: user, fallback: fallback}
}

// keyringDo runs f with a timeout. After one timeout the keyring is skipped
// for the rest of the run so every session save does not stall again.
func (s *sessionStorage) keyringDo(ctx context.Context, f func() (string, error)) (string, error) {
	if s.hung.Load() {
		return "", errKeyringTimeout
	}
	type result struct {
		v   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := f()
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(keyringTimeout):
		s.hung.Store(true)
		return "", errKeyringTimeout
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *sessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	secret, err := s.keyringDo(ctx, func() (string, error) { return keyring.Get(keyringService, s.user) })
	if err == nil {
		return []byte(secret), nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	// Keyring miss or unavailable: try the fallback file.
	data, err := os.ReadFile(s.fallback)
	switch {
	case err == nil:
		return data, nil
	case errors.Is(err, os.ErrNotExist):
		return nil, session.ErrNotFound
	default:
		return nil, err
	}
}

func (s *sessionStorage) StoreSession(ctx context.Context, data []byte) error {
	_, err := s.keyringDo(ctx, func() (string, error) { return "", keyring.Set(keyringService, s.user, string(data)) })
	if err == nil {
		return nil
	}
	// Keyring unavailable: write a private file instead.
	if err := os.MkdirAll(filepath.Dir(s.fallback), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.fallback, data, 0o600)
}
