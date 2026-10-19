package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnv(t *testing.T) {
	m := parseEnv("# a comment\nexport TGTUI_API_ID=42\napi_hash = \"abc\"\nproxy = socks5://h:1\n\ngarbage line\n")
	if m["TGTUI_API_ID"] != "42" {
		t.Errorf("export prefix not stripped: %q", m["TGTUI_API_ID"])
	}
	if m["api_hash"] != "abc" {
		t.Errorf("quotes/spaces not handled: %q", m["api_hash"])
	}
	if m["proxy"] != "socks5://h:1" {
		t.Errorf("value with : mishandled: %q", m["proxy"])
	}
	if _, ok := m["garbage line"]; ok {
		t.Error("line without = should be ignored")
	}
}

// writeConfig points os.UserConfigDir at a temp dir and drops a config file in
// it, then clears the TGTUI_* env so only the file (and any baked arg) is read.
func writeConfig(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tgtui"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tgtui", name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir) // os.UserConfigDir honours this on Linux
	t.Setenv("TGTUI_API_ID", "")
	t.Setenv("TGTUI_API_HASH", "")
	t.Setenv("TGTUI_PROXY", "")
}

// The proxy in the config file is applied - the bug the user hit.
func TestLoadReadsProxyFromConfigFile(t *testing.T) {
	writeConfig(t, "config.env", "TGTUI_API_ID=42\nTGTUI_API_HASH=hash\nTGTUI_PROXY=socks5://127.0.0.1:10808\n")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIID != 42 || cfg.APIHash != "hash" {
		t.Fatalf("creds = %d/%q", cfg.APIID, cfg.APIHash)
	}
	if cfg.Proxy != "socks5://127.0.0.1:10808" {
		t.Fatalf("proxy not applied from config file: %q", cfg.Proxy)
	}
}

// A legacy extensionless "config" with bare keys still works.
func TestLoadLegacyFileAndKeys(t *testing.T) {
	writeConfig(t, "config", "api_id = 7\napi_hash = h\nproxy = socks5://x:9\n")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIID != 7 || cfg.Proxy != "socks5://x:9" {
		t.Fatalf("legacy keys not honoured: %d %q", cfg.APIID, cfg.Proxy)
	}
}

// Baked creds win over env and file; the proxy is never baked, so it still comes
// from env/file.
func TestBakedCredsAuthoritativeProxyNot(t *testing.T) {
	writeConfig(t, "config.env", "TGTUI_API_ID=1\nTGTUI_API_HASH=fromfile\nTGTUI_PROXY=socks5://f:1\n")
	t.Setenv("TGTUI_API_ID", "999")
	t.Setenv("TGTUI_API_HASH", "fromenv")
	cfg, err := Load("TGTUI_API_ID=42\nTGTUI_API_HASH=baked\nTGTUI_PROXY=socks5://baked:1\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIID != 42 || cfg.APIHash != "baked" {
		t.Fatalf("baked creds should win: %d/%q", cfg.APIID, cfg.APIHash)
	}
	if cfg.Proxy != "socks5://f:1" {
		t.Fatalf("proxy must come from file, never baked: %q", cfg.Proxy)
	}
}
