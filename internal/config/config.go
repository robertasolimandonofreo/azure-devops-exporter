// Package config loads exporter configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIURL         = "https://dev.azure.com"
	defaultPort           = 8080
	defaultScrapeInterval = 300 * time.Second
	defaultLogLevel       = "info"
)

// Config holds the exporter's runtime configuration.
type Config struct {
	Organization   string
	Projects       []string
	Token          string
	APIURL         string
	Port           int
	ScrapeInterval time.Duration
	LogLevel       string
}

// Load reads configuration from environment variables and validates required fields.
func Load() (*Config, error) {
	cfg := &Config{
		Organization: strings.TrimSpace(os.Getenv("AZURE_DEVOPS_ORGANIZATION")),
		Token:        os.Getenv("AZURE_DEVOPS_TOKEN"),
		APIURL:       envOrDefault("AZURE_DEVOPS_API_URL", defaultAPIURL),
		LogLevel:     envOrDefault("LOG_LEVEL", defaultLogLevel),
	}

	cfg.Projects = parseProjects(os.Getenv("AZURE_DEVOPS_PROJECTS"))

	port, err := envIntOrDefault("EXPORTER_PORT", defaultPort)
	if err != nil {
		return nil, err
	}
	cfg.Port = port

	interval, err := envDurationSecondsOrDefault("SCRAPE_INTERVAL_SECONDS", defaultScrapeInterval)
	if err != nil {
		return nil, err
	}
	cfg.ScrapeInterval = interval

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Organization == "" {
		return fmt.Errorf("AZURE_DEVOPS_ORGANIZATION is required")
	}
	if len(c.Projects) == 0 {
		return fmt.Errorf("AZURE_DEVOPS_PROJECTS is required (comma-separated list)")
	}
	if c.Token == "" {
		return fmt.Errorf("AZURE_DEVOPS_TOKEN is required")
	}
	return nil
}

func parseProjects(raw string) []string {
	parts := strings.Split(raw, ",")
	projects := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			projects = append(projects, p)
		}
	}
	return projects
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOrDefault(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return n, nil
}

func envDurationSecondsOrDefault(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer number of seconds: %w", key, err)
	}
	return time.Duration(n) * time.Second, nil
}
