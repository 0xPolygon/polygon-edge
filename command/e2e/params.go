package e2e

import (
	"errors"
)

const registerFlag = "register"

var errInvalidDir = errors.New("no data directory passed in")

type registerParams struct {
	testDir string
}

func (rp *registerParams) validateFlags() error {
	if rp.testDir == "" {
		return errInvalidDir
	}

	return nil
}
