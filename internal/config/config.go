// Package config loads Defuse configuration from a YAML file with
// environment-variable overrides for secrets.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen  string         `yaml:"listen"`
	Servers []ServerConfig `yaml:"servers"`
	Auth    AuthConfig     `yaml:"auth"`
}

type ServerConfig struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	RCONHost string `yaml:"rcon_host"`
	LogPort  int    `yaml:"log_port"`

	// RCONPassword is loaded from env (RCON_PASSWORD_<UPPER_ID>),
	// not from the YAML file.
	RCONPassword string `yaml:"-"`
}

type AuthConfig struct {
	TrustProxyAuth  bool          `yaml:"trust_proxy_auth"`
	TrustedProxyIPs []string      `yaml:"trusted_proxy_ips"`
	SessionMaxAge   time.Duration `yaml:"session_max_age"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.applyEnv(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyEnv() error {
	for i := range c.Servers {
		s := &c.Servers[i]
		envKey := "RCON_PASSWORD_" + strings.ToUpper(strings.ReplaceAll(s.ID, "-", "_"))
		s.RCONPassword = os.Getenv(envKey)
		if s.RCONPassword == "" {
			return fmt.Errorf("server %q: env %s is required", s.ID, envKey)
		}
	}
	return nil
}

func (c *Config) validate() error {
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if len(c.Servers) == 0 {
		return fmt.Errorf("at least one server must be configured")
	}
	seen := map[string]bool{}
	for _, s := range c.Servers {
		if s.ID == "" {
			return fmt.Errorf("server with empty id")
		}
		if seen[s.ID] {
			return fmt.Errorf("duplicate server id: %s", s.ID)
		}
		seen[s.ID] = true
		if s.RCONHost == "" {
			return fmt.Errorf("server %q: rcon_host is required", s.ID)
		}
	}
	return nil
}
