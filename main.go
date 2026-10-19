package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"

	"github.com/0443n/tgtui/internal/config"
	"github.com/0443n/tgtui/internal/tg"
	"github.com/0443n/tgtui/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tgtui:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(bakedEnv)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	// Restore default handling after the first Ctrl+C so a second one kills a stuck dial.
	go func() { <-ctx.Done(); cancel() }()

	client, err := tg.New(cfg)
	if err != nil {
		return err
	}

	// `tgtui auth` logs in on the terminal.
	if len(os.Args) > 1 && os.Args[1] == "auth" {
		if cfg.Proxy != "" {
			shown := cfg.Proxy
			if u, err := url.Parse(cfg.Proxy); err == nil {
				shown = u.Redacted()
			}
			fmt.Println("Connecting to Telegram via proxy", shown, "...")
		} else {
			fmt.Println("Connecting to Telegram directly (no TGTUI_PROXY set) ...")
		}
		return client.Authenticate(ctx)
	}

	// Default: run the TUI, connect in the background.
	p := tea.NewProgram(ui.New().WithProxy(cfg.Proxy), tea.WithAltScreen(), tea.WithMouseCellMotion())

	go func() {
		err := client.Run(ctx, func(sess *tg.Session) {
			p.Send(ui.ConnectedMsg{Session: sess, Ctx: ctx})
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			p.Send(ui.FatalMsg{Err: err})
		}
	}()

	// Forward live updates into the UI.
	go func() {
		for ev := range client.Events() {
			switch e := ev.(type) {
			case tg.TypingEvent:
				p.Send(ui.TypingMsg{ChatID: e.ChatID, Name: e.Name})
			case tg.StatusEvent:
				p.Send(ui.StatusMsg{UserID: e.UserID, Status: e.Status, Online: e.Online})
			case tg.MessageEvent:
				p.Send(ui.IncomingMsg{ChatID: e.ChatID, Incoming: e.Incoming, Msg: e.Msg})
			case tg.ReadEvent:
				p.Send(ui.ReadMsg{ChatID: e.ChatID, Unread: e.Unread})
			case tg.OutboxReadEvent:
				p.Send(ui.OutboxReadMsg{ChatID: e.ChatID, MaxID: e.MaxID})
			}
		}
	}()

	_, err = p.Run()
	return err
}
