package initcontracts

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
)

const (
	manifestPathFlag   = "manifest"
	jsonRPCFlag        = "json-rpc"
	rootchainERC20Flag = "rootchain-erc20"

	defaultManifestPath = "./manifest.json"
)

type initContractsParams struct {
	manifestPath       string
	accountDir         string
	accountConfig      string
	jsonRPCAddress     string
	rootERC20TokenAddr string
	isTestMode         bool
}

func (ip *initContractsParams) validateFlags() error {
	if err := helper.ValidateSecretFlags(ip.isTestMode, ip.accountDir, ip.accountConfig); err != nil {
		return err
	}

	if _, err := os.Stat(ip.manifestPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided manifest path '%s' doesn't exist", ip.manifestPath)
	}

	return nil
}
