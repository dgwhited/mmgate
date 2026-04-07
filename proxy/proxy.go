package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/dgwhited/mmgate/auth"
)

// New creates a reverse proxy handler that forwards requests from /proxy/*
// to the upstream Mattermost server, stripping the /proxy prefix.
func New(upstreamURL string, timeout time.Duration) (http.Handler, error) {
	target, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, err
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Strip /proxy prefix
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = strings.TrimPrefix(req.URL.Path, "/proxy")
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}

			// Set Host header to upstream
			req.Host = target.Host

			// Strip bridge-specific headers
			req.Header.Del(auth.HeaderSignature)
			req.Header.Del(auth.HeaderTimestamp)

			// Set X-Forwarded-For if not already present
			if req.Header.Get("X-Forwarded-For") == "" {
				if addr := req.RemoteAddr; addr != "" {
					// RemoteAddr is "IP:port", extract just IP
					if idx := strings.LastIndex(addr, ":"); idx != -1 {
						addr = addr[:idx]
					}
					req.Header.Set("X-Forwarded-For", addr)
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			slog.Debug("upstream response",
				"status", resp.StatusCode,
				"path", resp.Request.URL.Path,
			)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("proxy error",
				"path", r.URL.Path,
				"error", err,
			)
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		},
		Transport: &http.Transport{
			ResponseHeaderTimeout: timeout,
		},
	}

	return proxy, nil
}
