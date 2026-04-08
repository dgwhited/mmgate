package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Handler struct {
	upstreamURL string
	healthPath  string
	httpClient  *http.Client
	cacheTTL    time.Duration

	mu          sync.RWMutex
	cachedReady bool
	cachedErr   string
	cachedAt    time.Time
}

func NewHandler(upstreamURL, healthPath string, timeout time.Duration) *Handler {
	return &Handler{
		upstreamURL: upstreamURL,
		healthPath:  healthPath,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		cacheTTL: 5 * time.Second,
	}
}

func writeJSON(w http.ResponseWriter, status int, data map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	if time.Since(h.cachedAt) < h.cacheTTL {
		ready, errMsg := h.cachedReady, h.cachedErr
		h.mu.RUnlock()
		if ready {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		} else {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable",
				"error":  errMsg,
			})
		}
		return
	}
	h.mu.RUnlock()

	resp, err := h.httpClient.Get(h.upstreamURL + h.healthPath)

	h.mu.Lock()
	defer h.mu.Unlock()

	if err != nil {
		h.cachedReady = false
		h.cachedErr = fmt.Sprintf("upstream check failed: %v", err)
		h.cachedAt = time.Now()
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
			"error":  h.cachedErr,
		})
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		h.cachedReady = true
		h.cachedErr = ""
		h.cachedAt = time.Now()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	} else {
		h.cachedReady = false
		h.cachedErr = fmt.Sprintf("upstream returned %d", resp.StatusCode)
		h.cachedAt = time.Now()
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
			"error":  h.cachedErr,
		})
	}
}
