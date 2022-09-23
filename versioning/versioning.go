package versioning

var (
	// Version is the main version at the moment.
	// Commit is the git commit that the binary was built on
	// BuildTime is the timestamp of the build
	// Embedded by --ldflags on build time
	// Versioning should follow the SemVer guidelines
	// https://semver.org/
	Version   string
	Branch    string
	Commit    string
	BuildTime string
)
