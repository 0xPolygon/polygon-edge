package version

var (
	// Version is the main version at the moment.
	// Embedded by --ldflags on build time
	Version = "v0.1.0"
)

// Versioning should follow the SemVer guidelines
// https://semver.org/

// GetVersion returns a string representation of the version
func GetVersion() string {
	return Version
}

// returns jsonrpc representation of the version
func GetVersionJsonrpc() string {
	return Version
}
