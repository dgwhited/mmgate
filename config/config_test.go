package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
clients:
  - id: "test"
    secret: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
    allowed_paths: ["/hooks/*"]
    rate_limit: 60
`
	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(cfg.Clients))
	}
	if cfg.Clients[0].ID != "test" {
		t.Errorf("expected client id 'test', got %s", cfg.Clients[0].ID)
	}

	// Check defaults
	if cfg.Server.ListenAddr != ":8080" {
		t.Errorf("expected default listen addr :8080, got %s", cfg.Server.ListenAddr)
	}
	if cfg.Upstream.URL != "http://localhost:8065" {
		t.Errorf("expected default upstream URL, got %s", cfg.Upstream.URL)
	}
	if cfg.Security.TimestampTolerance != 30 {
		t.Errorf("expected default timestamp tolerance 30, got %d", cfg.Security.TimestampTolerance)
	}
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	t.Setenv("TEST_SECRET", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4")

	content := `
clients:
  - id: "test"
    secret: "${TEST_SECRET}"
    allowed_paths: ["/hooks/*"]
`
	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Clients[0].Secret != "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4" {
		t.Errorf("expected expanded secret, got %s", cfg.Clients[0].Secret)
	}
}

func TestLoad_NoClients(t *testing.T) {
	content := `
server:
  listen_addr: ":9090"
`
	path := writeTemp(t, content)
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for empty clients")
	}
}

func TestLoad_MissingSecret(t *testing.T) {
	content := `
clients:
  - id: "test"
    secret: ""
    allowed_paths: ["/hooks/*"]
`
	path := writeTemp(t, content)
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for missing secret")
	}
}

func TestLoad_ShortSecret(t *testing.T) {
	content := `
clients:
  - id: "test"
    secret: "tooshort"
    allowed_paths: ["/hooks/*"]
`
	path := writeTemp(t, content)
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for short secret")
	}
}

func TestLoad_MissingPaths(t *testing.T) {
	content := `
clients:
  - id: "test"
    secret: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
`
	path := writeTemp(t, content)
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for missing allowed_paths")
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// validClientYAML builds a config body with a single valid client, so each
// test below can vary exactly one field.
func validClientYAML(extra string) string {
	return extra + `
clients:
  - id: "only"
    secret: "0123456789abcdef0123456789abcdef"
    allowed_paths: ["/hooks/*"]
`
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_RejectsDuplicateClientID(t *testing.T) {
	path := writeConfig(t, `
clients:
  - id: "dupe"
    secret: "0123456789abcdef0123456789abcdef"
    allowed_paths: ["/hooks/*"]
  - id: "dupe"
    secret: "ffffffffffffffffffffffffffffffff"
    allowed_paths: ["/api/v4/posts"]
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected duplicate id to be rejected")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("error should name the problem, got: %v", err)
	}
}

// Two clients sharing a secret makes auth.MatchClient ambiguous: it returns
// whichever client comes first, silently misattributing requests, audit log
// lines and rate-limit buckets.
func TestLoad_RejectsDuplicateSecret(t *testing.T) {
	path := writeConfig(t, `
clients:
  - id: "a"
    secret: "0123456789abcdef0123456789abcdef"
    allowed_paths: ["/hooks/*"]
  - id: "b"
    secret: "0123456789abcdef0123456789abcdef"
    allowed_paths: ["/api/v4/posts"]
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected duplicate secret to be rejected")
	}
	if !strings.Contains(err.Error(), "secrets must be unique") {
		t.Errorf("error should name the problem, got: %v", err)
	}
}

func TestLoad_RejectsInvalidLoggingEnums(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"bad level", validClientYAML("logging:\n  level: \"verbose\"\n")},
		{"bad format", validClientYAML("logging:\n  format: \"xml\"\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, tt.yaml)); err == nil {
				t.Error("expected invalid logging enum to be rejected")
			}
		})
	}
}

func TestLoad_RejectsNonPositiveNumerics(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"negative tolerance", validClientYAML("security:\n  timestamp_tolerance: -1\n")},
		{"negative max_body_bytes", validClientYAML("server:\n  max_body_bytes: -1\n")},
		{"negative max_header_bytes", validClientYAML("server:\n  max_header_bytes: -1\n")},
		{"negative read_timeout", validClientYAML("server:\n  read_timeout: -5s\n")},
		{"negative idle_timeout", validClientYAML("server:\n  idle_timeout: -5s\n")},
		{"negative upstream timeout", validClientYAML("upstream:\n  timeout: -5s\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, tt.yaml)); err == nil {
				t.Error("expected non-positive value to be rejected")
			}
		})
	}
}

func TestLoad_RejectsNegativeRateLimit(t *testing.T) {
	path := writeConfig(t, `
clients:
  - id: "only"
    secret: "0123456789abcdef0123456789abcdef"
    allowed_paths: ["/hooks/*"]
    rate_limit: -5
`)
	if _, err := Load(path); err == nil {
		t.Error("expected negative rate_limit to be rejected")
	}
}

// The new server timeout knobs must get non-zero defaults, otherwise the
// http.Server would fall back to "no timeout" and reintroduce the slowloris
// exposure these fields exist to close.
func TestLoad_ServerTimeoutDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, validClientYAML("")))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.ReadHeaderTimeout <= 0 {
		t.Error("read_header_timeout must default to a positive value")
	}
	if cfg.Server.IdleTimeout <= 0 {
		t.Error("idle_timeout must default to a positive value")
	}
	if cfg.Server.MaxHeaderBytes <= 0 {
		t.Error("max_header_bytes must default to a positive value")
	}
}
