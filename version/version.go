package version

var (
	// Version is the main version at the moment.
	Version = "0.1.0"
)

// Versioning should follow the SemVer guidelines
// https://semver.org/

// GetVersion returns a string representation of the version
func GetVersion() string {
	version := "\n[POLYGON-SDK VERSION]\n"
	version += Version

	version += "\n"

	return version
}
