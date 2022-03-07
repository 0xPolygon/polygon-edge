package license

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/licenses"
)

type LicenseResult struct {
	BSDLicenses []licenses.DepLicense `json:"bsd_licenses"`
}

func (r *LicenseResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[LICENSE]\n\n")
	buffer.WriteString(licenses.License)

	buffer.WriteString("\n[DEPENDENCY ATTRIBUTIONS]\n\n")

	for idx, l := range r.BSDLicenses {
		// put a blank line between attributions
		if idx != 0 {
			buffer.WriteString("\n")
		}

		name := l.Name
		if l.Version != nil {
			name += " " + *l.Version
		}

		buffer.WriteString(fmt.Sprintf(
			"   This product bundles %s,\n"+
				"   which is available under a \"%s\" license.\n"+
				"   For details, see %s.\n",
			name,
			l.Type,
			l.Path,
		))
	}

	return buffer.String()
}
