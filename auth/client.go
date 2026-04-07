package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"

	"github.com/dgwhited/mmgate/config"
)

type Client struct {
	ID           string
	secret       []byte
	allowedPaths []string
	RateLimit    int
}

type contextKey struct{}

func ClientFromContext(ctx context.Context) *Client {
	c, _ := ctx.Value(contextKey{}).(*Client)
	return c
}

func ContextWithClient(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

func NewClients(cfgs []config.ClientConfig) []*Client {
	clients := make([]*Client, len(cfgs))
	for i, cfg := range cfgs {
		clients[i] = &Client{
			ID:           cfg.ID,
			secret:       []byte(cfg.Secret),
			allowedPaths: cfg.AllowedPaths,
			RateLimit:    cfg.RateLimit,
		}
	}
	return clients
}

// VerifySignature checks if the given signature matches the HMAC-SHA256
// of the signing string using this client's secret. Returns true if valid.
func (c *Client) VerifySignature(signingString, signature string) bool {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(signingString))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// IsPathAllowed checks if the given request path matches any of the
// client's allowed path patterns using path.Match glob matching.
func (c *Client) IsPathAllowed(reqPath string) bool {
	// Normalize: strip trailing slash for matching consistency
	reqPath = strings.TrimRight(reqPath, "/")
	for _, pattern := range c.allowedPaths {
		pattern = strings.TrimRight(pattern, "/")
		if matched, _ := path.Match(pattern, reqPath); matched {
			return true
		}
	}
	return false
}

// MatchClient finds the client whose secret validates the given signing string
// and signature. Returns nil if no client matches.
func MatchClient(clients []*Client, signingString, signature string) *Client {
	for _, c := range clients {
		if c.VerifySignature(signingString, signature) {
			return c
		}
	}
	return nil
}
