package ui

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// notifiers lists desktop-notification tools to try, best first. Each entry
// builds the argument list for a given title and body.
var notifiers = []struct {
	name string
	args func(title, body string) []string
}{
	{"notify-send", func(title, body string) []string { return []string{"-a", "tgtui", title, body} }},
	{"terminal-notifier", func(title, body string) []string { return []string{"-title", title, "-message", body} }},
	{"osascript", func(title, body string) []string {
		return []string{"-e", "display notification " + quote(body) + " with title " + quote(title)}
	}},
}

// notifierName returns the first installed desktop notifier, or "" if none is
// found. Used by the settings screen to report whether notifications will work.
func notifierName() string {
	for _, n := range notifiers {
		if _, err := exec.LookPath(n.name); err == nil {
			return n.name
		}
	}
	return ""
}

// notifyCmd shows a desktop notification, if a notifier is installed.
func notifyCmd(title, body string) tea.Cmd {
	return func() tea.Msg {
		for _, n := range notifiers {
			bin, err := exec.LookPath(n.name)
			if err != nil {
				continue
			}
			cmd := exec.Command(bin, n.args(title, body)...)
			if cmd.Start() == nil {
				go func() { _ = cmd.Wait() }()
			}
			break
		}
		return nil
	}
}

// quote wraps s in double quotes for an osascript string literal.
func quote(s string) string {
	out := make([]rune, 0, len(s)+2)
	out = append(out, '"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	out = append(out, '"')
	return string(out)
}
