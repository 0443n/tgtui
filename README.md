# tgtui

A minimal two-pane Telegram client for the terminal. Single static binary, plain
text UI, works over SSH and on small ARM boxes.

## Install

Grab the binary for your system from the latest release
(`tgtui-linux-amd64`, `tgtui-linux-arm64`, `tgtui-darwin-amd64`,
`tgtui-darwin-arm64`) and put it on your PATH:

```sh
chmod +x tgtui-linux-amd64
mv tgtui-linux-amd64 ~/.local/bin/tgtui
```

## Usage

Log in once, then run it:

```sh
tgtui auth
tgtui
```

The login code arrives as a message from "Telegram" in your other sessions
(phone or desktop app), not by SMS. The session is kept in the OS keyring, or in
`~/.config/tgtui/session.json` when no keyring is available.

### Proxy

If Telegram is blocked on your network, point tgtui at a SOCKS5 proxy, either as
an env var or in `~/.config/tgtui/config.env`:

```sh
TGTUI_PROXY=socks5://user:pass@host:1080
```

## Keys

Chat list on the left, open chat on the right.

| key                     | action                             |
|-------------------------|------------------------------------|
| `j`/`k`, arrows         | move (previews the chat)           |
| `[` / `]`               | switch chat folder                 |
| `/`                     | search the chat list by name       |
| `enter`                 | open + mark read                   |
| `i` / `tab`             | write a message                    |
| type + `enter`          | send                               |
| `up` (empty input)      | edit your last message             |
| `ctrl+up` / `ctrl+down` | pick a message to reply to         |
| `v`                     | select messages in the open chat   |
| `space` (select mode)   | mark a message (multi-select)      |
| `d` (select mode)       | delete: `e` everyone / `m` just me |
| `f` (select mode)       | forward: pick a chat, `enter`      |
| `s`                     | search messages in the open chat   |
| `c`                     | contact/profile of the open user   |
| `r` / `b` / `d` (in `c`)| rename / block / delete contact    |
| `ctrl+p`                | play the latest voice/video note   |
| `ctrl+s` / `ctrl+x`     | stop playback / toggle 2x speed    |
| `?`                     | settings / status / key help       |
| `esc`                   | cancel edit/reply/select/search    |
| `pgup`/`pgdn`, wheel    | scroll (pgup pages in older)       |
| `q` / `ctrl+q`          | quit                               |

To copy text, hold `Shift` and drag with the mouse.

Voice and video notes play through `mpv` or `ffplay` (also `cvlc`, `afplay`,
`paplay`). Desktop notifications use `notify-send` on Linux and
`terminal-notifier` or `osascript` on macOS.

## Building from source

```sh
cp .env.example .env   # fill in your own api_id / api_hash from my.telegram.org
go build
```
