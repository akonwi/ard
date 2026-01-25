package version

// Version is set at build time via -ldflags
var Version = "dev"

// Get returns the version string
func Get() string {
	return Version
}
