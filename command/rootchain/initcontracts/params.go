package initcontracts

import (
	"errors"
	"fmt"
	"os"
)

const (
	contractsPathFlag = "path"
	manifestPathFlag  = "manifest"
	jsonRPCFlag       = "json-rpc"
	adminKeyFlag      = "admin-key"

	defaultManifestPath = "./manifest.json"
)

type initContractsParams struct {
	contractsPath  string
	manifestPath   string
	adminKey       string
	jsonRPCAddress string
}

func (ip *initContractsParams) validateFlags() error {
	if _, err := os.Stat(ip.contractsPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided smart contracts directory '%s' doesn't exist", ip.contractsPath)
	}

	if _, err := os.Stat(ip.manifestPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided manifest path '%s' doesn't exist", ip.manifestPath)
	}

	return nil
}
