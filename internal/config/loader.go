package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"path/filepath"

	"github.com/spf13/viper"

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

	// Update viper values with the new configuration
	viper.Set("load_balancing.algorithm", cfg.LoadBalancing.Algorithm)
	viper.Set("rate_limit.enabled", cfg.RateLimit.Enabled)
	viper.Set("rate_limit.rps", cfg.RateLimit.RPS)
	viper.Set("rate_limit.burst", cfg.RateLimit.Burst)
	viper.Set("circuit_breaker.enabled", cfg.CircuitBreaker.Enabled)
	viper.Set("circuit_breaker.failure_threshold", cfg.CircuitBreaker.FailureThreshold)
	viper.Set("circuit_breaker.success_threshold", cfg.CircuitBreaker.SuccessThreshold)
	viper.Set("circuit_breaker.timeout", cfg.CircuitBreaker.Timeout.String())

	// Write configuration
	if err := viper.WriteConfigAs(l.configPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	l.logger.Info("Saved configuration", "file", l.configPath)
	return nil
}

// LoadBootstrapData loads services, routes, and users from configuration file for bootstrapping
func (l *Loader) LoadBootstrapData(storage types.Storage) error {
	ctx := context.Background()

	// Bootstrap admin user first
	if err := l.bootstrapAdminUser(storage); err != nil {
		l.logger.Error("failed to bootstrap admin user", "error", err)
	}

	// Check if we have services defined in config
	servicesRaw := viper.Get("services")
	if servicesRaw != nil {
		services, ok := servicesRaw.([]any)
		if ok {
			for _, svcRaw := range services {
				svcMap, ok := svcRaw.(map[string]any)
				if !ok {
					continue
				}

				service := &types.Service{}

				// Parse service fields
				if id, ok := svcMap["id"].(string); ok {
					service.ID = id
				}
				if name, ok := svcMap["name"].(string); ok {
					service.Name = name
				}

				// Parse endpoints
				if endpointsRaw, ok := svcMap["endpoints"].([]any); ok {
					for _, ep := range endpointsRaw {
						if endpoint, ok := ep.(string); ok {
							service.Endpoints = append(service.Endpoints, endpoint)
						}
					}
				}

				if healthPath, ok := svcMap["health_path"].(string); ok {
					service.HealthPath = healthPath
				}
				if weight, ok := svcMap["weight"].(int); ok {
					service.Weight = weight
				}
				if maxConns, ok := svcMap["max_conns"].(int); ok {
					service.MaxConns = maxConns
				}
				if stripPrefix, ok := svcMap["strip_prefix"].(bool); ok {
					service.StripPrefix = stripPrefix
				}
				if active, ok := svcMap["active"].(bool); ok {
					service.Active = active
				}

				// Parse timeout
				if timeoutStr, ok := svcMap["timeout"].(string); ok {
					if duration, err := time.ParseDuration(timeoutStr); err == nil {
						service.Timeout = duration
					}
				}

				// Parse metadata
				if metadataRaw, ok := svcMap["metadata"].(map[string]any); ok {
					service.Metadata = make(map[string]string)
					for k, v := range metadataRaw {
						if strVal, ok := v.(string); ok {
							service.Metadata[k] = strVal
						}
					}
				}

				// Check if service exists
				if _, err := storage.GetService(ctx, service.ID); err != nil {
					// Service doesn't exist, create it
					if err := storage.CreateService(ctx, service); err != nil {
						l.logger.Error("failed to create bootstrap service", "id", service.ID, "error", err)
					} else {
						l.logger.Info("created bootstrap service", "id", service.ID, "name", service.Name)
					}
				}
			}
		}
	}

	// Check if we have routes defined in config
	routesRaw := viper.Get("routes")
	if routesRaw != nil {
		routes, ok := routesRaw.([]any)
		if ok {
			for _, routeRaw := range routes {
				routeMap, ok := routeRaw.(map[string]any)
				if !ok {
					continue
				}

				route := &types.Route{}

				// Parse route fields
				if id, ok := routeMap["id"].(string); ok {
					route.ID = id
				}
				if priority, ok := routeMap["priority"].(int); ok {
					route.Priority = priority
				}
				if host, ok := routeMap["host"].(string); ok {
					route.Host = host
				}
				if pathPrefix, ok := routeMap["path_prefix"].(string); ok {
					route.PathPrefix = pathPrefix
				}
				if serviceID, ok := routeMap["service_id"].(string); ok {
					route.ServiceID = serviceID
				}

				// Parse middlewares
				if middlewaresRaw, ok := routeMap["middlewares"].([]any); ok {
					for _, mw := range middlewaresRaw {
						if middleware, ok := mw.(string); ok {
							route.Middlewares = append(route.Middlewares, middleware)
						}
					}
				}

				// Parse metadata
				if metadataRaw, ok := routeMap["metadata"].(map[string]any); ok {
					route.Metadata = metadataRaw
				}

				// Check if route exists
				if _, err := storage.GetRoute(ctx, route.ID); err != nil {
					// Route doesn't exist, create it
					if err := storage.CreateRoute(ctx, route); err != nil {
						l.logger.Error("failed to create bootstrap route", "id", route.ID, "error", err)
					} else {
						l.logger.Info("created bootstrap route", "id", route.ID, "service", route.ServiceID)
					}
				}
			}
		}
	}

	return nil
}

// bootstrapAdminUser creates an admin user if none exists
func (l *Loader) bootstrapAdminUser(storage types.Storage) error {
	ctx := context.Background()

	// Check if any users exist
	users, err := storage.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	// If users exist, skip bootstrap
	if len(users) > 0 {
		l.logger.Info("Users already exist, skipping admin bootstrap")
		return nil
	}

	// Create admin user
	adminUsername := viper.GetString("admin.username")
	if adminUsername == "" {
		adminUsername = "admin"
	}

	adminPassword := viper.GetString("admin.password")
	if adminPassword == "" {
		// Generate a random password if not specified
		adminPassword = generateRandomPassword(16)
		l.logger.Warn("No admin password specified, generated random password", "username", adminUsername, "password", adminPassword)
		l.logger.Warn("IMPORTANT: Please change this password immediately after first login!")
	}

	// Hash password
	hashedPassword, err := hashPassword(adminPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Create admin user
	adminUser := &types.User{
		ID:                 generateID("user"),
		Username:           adminUsername,
		PasswordHash:       hashedPassword,
		Email:              viper.GetString("admin.email"),
		IsAdmin:            true,
		MustChangePassword: true, // Force password change on first login
		Active:             true,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		Metadata: map[string]string{
			"created_by": "bootstrap",
		},
	}

	if err := storage.CreateUser(ctx, adminUser); err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	l.logger.Info("Created bootstrap admin user", "username", adminUsername, "must_change_password", true)

	return nil
}
