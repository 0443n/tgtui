package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	tgc "github.com/0443n/tgtui/internal/tg"
)

// voicePath is the cache/temp file for a chat's voice or video note. Keying it
// by chat and message id lets a second play reuse the download.
func voicePath(chatID int64, msg tgc.Message) string {
	ext := ".ogg"
	switch {
	case strings.Contains(msg.Mime, "mp4"):
		ext = ".mp4"
	case strings.Contains(msg.Mime, "mpeg"), strings.Contains(msg.Mime, "mp3"):
		ext = ".mp3"
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("tgtui-%d-%d%s", chatID, msg.ID, ext))
}

// fileReady reports whether a cached download already exists and is non-empty.
func fileReady(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 0
}

// speedLabel formats a playback speed for display, e.g. 2 -> "2", 1.5 -> "1.5".
func speedLabel(sp float64) string {
	return strconv.FormatFloat(sp, 'g', -1, 64)
}

// audioState owns the single background player process, so it can be stopped or
// restarted (e.g. to change speed). It is a package singleton.
type audioState struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	path    string
	label   string
	speed   float64
	playing bool
}

var audio = &audioState{speed: 1}

// status reports the current playback for the UI to show.
func (a *audioState) status() (playing bool, label string, speed float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.playing, a.label, a.speed
}

// play starts a file, replacing anything already playing.
func (a *audioState) play(path, label string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.path, a.label = path, label
	return a.startLocked()
}

// stop kills the current player, if any.
func (a *audioState) stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.killLocked()
}

// toggleSpeed switches between 1x and 2x, restarting the current file if one is
// playing. It returns the new speed.
func (a *audioState) toggleSpeed() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.speed == 1 {
		a.speed = 2
	} else {
		a.speed = 1
	}
	if a.playing && a.path != "" {
		_ = a.startLocked()
	}
	return a.speed
}

// killLocked stops the current process; call with mu held.
func (a *audioState) killLocked() {
	if a.cmd != nil && a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
	}
	a.cmd = nil
	a.playing = false
	a.label = ""
}

// startLocked launches the player for a.path at a.speed; call with mu held.
func (a *audioState) startLocked() error {
	a.killLocked()
	bin, args, ok := playerCommand(a.speed, a.path)
	if !ok {
		return fmt.Errorf("no audio player found (install mpv or ffplay)")
	}
	label := a.label
	cmd := exec.Command(bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil // detach from the TUI
	if err := cmd.Start(); err != nil {
		return err
	}
	a.cmd = cmd
	a.playing = true
	a.label = label
	go func() {
		_ = cmd.Wait()
		a.mu.Lock()
		if a.cmd == cmd { // still current (not replaced by a newer play)
			a.cmd = nil
			a.playing = false
			a.label = ""
		}
		a.mu.Unlock()
	}()
	return nil
}

// playerName returns the first installed audio player, or "" if none is found.
// Used by the settings screen to report whether playback will work.
func playerName() string {
	for _, name := range []string{"mpv", "ffplay", "cvlc", "afplay", "paplay"} {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return ""
}

// playerCommand builds a headless player invocation at the given speed.
func playerCommand(speed float64, path string) (bin string, args []string, ok bool) {
	sp := strconv.FormatFloat(speed, 'f', 2, 64)
	for _, c := range []struct {
		name string
		args []string
	}{
		{"mpv", []string{"--no-video", "--really-quiet", "--speed=" + sp, path}},
		{"ffplay", []string{"-nodisp", "-autoexit", "-loglevel", "quiet", "-af", "atempo=" + sp, path}},
		{"cvlc", []string{"--play-and-exit", "--intf", "dummy", "--rate=" + sp, path}},
		{"afplay", []string{"-r", sp, path}},
		{"paplay", []string{path}}, // no speed control
	} {
		if p, err := exec.LookPath(c.name); err == nil {
			return p, c.args, true
		}
	}
	return "", nil, false
}
