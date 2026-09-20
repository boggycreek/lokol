package version

// Version is the current semantic version of lokol.
// In CI release builds, this is overridden via:
// -ldflags "-X github.com/boggycreek/lokol/pkg/version.Version=v0.1.0"
var (
	Version   = "v0.1.0-alpha"
	GitCommit = "dev"
	BuildDate = "unknown"
)
