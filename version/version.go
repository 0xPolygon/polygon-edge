package version

import (
	_ "embed"
)

var (
	// Version is the main version at the moment.
	// Embedded by --ldflags on build time
	// Versioning should follow the SemVer guidelines
	// https://semver.org/
	Version = "v0.1.0"

	License string
)

func SetLicense(license string) {
	License = license
}
