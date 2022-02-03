package version

import (
	_ "embed"

	"encoding/json"
	"fmt"
	"os"
)

type DepLicense struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
	Path    string `json:"path"`
}

var (
	// Version is the main version at the moment.
	// Embedded by --ldflags on build time
	// Versioning should follow the SemVer guidelines
	// https://semver.org/
	Version string

	License string

	//go:embed bsd_licenses.json
	bsdLicensesJSON string
	BsdLicenses     []DepLicense
)

func init() {
	if err := json.Unmarshal([]byte(bsdLicensesJSON), &BsdLicenses); err != nil {
		fmt.Printf("failed to parse bsd_licenses.json")
		os.Exit(1)
	}
}

func SetLicense(license string) {
	License = license
}
