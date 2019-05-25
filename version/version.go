package version

import "fmt"

var (
	// GitCommit is the git commit that was compiled.
	GitCommit string

	// Version is the main version at the moment.
	Version = "0.1.0"

	// VersionPrerelease is a marker for the version.
	VersionPrerelease = "dev"
)

// GetVersion returns a string representation of the version
func GetVersion() string {
	version := Version

	release := VersionPrerelease
	if release != "" {
		version += fmt.Sprintf("-%s", release)

		if GitCommit != "" {
			version += fmt.Sprintf(" (%s)", GitCommit)
		}
	}

	return version
}
