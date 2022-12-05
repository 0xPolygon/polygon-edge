package initcontracts

import (
	"errors"
	"fmt"
	"os"
)

const (
	contractsPathFlag       = "path"
	validatorPrefixPathFlag = "validator-prefix"
	validatorPathFlag       = "validator-path"
	genesisPathFlag         = "genesis-path"
	jsonRPCFlag             = "json-rpc"

	defaultValidatorPrefixPath = "test-chain-"
	defaultValidatorPath       = "./"
	defaultGenesisPath         = "./genesis.json"
)

type initContractsParams struct {
	contractsPath       string
	validatorPrefixPath string
	validatorPath       string
	genesisPath         string
	jsonRPCAddress      string
}

func (ip *initContractsParams) validateFlags() error {
	if _, err := os.Stat(ip.contractsPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided smart contracts directory '%s' doesn't exist", ip.contractsPath)
	}

	if _, err := os.Stat(ip.validatorPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided validators data directory '%s' doesn't exist", ip.validatorPath)
	}

	if _, err := os.Stat(ip.genesisPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided genesis path '%s' doesn't exist", ip.genesisPath)
	}

	return nil
}
