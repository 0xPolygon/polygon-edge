package e2e

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/types"
)

const (
	dataDirFlag            = "data-dir"
	registratorDataDirFlag = "registrator-data-dir"
	balanceFlag            = "balance"
	stakeFlag              = "stake"
)

type registerParams struct {
	newValidatorDataDir         string
	registratorValidatorDataDir string
	jsonRPCAddr                 string
	balance                     string
	stake                       string
}

func (rp *registerParams) validateFlags() error {
	if _, err := os.Stat(rp.newValidatorDataDir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided new validator data directory '%s' doesn't exist", rp.newValidatorDataDir)
	}

	if _, err := os.Stat(rp.registratorValidatorDataDir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided registrator validator data directory '%s' doesn't exist", rp.registratorValidatorDataDir)
	}

	balance, err := types.ParseUint256orHex(&rp.balance)
	if err != nil {
		return fmt.Errorf("provided balance '%s' isn't valid", rp.balance)
	}

	stake, err := types.ParseUint256orHex(&rp.stake)
	if err != nil {
		return fmt.Errorf("provided stake '%s' isn't valid", rp.stake)
	}

	if stake.Cmp(balance) > 0 {
		return fmt.Errorf("provided stake is greater than funded balance (stake=%s balance=%s)",
			stake.String(), balance.String())
	}

	return nil
}
