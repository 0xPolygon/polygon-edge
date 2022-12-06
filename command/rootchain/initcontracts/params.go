package initcontracts

import (
	"errors"
	"fmt"
	"os"
)

const (
	contractsPathFlag = "path"
	genesisPathFlag   = "genesis"
	manifestPathFlag  = "manifest"
	jsonRPCFlag       = "json-rpc"

	defaultGenesisPath  = "./genesis.json"
	defaultManifestPath = "./manifest.json"
)

type initContractsParams struct {
	contractsPath  string
	genesisPath    string
	jsonRPCAddress string
	manifestPath   string
}

func (ip *initContractsParams) validateFlags() error {
	if _, err := os.Stat(ip.contractsPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided smart contracts directory '%s' doesn't exist", ip.contractsPath)
	}

	if _, err := os.Stat(ip.genesisPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided genesis path '%s' doesn't exist", ip.genesisPath)
	}

	return nil
}
