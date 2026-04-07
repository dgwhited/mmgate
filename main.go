package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dgwhited/mmgate/auth"
	"github.com/dgwhited/mmgate/config"
	"github.com/dgwhited/mmgate/health"
	"github.com/dgwhited/mmgate/middleware"
	"github.com/dgwhited/mmgate/proxy"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Setup structured logging
	var handler slog.Handler
	level := parseLogLevel(cfg.Logging.Level)
	opts := &slog.HandlerOptions{Level: level}
	if cfg.Logging.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))

	// Build clients
	clients := auth.NewClients(cfg.Clients)
	slog.Info("loaded clients", "count", len(clients))

	// Build reverse proxy
	proxyHandler, err := proxy.New(cfg.Upstream.URL, cfg.Upstream.Timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating proxy: %v\n", err)
		os.Exit(1)
	}

	// Build HMAC middleware
	hmacMiddleware := auth.NewHMACMiddleware(clients, cfg.Security.TimestampTolerance, cfg.Server.MaxBodyBytes)

	// Build health handler
	healthHandler := health.NewHandler(cfg.Upstream.URL, cfg.Upstream.HealthPath, 5*time.Second)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler.Healthz)
	mux.HandleFunc("GET /readyz", healthHandler.Readyz)

	// Rate limiter (applied after HMAC auth identifies the client)
	rateLimiter := middleware.NewRateLimiter()

	// Proxy route: HMAC auth -> rate limit -> proxy
	mux.Handle("/proxy/", hmacMiddleware.Wrap(rateLimiter.Wrap(proxyHandler)))

	// Wrap everything with logging and request ID
	handler2 := middleware.RequestID(middleware.Logging(mux))

	server := &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      handler2,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("starting mmgate", "addr", cfg.Server.ListenAddr, "upstream", cfg.Upstream.URL)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
