package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Handler struct {
	upstreamURL  string
	healthPath   string
	httpClient   *http.Client
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

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	resp, err := h.httpClient.Get(h.upstreamURL + h.healthPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "unavailable",
			"error":  fmt.Sprintf("upstream check failed: %v", err),
		})
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "unavailable",
			"error":  fmt.Sprintf("upstream returned %d", resp.StatusCode),
		})
	}
}
