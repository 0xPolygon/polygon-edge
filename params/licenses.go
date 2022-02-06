package params

import (
	_ "embed"

	"encoding/json"
	"fmt"
	"os"
)

type DepLicense struct {
	Name    string  `json:"name"`
	Version *string `json:"version"`
	Type    string  `json:"type"`
	Path    string  `json:"path"`
}

var (
	// Polygon Edge License
	License string

	// Dependency Licenses
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
