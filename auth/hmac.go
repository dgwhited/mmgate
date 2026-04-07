package auth

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderSignature = "X-Bridge-Signature"
	HeaderTimestamp = "X-Bridge-Timestamp"
	signaturePrefix = "sha256="
)

type HMACMiddleware struct {
	clients            []*Client
	timestampTolerance time.Duration
	maxBodyBytes       int64
}

func NewHMACMiddleware(clients []*Client, toleranceSec int, maxBodyBytes int64) *HMACMiddleware {
	return &HMACMiddleware{
		clients:            clients,
		timestampTolerance: time.Duration(toleranceSec) * time.Second,
		maxBodyBytes:       maxBodyBytes,
	}
}

func (m *HMACMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader := r.Header.Get(HeaderSignature)
		tsHeader := r.Header.Get(HeaderTimestamp)

		if sigHeader == "" || tsHeader == "" {
			http.Error(w, "missing signature or timestamp header", http.StatusUnauthorized)
			return
		}

		// Parse and validate timestamp
		ts, err := strconv.ParseInt(tsHeader, 10, 64)
		if err != nil {
			http.Error(w, "invalid timestamp", http.StatusBadRequest)
			return
		}

		now := time.Now().Unix()
		drift := time.Duration(math.Abs(float64(now-ts))) * time.Second
		if drift > m.timestampTolerance {
			slog.Warn("request timestamp outside tolerance",
				"drift_seconds", drift.Seconds(),
				"tolerance_seconds", m.timestampTolerance.Seconds(),
			)
			http.Error(w, "timestamp outside tolerance window", http.StatusUnauthorized)
			return
		}

		// Parse signature
		if !strings.HasPrefix(sigHeader, signaturePrefix) {
			http.Error(w, "signature must start with sha256=", http.StatusBadRequest)
			return
		}
		signature := strings.TrimPrefix(sigHeader, signaturePrefix)

		// Read body (with size limit)
		body, err := io.ReadAll(io.LimitReader(r.Body, m.maxBodyBytes+1))
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		if int64(len(body)) > m.maxBodyBytes {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		// Build signing string: <timestamp>.<method>.<path_with_query>.<body>
		pathWithQuery := r.URL.Path
		if r.URL.RawQuery != "" {
			pathWithQuery = r.URL.Path + "?" + r.URL.RawQuery
		}
		signingString := fmt.Sprintf("%s.%s.%s.%s", tsHeader, r.Method, pathWithQuery, string(body))

		// Match client by secret
		client := MatchClient(m.clients, signingString, signature)
		if client == nil {
			slog.Warn("no client matched signature", "path", r.URL.Path)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		// Check path authorization (strip /proxy prefix for matching)
		mmPath := strings.TrimPrefix(r.URL.Path, "/proxy")
		if !client.IsPathAllowed(mmPath) {
			slog.Warn("client not authorized for path",
				"client", client.ID,
				"path", mmPath,
			)
			http.Error(w, "path not allowed", http.StatusForbidden)
			return
		}

		slog.Debug("request authenticated", "client", client.ID, "path", mmPath)

		// Restore body for downstream handlers
		r.Body = io.NopCloser(strings.NewReader(string(body)))

		// Set client in context
		ctx := ContextWithClient(r.Context(), client)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
