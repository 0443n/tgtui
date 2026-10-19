// Package config loads tgtui's runtime configuration: Telegram API
// credentials, proxy, and where to keep the login session.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds everything tgtui needs to talk to Telegram.
type Config struct {
	APIID   int
	APIHash string
	Proxy   string // optional socks5://[user:pass@]host:port

	// SessionFile is a 0600 file used only when the OS keyring is not
	// available. The session normally lives in the keyring.
	SessionFile string
}

// Load resolves credentials and the proxy from, in order:
//
//  1. bakedEnv - the .env compiled into the binary via go:embed (see baked.go).
//  2. environment variables TGTUI_API_ID / TGTUI_API_HASH / TGTUI_PROXY.
//  3. the dotenv file at <os.UserConfigDir>/tgtui/config.env (same TGTUI_* keys;
//     legacy bare api_id / api_hash / proxy keys are also accepted).
//
// Baked credentials are authoritative: when they carry an id and hash, env and
// the config file are not consulted for them, period. The proxy is never baked -
// it is a per-machine runtime detail, so it always comes from the env var or the
// config file.
func Load(bakedEnv string) (Config, error) {
	baked := parseEnv(bakedEnv)
	idStr := baked["TGTUI_API_ID"]
	hash := baked["TGTUI_API_HASH"]

	dir := configDir()
	file := readConfigFile(dir)

	if idStr == "" || hash == "" {
		idStr = firstNonEmpty(idStr, os.Getenv("TGTUI_API_ID"), pick(file, "TGTUI_API_ID", "api_id"))
		hash = firstNonEmpty(hash, os.Getenv("TGTUI_API_HASH"), pick(file, "TGTUI_API_HASH", "api_hash"))
	}

	proxy := firstNonEmpty(os.Getenv("TGTUI_PROXY"), pick(file, "TGTUI_PROXY", "proxy"))

	if idStr == "" || hash == "" {
		return Config{}, fmt.Errorf(
			"missing Telegram API credentials.\n"+
				"Make an app at https://my.telegram.org/apps, then either export\n"+
				"  TGTUI_API_ID=... TGTUI_API_HASH=...\n"+
				"or put them in %s as:\n"+
				"  TGTUI_API_ID=12345\n"+
				"  TGTUI_API_HASH=abcdef...",
			filepath.Join(dir, "config.env"),
		)
	}

	id, err := strconv.Atoi(strings.TrimSpace(idStr))
	if err != nil {
		return Config{}, fmt.Errorf("invalid api_id %q: %w", idStr, err)
	}

	return Config{
		APIID:       id,
		APIHash:     strings.TrimSpace(hash),
		Proxy:       proxy,
		SessionFile: filepath.Join(dir, "session.json"),
	}, nil
}

// configDir is <os.UserConfigDir>/tgtui (honours XDG_CONFIG_HOME on Linux). It
// returns "" only when the user config dir cannot be determined.
func configDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tgtui")
}

// readConfigFile reads the dotenv config from the config dir. It prefers
// config.env and falls back to a legacy extensionless "config". A missing dir or
// file yields an empty map.
func readConfigFile(dir string) map[string]string {
	if dir == "" {
		return map[string]string{}
	}
	for _, name := range []string{"config.env", "config"} {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			return parseEnv(string(b))
		}
	}
	return map[string]string{}
}

// parseEnv parses "KEY=VALUE" / "key = value" lines into a map. It ignores blank
// lines, '#' comments, and an optional leading "export ", and strips matching
// surrounding quotes from the value. Used for the embedded .env and the config
// file alike.
func parseEnv(content string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		out[key] = val
	}
	return out
}

// pick returns the first non-empty value among the given keys.
func pick(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := m[k]; v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
