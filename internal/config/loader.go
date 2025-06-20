package config

import (
	"fmt"
	"os"
	"strings"
	
	"github.com/spf13/viper"
	"path/filepath"
	
	"discobox/internal/types"
)

// Loader handles configuration loading
type Loader struct {
	configPath string
	logger     types.Logger
}

// NewLoader creates a new configuration loader
func NewLoader(configPath string, logger types.Logger) *Loader {
	return &Loader{
		configPath: configPath,
		logger:     logger,
	}
}

// LoadConfig loads configuration from file or environment
func (l *Loader) LoadConfig() (*types.ProxyConfig, error) {
	// Setup viper
	if l.configPath != "" {
		viper.SetConfigFile(l.configPath)
	} else {
		// Look for config in standard locations
		viper.SetConfigName("discobox")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/discobox/")
		viper.AddConfigPath("$HOME/.discobox")
	}
	
	// Enable environment variables
	viper.SetEnvPrefix("DISCOBOX")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	
	// Set defaults
	setDefaults()
	
	// Read configuration
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			l.logger.Warn("No config file found, using defaults and environment")
		} else {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	} else {
		l.logger.Info("Loaded configuration", "file", viper.ConfigFileUsed())
	}
	
	// Unmarshal configuration
	var cfg types.ProxyConfig
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Validate configuration
	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return &cfg, nil
}

// LoadFromBytes loads configuration from byte array (for testing)
func LoadFromBytes(data []byte, format string) (*types.ProxyConfig, error) {
	viper.SetConfigType(format)
	
	// Set defaults
	setDefaults()
	
	// Read from bytes
	if err := viper.ReadConfig(strings.NewReader(string(data))); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	
	// Unmarshal configuration
	var cfg types.ProxyConfig
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Validate configuration
	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return &cfg, nil
}

// SaveConfig saves the configuration to file
func (l *Loader) SaveConfig(cfg *types.ProxyConfig) error {
	// Validate before saving
	if err := Validate(cfg); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	
	// Ensure directory exists
	dir := filepath.Dir(l.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	// Write configuration
	if err := viper.WriteConfigAs(l.configPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	
	l.logger.Info("Saved configuration", "file", l.configPath)
	return nil
}
