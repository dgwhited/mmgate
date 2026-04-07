package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/dgwhited/mmgate/config"
)

func computeHMAC(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	c := &Client{
		ID:     "test",
		secret: []byte("mysecret"),
	}

	signingString := "1234567890.POST./hooks/abc123.{\"text\":\"hello\"}"
	validSig := computeHMAC(signingString, "mysecret")

	if !c.VerifySignature(signingString, validSig) {
		t.Error("expected valid signature to pass")
	}

	if c.VerifySignature(signingString, "invalidsignature") {
		t.Error("expected invalid signature to fail")
	}

	if c.VerifySignature("different.signing.string.", validSig) {
		t.Error("expected mismatched signing string to fail")
	}
}

func TestIsPathAllowed(t *testing.T) {
	c := &Client{
		allowedPaths: []string{"/hooks/*", "/api/v4/posts"},
	}

	tests := []struct {
		path    string
		allowed bool
	}{
		{"/hooks/abc123", true},
		{"/hooks/xyz", true},
		{"/api/v4/posts", true},
		{"/api/v4/posts/", true},    // trailing slash normalized
		{"/api/v4/channels", false}, // not in allowlist
		{"/hooks", false},           // no wildcard match for bare path
		{"/admin/logs", false},
	}

	for _, tt := range tests {
		got := c.IsPathAllowed(tt.path)
		if got != tt.allowed {
			t.Errorf("IsPathAllowed(%q) = %v, want %v", tt.path, got, tt.allowed)
		}
	}
}

func TestMatchClient(t *testing.T) {
	clients := NewClients([]config.ClientConfig{
		{ID: "client-a", Secret: "secret-a", AllowedPaths: []string{"/hooks/*"}},
		{ID: "client-b", Secret: "secret-b", AllowedPaths: []string{"/api/v4/posts"}},
	})

	signingString := "12345.POST./proxy/hooks/test.body"

	// Should match client-a
	sigA := computeHMAC(signingString, "secret-a")
	matched := MatchClient(clients, signingString, sigA)
	if matched == nil || matched.ID != "client-a" {
		t.Errorf("expected client-a, got %v", matched)
	}

	// Should match client-b
	sigB := computeHMAC(signingString, "secret-b")
	matched = MatchClient(clients, signingString, sigB)
	if matched == nil || matched.ID != "client-b" {
		t.Errorf("expected client-b, got %v", matched)
	}

	// Should match nobody
	matched = MatchClient(clients, signingString, "bogus")
	if matched != nil {
		t.Errorf("expected nil, got %v", matched.ID)
	}
}

func TestContextWithClient(t *testing.T) {
	c := &Client{ID: "test-client"}
	ctx := ContextWithClient(context.Background(), c)
	got := ClientFromContext(ctx)
	if got == nil || got.ID != "test-client" {
		t.Errorf("expected test-client from context, got %v", got)
	}

	// Empty context should return nil
	got = ClientFromContext(context.Background())
	if got != nil {
		t.Errorf("expected nil from empty context, got %v", got)
	}
}
