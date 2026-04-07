package config

import (
	"fmt"
	"os"
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
	ListenAddr   string        `yaml:"listen_addr"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	MaxBodyBytes int64         `yaml:"max_body_bytes"`
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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
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
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30 * time.Second
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
		cfg.Security.TimestampTolerance = 300
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
}

func validate(cfg *Config) error {
	if len(cfg.Clients) == 0 {
		return fmt.Errorf("at least one client must be configured")
	}
	for i, c := range cfg.Clients {
		if c.ID == "" {
			return fmt.Errorf("client[%d]: id is required", i)
		}
		if c.Secret == "" {
			return fmt.Errorf("client[%d] (%s): secret is required", i, c.ID)
		}
		if len(c.AllowedPaths) == 0 {
			return fmt.Errorf("client[%d] (%s): at least one allowed_path is required", i, c.ID)
		}
	}
	return nil
}
