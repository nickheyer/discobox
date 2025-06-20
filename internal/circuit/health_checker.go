package circuit

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"net/http"
	"sync/atomic"
	
	"discobox/internal/types"
)


// healthChecker implements active and passive health checking
type healthChecker struct {
	interval      time.Duration
	timeout       time.Duration
	failThreshold int
	passThreshold int
	logger        types.Logger
	client        *http.Client
	mu            sync.RWMutex
	healthStatus  map[string]*healthInfo
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

type healthInfo struct {
	healthy           bool
	consecutiveFails  int32
	consecutivePass   int32
	lastCheck         time.Time
	lastError         error
	checkInProgress   int32
	totalChecks       int64
	totalFailures     int64
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(interval, timeout time.Duration, failThreshold, passThreshold int, logger types.Logger) types.HealthChecker {
	return &healthChecker{
		interval:      interval,
		timeout:       timeout,
		failThreshold: failThreshold,
		passThreshold: passThreshold,
		logger:        logger,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Don't follow redirects
			},
		},
		healthStatus: make(map[string]*healthInfo),
		stopCh:       make(chan struct{}),
	}
}

// Check performs a health check on the server
func (hc *healthChecker) Check(ctx context.Context, server *types.Server) error {
	// Skip if check is already in progress
	info := hc.getOrCreateHealthInfo(server.ID)
	if !atomic.CompareAndSwapInt32(&info.checkInProgress, 0, 1) {
		return nil
	}
	defer atomic.StoreInt32(&info.checkInProgress, 0)
	
	// Build health check URL
	healthURL := server.URL.String()
	if server.Metadata["health_path"] != "" {
		healthURL += server.Metadata["health_path"]
	} else {
		healthURL += "/health"
	}
	
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return hc.recordFailure(server.ID, err)
	}
	
	// Perform health check
	start := time.Now()
	resp, err := hc.client.Do(req)
	duration := time.Since(start)
	
	if err != nil {
		return hc.recordFailure(server.ID, err)
	}
	defer resp.Body.Close()
	
	// Check status code
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		hc.recordSuccess(server.ID)
		hc.logger.Debug("health check passed",
			"server_id", server.ID,
			"url", healthURL,
			"status", resp.StatusCode,
			"duration", duration,
		)
		return nil
	}
	
	err = fmt.Errorf("unhealthy status: %d", resp.StatusCode)
	return hc.recordFailure(server.ID, err)
}

// Watch continuously monitors server health
func (hc *healthChecker) Watch(ctx context.Context, server *types.Server, interval time.Duration) <-chan error {
	errCh := make(chan error, 1)
	
	if interval <= 0 {
		interval = hc.interval
	}
	
	hc.wg.Add(1)
	go func() {
		defer hc.wg.Done()
		defer close(errCh)
		
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		// Initial check
		if err := hc.Check(ctx, server); err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-hc.stopCh:
				return
			case <-ticker.C:
				if err := hc.Check(ctx, server); err != nil {
					select {
					case errCh <- err:
					default:
					}
				}
			}
		}
	}()
	
	return errCh
}

// RecordSuccess records a successful request (for passive checks)
func (hc *healthChecker) RecordSuccess(serverID string) {
	hc.recordSuccess(serverID)
}

// RecordFailure records a failed request (for passive checks)
func (hc *healthChecker) RecordFailure(serverID string, err error) {
	hc.recordFailure(serverID, err)
}

// getOrCreateHealthInfo gets or creates health info for a server
func (hc *healthChecker) getOrCreateHealthInfo(serverID string) *healthInfo {
	hc.mu.RLock()
	info, exists := hc.healthStatus[serverID]
	hc.mu.RUnlock()
	
	if exists {
		return info
	}
	
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	// Double-check
	if info, exists := hc.healthStatus[serverID]; exists {
		return info
	}
	
	info = &healthInfo{
		healthy:   true,
		lastCheck: time.Now(),
	}
	hc.healthStatus[serverID] = info
	
	return info
}

// recordSuccess records a successful health check
func (hc *healthChecker) recordSuccess(serverID string) {
	info := hc.getOrCreateHealthInfo(serverID)
	
	atomic.StoreInt32(&info.consecutiveFails, 0)
	atomic.AddInt32(&info.consecutivePass, 1)
	atomic.AddInt64(&info.totalChecks, 1)
	
	hc.mu.Lock()
	info.lastCheck = time.Now()
	info.lastError = nil
	
	// Mark as healthy if threshold is met
	if atomic.LoadInt32(&info.consecutivePass) >= int32(hc.passThreshold) && !info.healthy {
		info.healthy = true
		hc.logger.Info("server marked healthy",
			"server_id", serverID,
			"consecutive_pass", info.consecutivePass,
		)
	}
	hc.mu.Unlock()
}

// recordFailure records a failed health check
func (hc *healthChecker) recordFailure(serverID string, err error) error {
	info := hc.getOrCreateHealthInfo(serverID)
	
	atomic.StoreInt32(&info.consecutivePass, 0)
	atomic.AddInt32(&info.consecutiveFails, 1)
	atomic.AddInt64(&info.totalChecks, 1)
	atomic.AddInt64(&info.totalFailures, 1)
	
	hc.mu.Lock()
	info.lastCheck = time.Now()
	info.lastError = err
	
	// Mark as unhealthy if threshold is met
	if atomic.LoadInt32(&info.consecutiveFails) >= int32(hc.failThreshold) && info.healthy {
		info.healthy = false
		hc.logger.Warn("server marked unhealthy",
			"server_id", serverID,
			"consecutive_fails", info.consecutiveFails,
			"error", err,
		)
	}
	hc.mu.Unlock()
	
	return err
}

// IsHealthy returns whether a server is healthy
func (hc *healthChecker) IsHealthy(serverID string) bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	
	if info, exists := hc.healthStatus[serverID]; exists {
		return info.healthy
	}
	
	// Default to healthy if not tracked
	return true
}

// GetHealthStatus returns detailed health status for a server
func (hc *healthChecker) GetHealthStatus(serverID string) map[string]interface{} {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	
	info, exists := hc.healthStatus[serverID]
	if !exists {
		return map[string]interface{}{
			"healthy": true,
			"tracked": false,
		}
	}
	
	status := map[string]interface{}{
		"healthy":            info.healthy,
		"consecutive_fails":  atomic.LoadInt32(&info.consecutiveFails),
		"consecutive_pass":   atomic.LoadInt32(&info.consecutivePass),
		"last_check":         info.lastCheck,
		"total_checks":       atomic.LoadInt64(&info.totalChecks),
		"total_failures":     atomic.LoadInt64(&info.totalFailures),
		"tracked":            true,
	}
	
	if info.lastError != nil {
		status["last_error"] = info.lastError.Error()
	}
	
	return status
}

// Stop stops all health check watchers
func (hc *healthChecker) Stop() {
	close(hc.stopCh)
	hc.wg.Wait()
}

// PassiveHealthChecker tracks health based on request success/failure
type PassiveHealthChecker struct {
	mu            sync.RWMutex
	healthStatus  map[string]*passiveHealthInfo
	failThreshold int
	window        time.Duration
}

type passiveHealthInfo struct {
	failures      []time.Time
	lastFailure   time.Time
	healthy       bool
}

// NewPassiveHealthChecker creates a passive health checker
func NewPassiveHealthChecker(failThreshold int, window time.Duration) *PassiveHealthChecker {
	return &PassiveHealthChecker{
		healthStatus:  make(map[string]*passiveHealthInfo),
		failThreshold: failThreshold,
		window:        window,
	}
}

// RecordSuccess records a successful request
func (p *PassiveHealthChecker) RecordSuccess(serverID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if info, exists := p.healthStatus[serverID]; exists {
		// Clear failures on success
		info.failures = nil
		info.healthy = true
	}
}

// RecordFailure records a failed request
func (p *PassiveHealthChecker) RecordFailure(serverID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	info, exists := p.healthStatus[serverID]
	if !exists {
		info = &passiveHealthInfo{
			healthy:  true,
			failures: make([]time.Time, 0),
		}
		p.healthStatus[serverID] = info
	}
	
	now := time.Now()
	info.lastFailure = now
	
	// Remove old failures outside the window
	cutoff := now.Add(-p.window)
	newFailures := make([]time.Time, 0, len(info.failures)+1)
	for _, t := range info.failures {
		if t.After(cutoff) {
			newFailures = append(newFailures, t)
		}
	}
	newFailures = append(newFailures, now)
	info.failures = newFailures
	
	// Check if threshold is exceeded
	if len(info.failures) >= p.failThreshold {
		info.healthy = false
	}
}

// IsHealthy returns whether a server is healthy
func (p *PassiveHealthChecker) IsHealthy(serverID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if info, exists := p.healthStatus[serverID]; exists {
		return info.healthy
	}
	
	return true
}
