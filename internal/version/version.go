package version

import (
	"fmt"
	"runtime"
	"time"
)

// Build information set at compile time via ldflags
var (
	// Version is the semantic version
	Version = "dev"
	
	// GitCommit is the git commit hash
	GitCommit = "unknown"
	
	// BuildTime is the build timestamp
	BuildTime = "unknown"
	
	// GoVersion is the Go version used to build
	GoVersion = runtime.Version()
	
	// Platform is the OS/Arch combination
	Platform = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)

// Info contains version information
type Info struct {
	Version   string    `json:"version"`
	GitCommit string    `json:"git_commit"`
	BuildTime string    `json:"build_time"`
	GoVersion string    `json:"go_version"`
	Platform  string    `json:"platform"`
	StartTime time.Time `json:"start_time"`
	Uptime    string    `json:"uptime"`
}

// startTime tracks when the application started
var startTime = time.Now()

// GetInfo returns version information
func GetInfo() Info {
	uptime := time.Since(startTime)
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildTime: BuildTime,
		GoVersion: GoVersion,
		Platform:  Platform,
		StartTime: startTime,
		Uptime:    formatDuration(uptime),
	}
}

// String returns a formatted version string
func String() string {
	return fmt.Sprintf("Discobox %s (commit: %s, built: %s)", Version, GitCommit, BuildTime)
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}