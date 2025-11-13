// Package config wires CLI and environment configuration for the DNS server.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"resolvit/pkg/version"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config contains all runtime options required by the resolvit server.
type Config struct {
	Upstreams   []string
	Listen      string
	ResolveFrom string
	LogLevel    string
	LogFile     string
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

func validateArgs() error {
	if err := ValidateLogLevel(viper.GetString("log-level")); err != nil {
		return err
	}

	if resolveFile := viper.GetString("resolve-from"); resolveFile != "" {
		if _, err := os.Stat(resolveFile); err != nil {
			return fmt.Errorf("resolve-from file not accessible: %w", err)
		}
	}

	upstreams := viper.GetStringSlice("upstream")
	for _, addr := range upstreams {
		if err := ValidateAddress(ParseUpstream(addr)); err != nil {
			return fmt.Errorf("invalid upstream address %s: %w", addr, err)
		}
	}

	if err := ValidateAddress(viper.GetString("listen")); err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}

	return nil
}

// Setup configures Cobra/Viper, parses CLI flags, and produces a Config instance.
func Setup() (*Config, error) {
	var rootCmd = &cobra.Command{
		Use:   "resolvit",
		Short: "A DNS server with local record resolving",
		Long: `A DNS server that allows you to resolve specific DNS records locally
while forwarding all other requests to upstream DNS servers.`,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			return validateArgs()
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
	ApplyExitOnHelp(rootCmd, 0)

	rootCmd.PersistentFlags().StringSlice("upstream", []string{"9.9.9.9:53"}, "Upstream DNS server (can specify multiple)")
	rootCmd.PersistentFlags().String("listen", "127.0.0.1:5300", "Listen address for DNS server")
	rootCmd.PersistentFlags().String("resolve-from", "", "File containing DNS records to resolve locally")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-file", "stdout", "Log file path (stdout for console)")
	rootCmd.PersistentFlags().Bool("version", false, "Print version and exit")

	err := viper.BindPFlags(rootCmd.PersistentFlags())
	if err != nil {
		return nil, err
	}

	if err := rootCmd.Execute(); err != nil {
		return nil, err
	}

	if viper.GetBool("version") {
		fmt.Printf("resolvit version %s\n", version.ResolvitVersion)
		os.Exit(0)
	}

	upstreams := viper.GetStringSlice("upstream")
	parsedUpstreams := make([]string, len(upstreams))
	for i, addr := range upstreams {
		parsedUpstreams[i] = ParseUpstream(addr)
	}

	config := &Config{
		Upstreams:   parsedUpstreams,
		Listen:      viper.GetString("listen"),
		ResolveFrom: viper.GetString("resolve-from"),
		LogLevel:    viper.GetString("log-level"),
		LogFile:     viper.GetString("log-file"),
	}

	return config, nil
}

// ApplyExitOnHelp exits with the provided code after Cobra prints help text.
func ApplyExitOnHelp(c *cobra.Command, exitCode int) {
	helpFunc := c.HelpFunc()
	c.SetHelpFunc(func(c *cobra.Command, s []string) {
		helpFunc(c, s)
		os.Exit(exitCode)
	})
}
