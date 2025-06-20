package config

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	
	"discobox/internal/types"
)

// Watcher watches for configuration changes
type Watcher struct {
	loader    *Loader
	logger    types.Logger
	callbacks []func(*types.ProxyConfig)
	mu        sync.RWMutex
	watcher   *fsnotify.Watcher
	stopCh    chan struct{}
	config    *types.ProxyConfig
}

// NewWatcher creates a new configuration watcher
func NewWatcher(loader *Loader, logger types.Logger) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}
	
	return &Watcher{
		loader:    loader,
		logger:    logger,
		callbacks: make([]func(*types.ProxyConfig), 0),
		watcher:   fsWatcher,
		stopCh:    make(chan struct{}),
	}, nil
}

// Start starts watching for configuration changes
func (w *Watcher) Start(ctx context.Context) error {
	// Load initial configuration
	cfg, err := w.loader.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load initial config: %w", err)
	}
	
	w.mu.Lock()
	w.config = cfg
	w.mu.Unlock()
	
	// Add config file to watcher
	configFile := viper.ConfigFileUsed()
	if configFile != "" {
		if err := w.watcher.Add(configFile); err != nil {
			return fmt.Errorf("failed to watch config file: %w", err)
		}
		w.logger.Info("Watching configuration file", "file", configFile)
	}
	
	// Start watching
	go w.watch(ctx)
	
	return nil
}

// Stop stops the configuration watcher
func (w *Watcher) Stop() error {
	close(w.stopCh)
	return w.watcher.Close()
}

// OnChange registers a callback for configuration changes
func (w *Watcher) OnChange(callback func(*types.ProxyConfig)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, callback)
}

// GetConfig returns the current configuration
func (w *Watcher) GetConfig() *types.ProxyConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.config
}

// watch watches for configuration changes
func (w *Watcher) watch(ctx context.Context) {
	// Debounce timer to avoid multiple reloads
	var debounceTimer *time.Timer
	debounceDuration := 500 * time.Millisecond
	
	for {
		select {
		case <-ctx.Done():
			return
			
		case <-w.stopCh:
			return
			
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				w.logger.Debug("Configuration file changed", "file", event.Name, "op", event.Op)
				
				// Reset debounce timer
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				
				debounceTimer = time.AfterFunc(debounceDuration, func() {
					w.reload()
				})
			}
			
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("Configuration watcher error", "error", err)
		}
	}
}

// reload reloads the configuration
func (w *Watcher) reload() {
	w.logger.Info("Reloading configuration")
	
	// Load new configuration
	newCfg, err := w.loader.LoadConfig()
	if err != nil {
		w.logger.Error("Failed to reload configuration", "error", err)
		return
	}
	
	// Update configuration
	w.mu.Lock()
	oldCfg := w.config
	w.config = newCfg
	callbacks := make([]func(*types.ProxyConfig), len(w.callbacks))
	copy(callbacks, w.callbacks)
	w.mu.Unlock()
	
	// Check if configuration actually changed
	if configEqual(oldCfg, newCfg) {
		w.logger.Debug("Configuration unchanged after reload")
		return
	}
	
	// Notify callbacks
	for _, callback := range callbacks {
		go func(cb func(*types.ProxyConfig)) {
			defer func() {
				if r := recover(); r != nil {
					w.logger.Error("Configuration change callback panicked", "error", r)
				}
			}()
			cb(newCfg)
		}(callback)
	}
	
	w.logger.Info("Configuration reloaded successfully")
}

// configEqual compares two configurations for equality
// This is a simplified comparison - in production, you'd want a more thorough comparison
func configEqual(a, b *types.ProxyConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	
	// Compare key fields
	return a.ListenAddr == b.ListenAddr &&
		a.LoadBalancing.Algorithm == b.LoadBalancing.Algorithm &&
		a.RateLimit.Enabled == b.RateLimit.Enabled &&
		a.RateLimit.RPS == b.RateLimit.RPS
}
