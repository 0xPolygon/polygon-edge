package e2e

import (
	"errors"
)

const dataDirFlag = "data-dir"

var errInvalidDir = errors.New("no data directory passed in")

type registerParams struct {
	dataDirectory string
}

func (rp *registerParams) validateFlags() error {
	if rp.dataDirectory == "" {
		return errInvalidDir
	}

	return nil
}
