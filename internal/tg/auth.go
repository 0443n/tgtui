package tg

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"
)

// Authenticate runs the login flow on the terminal and saves the session.
// It asks for phone, code, and 2FA password if needed.
func (c *Client) Authenticate(ctx context.Context) error {
	return c.client.Run(ctx, func(ctx context.Context) error {
		a := c.client.Auth()
		status, err := a.Status(ctx)
		if err != nil {
			return err
		}
		if status.Authorized {
			fmt.Println("Already logged in.")
			return nil
		}

		r := bufio.NewReader(os.Stdin)
		fmt.Print("Phone (like +12025550123): ")
		phone, err := readLine(r)
		if err != nil {
			return err
		}

		res, err := a.SendCode(ctx, phone, auth.SendCodeOptions{})
		if err != nil {
			return err
		}
		var hash string
		for {
			sent, ok := res.(*tg.AuthSentCode)
			if !ok {
				// auth.sentCodeSuccess, the server logged us in without a code.
				fmt.Println("Done. You are logged in.")
				return nil
			}
			hash = sent.PhoneCodeHash
			fmt.Println(describeSentCode(sent))
			if _, ok := sent.Type.(*tg.AuthSentCodeTypeSetUpEmailRequired); ok {
				return errors.New("Telegram wants a login email set up first; do it in an official app, then retry")
			}

			fmt.Print("Login code (empty line to resend): ")
			code, err := readLine(r)
			if err != nil {
				return err
			}
			if code == "" {
				// A failed resend leaves the first code valid, so keep waiting for it.
				if next, rerr := a.ResendCode(ctx, phone, hash); rerr != nil {
					fmt.Println("Resend failed -", rerr)
				} else {
					res = next
				}
				continue
			}

			_, err = a.SignIn(ctx, phone, code, hash)
			if errors.Is(err, auth.ErrPasswordAuthNeeded) {
				pw, perr := readPassword()
				if perr != nil {
					return perr
				}
				if _, err = a.Password(ctx, pw); err != nil {
					return err
				}
			} else if errors.As(err, new(*auth.SignUpRequired)) {
				return errors.New("this phone has no account; sign up in an official app first")
			} else if err != nil {
				return err
			}
			fmt.Println("Done. You are logged in.")
			return nil
		}
	})
}

// describeSentCode tells where Telegram delivered the code. Without it the
// default "app" delivery looks like no code arrived at all.
func describeSentCode(s *tg.AuthSentCode) string {
	var where string
	switch t := s.Type.(type) {
	case *tg.AuthSentCodeTypeApp:
		where = "as a message from \"Telegram\" in your other Telegram sessions (phone/desktop app), not by SMS"
	case *tg.AuthSentCodeTypeSMS, *tg.AuthSentCodeTypeSMSWord, *tg.AuthSentCodeTypeSMSPhrase:
		where = "by SMS"
	case *tg.AuthSentCodeTypeCall:
		where = "by a phone call"
	case *tg.AuthSentCodeTypeFlashCall, *tg.AuthSentCodeTypeMissedCall:
		where = "as a missed call, the code is the last digits of the caller number"
	case *tg.AuthSentCodeTypeEmailCode:
		where = "to your login email " + t.EmailPattern
	case *tg.AuthSentCodeTypeFragmentSMS:
		where = "via Fragment, open " + t.URL
	case *tg.AuthSentCodeTypeFirebaseSMS:
		where = "by SMS that needs Google Play verification, which tgtui cannot do (try resend)"
	case *tg.AuthSentCodeTypeSetUpEmailRequired:
		where = "nowhere yet, Telegram requires a login email to be set up"
	default:
		where = fmt.Sprintf("via %T", t)
	}
	msg := "Code sent " + where + "."
	if next, ok := s.GetNextType(); ok {
		msg += fmt.Sprintf("\nResend would use %s", strings.TrimPrefix(fmt.Sprintf("%T", next), "*tg.AuthCodeType"))
		if t, ok := s.GetTimeout(); ok {
			msg += fmt.Sprintf(" (available in %ds)", t)
		}
		msg += "."
	} else {
		msg += "\nTelegram offers no other delivery, resend will not help."
	}
	return msg
}

func readLine(r *bufio.Reader) (string, error) {
	s, err := r.ReadString('\n')
	return strings.TrimSpace(s), err
}

// readPassword reads the 2FA password without echo so it does not show on screen.
func readPassword() (string, error) {
	fmt.Print("2FA password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(pw)), nil
}

// randomID makes a random int64 for outgoing messages.
func randomID() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return int64(binary.LittleEndian.Uint64(b[:]))
}
