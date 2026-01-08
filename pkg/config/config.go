// Package config loads configuration for the DNS server.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"resolvit/pkg/filtering"
)

const (
	defaultConfigPath = "/etc/resolvit/resolvit.conf"
	configEnvVar      = "RESOLVIT_CONFIG"
)

// Config contains all runtime options required by the resolvit server.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Upstream  UpstreamConfig  `mapstructure:"upstream"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Records   RecordsConfig   `mapstructure:"records"`
	Filtering FilteringConfig `mapstructure:"filtering"`
}

// ServerConfig holds server-level settings.
type ServerConfig struct {
	Listen string `mapstructure:"listen"`
}

// UpstreamConfig holds upstream DNS resolver settings.
type UpstreamConfig struct {
	Servers []string `mapstructure:"servers"`
}

// LoggingConfig holds log settings.
type LoggingConfig struct {
	Level               string `mapstructure:"level"`
	File                string `mapstructure:"file"`
	BlocklistErrorLimit int    `mapstructure:"blocklist_error_limit"`
}

// RecordsConfig holds local record file settings.
type RecordsConfig struct {
	ResolveFrom string `mapstructure:"resolve_from"`
}

// FilteringConfig holds content filtering settings.
type FilteringConfig struct {
	Enabled         bool            `mapstructure:"enabled"`
	CacheDir        string          `mapstructure:"cache_dir"`
	UpdateInterval  time.Duration   `mapstructure:"-"`
	BlockedLog      string          `mapstructure:"blocked_log"`
	BlockSubdomains bool            `mapstructure:"block_subdomains"`
	Allowlist       AllowlistConfig `mapstructure:"allowlist"`
	Custom          CustomConfig    `mapstructure:"custom"`
	Lists           map[string]filtering.ListConfig
}

// AllowlistConfig holds allowlist settings.
type AllowlistConfig struct {
	Path string `mapstructure:"path"`
}

// CustomConfig holds custom blocklist settings.
type CustomConfig struct {
	List []string `mapstructure:"list"`
}

// ValidateLogLevel ensures the user-provided log level matches the supported set.
func ValidateLogLevel(level string) error {
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[strings.ToLower(level)] {
		return fmt.Errorf("invalid log level: %s (must be one of: debug, info, warn, error)", level)
	}
	return nil
}

// ValidateAddress confirms that an address string has a valid host and UDP port.
func ValidateAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if port == "" {
		return errors.New("invalid port")
	}
	if err != nil {
		return fmt.Errorf("invalid address format %s: %w", addr, err)
	}
	if ip := net.ParseIP(host); ip == nil {
		return fmt.Errorf("invalid IP address: %s", host)
	}
	if _, err := net.LookupPort("udp", port); err != nil {
		return fmt.Errorf("invalid port: %s", port)
	}
	return nil
}

// ParseUpstream adds the default DNS port when an upstream is provided without one.
func ParseUpstream(upstream string) string {
	if !strings.Contains(upstream, ":") {
		return upstream + ":53"
	}
	return upstream
}

// Setup loads the TOML configuration file and produces a Config instance.
func Setup() (*Config, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadConfig() (*Config, error) {
	configPath := defaultConfigPath
	if fromEnv := strings.TrimSpace(os.Getenv(configEnvVar)); fromEnv != "" {
		configPath = fromEnv
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	listConfigs, err := parseListConfigs(v)
	if err != nil {
		return nil, err
	}
	cfg.Filtering.Lists = listConfigs

	cfg.Filtering.UpdateInterval, err = parseDuration(v.GetString("filtering.update_interval"))
	if err != nil {
		return nil, fmt.Errorf("invalid filtering.update_interval: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.file", "stdout")
	v.SetDefault("logging.blocklist_error_limit", 20)
	v.SetDefault("filtering.enabled", false)
	v.SetDefault("filtering.cache_dir", "/var/cache/resolvit")
	v.SetDefault("filtering.update_interval", "24h")
	v.SetDefault("filtering.block_subdomains", false)
}

func parseDuration(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	return time.ParseDuration(raw)
}

func validateConfig(cfg *Config) error {
	if err := ValidateLogLevel(cfg.Logging.Level); err != nil {
		return err
	}

	if cfg.Server.Listen == "" {
		return errors.New("server.listen is required")
	}
	if err := ValidateAddress(cfg.Server.Listen); err != nil {
		return fmt.Errorf("invalid server.listen: %w", err)
	}

	if len(cfg.Upstream.Servers) == 0 {
		return errors.New("upstream.servers must contain at least one entry")
	}
	parsedUpstreams := make([]string, len(cfg.Upstream.Servers))
	for i, addr := range cfg.Upstream.Servers {
		parsed := ParseUpstream(addr)
		if err := ValidateAddress(parsed); err != nil {
			return fmt.Errorf("invalid upstream address %s: %w", addr, err)
		}
		parsedUpstreams[i] = parsed
	}
	cfg.Upstream.Servers = parsedUpstreams

	if cfg.Logging.BlocklistErrorLimit < 0 {
		return errors.New("logging.blocklist_error_limit must be >= 0")
	}

	if resolveFile := cfg.Records.ResolveFrom; resolveFile != "" {
		if _, err := os.Stat(resolveFile); err != nil {
			return fmt.Errorf("records.resolve_from not accessible: %w", err)
		}
	}

	return nil
}

func parseListConfigs(v *viper.Viper) (map[string]filtering.ListConfig, error) {
	raw := v.GetStringMap("filtering")
	if len(raw) == 0 {
		return map[string]filtering.ListConfig{}, nil
	}

	ignored := map[string]bool{
		"enabled":          true,
		"cache_dir":        true,
		"update_interval":  true,
		"blocked_log":      true,
		"block_subdomains": true,
		"allowlist":        true,
		"custom":           true,
	}

	listConfigs := make(map[string]filtering.ListConfig)
	for key, value := range raw {
		if ignored[key] {
			continue
		}
		subMap, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("filtering.%s must be a table", key)
		}
		var cfg filtering.ListConfig
		if err := mapstructure.Decode(subMap, &cfg); err != nil {
			return nil, fmt.Errorf("parse filtering.%s: %w", key, err)
		}
		listConfigs[strings.ToLower(key)] = cfg
	}

	return listConfigs, nil
}
