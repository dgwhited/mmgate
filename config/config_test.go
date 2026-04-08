package config

import (
	"os"
	"path/filepath"
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
