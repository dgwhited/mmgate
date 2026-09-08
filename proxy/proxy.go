package proxy

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/dgwhited/mmgate/auth"
)

// New creates a reverse proxy handler that forwards requests to the upstream
// Mattermost server.
//
// The caller is responsible for stripping the /proxy prefix before the request
// reaches this handler (see http.StripPrefix in main.go). StripPrefix rewrites
// URL.Path and URL.RawPath together, which is why it is preferred over doing
// the trim inside the rewrite hook.
func New(upstreamURL string, timeout time.Duration) (http.Handler, error) {
	target, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("parsing upstream url: %w", err)
	}
	// url.Parse is permissive: it accepts "file:///etc/passwd" and bare
	// hostnames with no scheme. Since this value decides where authenticated
	// traffic is sent, constrain it explicitly.
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("upstream url scheme must be http or https (got %q)", target.Scheme)
	}
	if target.Host == "" {
		return nil, fmt.Errorf("upstream url must include a host (got %q)", upstreamURL)
	}

	proxy := &httputil.ReverseProxy{
		// Rewrite supersedes the deprecated Director hook. SetURL joins the
		// target and inbound paths (handling Path/RawPath consistently), and
		// SetXForwarded derives the peer address via net.SplitHostPort, so
		// IPv6 peers are no longer mangled and a caller-supplied
		// X-Forwarded-For can no longer stand in for the real peer.
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
			pr.Out.Host = target.Host

			// Bridge credentials terminate here; never forward them upstream.
			pr.Out.Header.Del(auth.HeaderSignature)
			pr.Out.Header.Del(auth.HeaderTimestamp)
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
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: timeout,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			// Only relevant for https upstreams; harmless otherwise.
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}

	return proxy, nil
}
