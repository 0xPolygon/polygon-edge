package licenses

import (
	_ "embed"

	"encoding/json"
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
	bsdLicenses     []DepLicense
)

func SetLicense(license string) {
	License = license
}

func GetBSDLicenses() ([]DepLicense, error) {
	if bsdLicenses == nil {
		if err := json.Unmarshal([]byte(bsdLicensesJSON), &bsdLicenses); err != nil {
			return nil, err
		}
	}

	return bsdLicenses, nil
}
