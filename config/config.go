package config

import (
	"fmt"
	"os"
	"slices"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Upstream UpstreamConfig `yaml:"upstream"`
	Security SecurityConfig `yaml:"security"`
	Clients  []ClientConfig `yaml:"clients"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	// ReadHeaderTimeout bounds the time spent reading request headers. This is
	// the specific defence against slowloris-style header dribbling.
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	// IdleTimeout bounds how long a keep-alive connection may sit unused.
	IdleTimeout    time.Duration `yaml:"idle_timeout"`
	MaxHeaderBytes int           `yaml:"max_header_bytes"`
	MaxBodyBytes   int64         `yaml:"max_body_bytes"`
}

type UpstreamConfig struct {
	URL        string        `yaml:"url"`
	Timeout    time.Duration `yaml:"timeout"`
	HealthPath string        `yaml:"health_path"`
}

type SecurityConfig struct {
	TimestampTolerance int `yaml:"timestamp_tolerance"`
}

type ClientConfig struct {
	ID           string   `yaml:"id"`
	Secret       string   `yaml:"secret"`
	AllowedPaths []string `yaml:"allowed_paths"`
	RateLimit    int      `yaml:"rate_limit"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// validLogLevels and validLogFormats are the accepted enum values. Anything
// else is rejected at load time rather than silently falling back to a default
// deep inside the logging setup.
var (
	validLogLevels  = []string{"debug", "info", "warn", "error"}
	validLogFormats = []string{"json", "text"}
)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- config path comes from CLI flag, not user input
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	setDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg.Server.ListenAddr == "" {
		cfg.Server.ListenAddr = ":8080"
	}
	if cfg.Server.ReadHeaderTimeout == 0 {
		cfg.Server.ReadHeaderTimeout = 10 * time.Second
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30 * time.Second
	}
	if cfg.Server.IdleTimeout == 0 {
		cfg.Server.IdleTimeout = 120 * time.Second
	}
	if cfg.Server.MaxHeaderBytes == 0 {
		cfg.Server.MaxHeaderBytes = 1 << 20 // 1MB
	}
	if cfg.Server.MaxBodyBytes == 0 {
		cfg.Server.MaxBodyBytes = 10 * 1024 * 1024 // 10MB
	}
	if cfg.Upstream.URL == "" {
		cfg.Upstream.URL = "http://localhost:8065"
	}
	if cfg.Upstream.Timeout == 0 {
		cfg.Upstream.Timeout = 30 * time.Second
	}
	if cfg.Upstream.HealthPath == "" {
		cfg.Upstream.HealthPath = "/api/v4/system/ping"
	}
	if cfg.Security.TimestampTolerance == 0 {
		cfg.Security.TimestampTolerance = 30
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
}

func validate(cfg *Config) error {
	if cfg.Server.MaxBodyBytes <= 0 {
		return fmt.Errorf("server.max_body_bytes must be positive (got %d)", cfg.Server.MaxBodyBytes)
	}
	if cfg.Server.MaxHeaderBytes <= 0 {
		return fmt.Errorf("server.max_header_bytes must be positive (got %d)", cfg.Server.MaxHeaderBytes)
	}
	for name, d := range map[string]time.Duration{
		"server.read_header_timeout": cfg.Server.ReadHeaderTimeout,
		"server.read_timeout":        cfg.Server.ReadTimeout,
		"server.write_timeout":       cfg.Server.WriteTimeout,
		"server.idle_timeout":        cfg.Server.IdleTimeout,
		"upstream.timeout":           cfg.Upstream.Timeout,
	} {
		if d <= 0 {
			return fmt.Errorf("%s must be positive (got %s)", name, d)
		}
	}
	if cfg.Security.TimestampTolerance <= 0 {
		return fmt.Errorf("security.timestamp_tolerance must be positive (got %d)", cfg.Security.TimestampTolerance)
	}
	if !slices.Contains(validLogLevels, cfg.Logging.Level) {
		return fmt.Errorf("logging.level must be one of %v (got %q)", validLogLevels, cfg.Logging.Level)
	}
	if !slices.Contains(validLogFormats, cfg.Logging.Format) {
		return fmt.Errorf("logging.format must be one of %v (got %q)", validLogFormats, cfg.Logging.Format)
	}

	if len(cfg.Clients) == 0 {
		return fmt.Errorf("at least one client must be configured")
	}

	// Duplicate IDs make logs and rate-limit buckets ambiguous; duplicate
	// secrets make auth.MatchClient ambiguous, since it returns whichever
	// client happens to come first and would misattribute the request.
	seenIDs := make(map[string]int, len(cfg.Clients))
	seenSecrets := make(map[string]int, len(cfg.Clients))

	for i, c := range cfg.Clients {
		if c.ID == "" {
			return fmt.Errorf("client[%d]: id is required", i)
		}
		if j, dup := seenIDs[c.ID]; dup {
			return fmt.Errorf("client[%d] (%s): duplicate id, already used by client[%d]", i, c.ID, j)
		}
		seenIDs[c.ID] = i

		if c.Secret == "" {
			return fmt.Errorf("client[%d] (%s): secret is required", i, c.ID)
		}
		if len(c.Secret) < 32 {
			return fmt.Errorf("client[%d] (%s): secret must be at least 32 characters (got %d)", i, c.ID, len(c.Secret))
		}
		if j, dup := seenSecrets[c.Secret]; dup {
			return fmt.Errorf("client[%d] (%s): secret is identical to client[%d] (%s); secrets must be unique per client",
				i, c.ID, j, cfg.Clients[j].ID)
		}
		seenSecrets[c.Secret] = i

		if len(c.AllowedPaths) == 0 {
			return fmt.Errorf("client[%d] (%s): at least one allowed_path is required", i, c.ID)
		}
		if c.RateLimit < 0 {
			return fmt.Errorf("client[%d] (%s): rate_limit must not be negative (got %d)", i, c.ID, c.RateLimit)
		}
	}
	return nil
}
