package initcontracts

import (
	"errors"
	"fmt"
	"os"
)

const (
	manifestPathFlag   = "manifest"
	deployerKeyFlag    = "deployer-key"
	jsonRPCFlag        = "json-rpc"
	rootchainERC20Flag = "rootchain-erc20"

	defaultManifestPath = "./manifest.json"
)

type initContractsParams struct {
	manifestPath       string
	deployerKey        string
	jsonRPCAddress     string
	rootERC20TokenAddr string
	isTestMode         bool
}

func (ip *initContractsParams) validateFlags() error {
	if _, err := os.Stat(ip.manifestPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided manifest path '%s' doesn't exist", ip.manifestPath)
	}

	return nil
}
