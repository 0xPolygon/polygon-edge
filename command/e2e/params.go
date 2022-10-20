package e2e

import (
	"errors"
	"fmt"
	"os"
)

const (
	dataDirFlag            = "data-dir"
	registratorDataDirFlag = "registrator-data-dir"
)

type registerParams struct {
	newValidatorDataDir         string
	registratorValidatorDataDir string
}

func (rp *registerParams) validateFlags() error {
	if _, err := os.Stat(rp.newValidatorDataDir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided new validator data directory '%s' doesn't exist", rp.newValidatorDataDir)
	}

	if _, err := os.Stat(rp.registratorValidatorDataDir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided registrator validator data directory '%s' doesn't exist", rp.registratorValidatorDataDir)
	}

	return nil
}
