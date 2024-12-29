package app

import (
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
)

// Config defines the application configuration.
type Config struct {
	// Name of the application. Use only lowercase letters, dashes and underscores. No spaces.
	Name    string                  `mapstructure:"name"`
	NATS    NATSConfig              `mapstructure:"nats"`
	HTTP    HTTPConfig              `mapstructure:"http"`
	Modules map[string]ModuleConfig `mapstructure:"modules"`
}

// NATSConfig holds NATS-specific configuration
type NATSConfig struct {
	// URL of the NATS server. Optional. Ignored if Embedded is true
	URL string `mapstructure:"url"`
	// Should the NATS server be embedded? If so, the URL is ignored. Default: true
	Embedded bool `mapstructure:"embedded"`
	// Should the NATS server connection be over IPC (Inter-process Communication)? If so, external NATS client cannot connect
	// Applicable only if Embedded is true.
	Private bool `mapstructure:"private"`
	// Should the NATS server logs get printed?
	Logging bool `mapstructure:"logging"`
}

// HTTPConfig holds HTTP-specific configuration
type HTTPConfig struct {
	Port int `mapstructure:"port"`
}

// ModuleConfig defines the configuration for each module
type ModuleConfig struct {
	Enabled bool           `mapstructure:"enabled"`
	Config  map[string]any `mapstructure:"config"`
}

// LoadConfig loads the configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// Set default values
	v.SetDefault("name", "upspeak")
	v.SetDefault("nats.embedded", true)
	v.SetDefault("nats.private", false)
	v.SetDefault("nats.logging", true)
	v.SetDefault("nats.url", nats.DefaultURL)
	v.SetDefault("http.port", 8080)

	// Configuration file settings
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml") // or "json", "toml" etc.

	// Enable environment variables
	v.AutomaticEnv()
	v.SetEnvPrefix("UPSPEAK") // APP_NAME, APP_NATS_URL, etc.

	// Read configuration
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found; ignore error if desired
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	return &config, nil
}
