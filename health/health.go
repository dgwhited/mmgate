package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Handler struct {
	upstreamURL string
	healthPath  string
	httpClient  *http.Client
}

func NewHandler(upstreamURL, healthPath string, timeout time.Duration) *Handler {
	return &Handler{
		upstreamURL: upstreamURL,
		healthPath:  healthPath,
		httpClient: &http.Client{
			Timeout: timeout,
		},
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
	resp, err := h.httpClient.Get(h.upstreamURL + h.healthPath)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
			"error":  fmt.Sprintf("upstream check failed: %v", err),
		})
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	} else {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
			"error":  fmt.Sprintf("upstream returned %d", resp.StatusCode),
		})
	}
}
