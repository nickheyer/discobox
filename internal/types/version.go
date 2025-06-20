package types

// BuildInfo contains build information
type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

// DefaultBuildInfo is set during build time
var DefaultBuildInfo = BuildInfo{
	Version:   "dev",
	Commit:    "unknown",
	BuildDate: "unknown",
}

// SetBuildInfo updates the build information
func SetBuildInfo(version, commit, buildDate string) {
	DefaultBuildInfo.Version = version
	DefaultBuildInfo.Commit = commit
	DefaultBuildInfo.BuildDate = buildDate
}