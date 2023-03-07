package initcontracts

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
)

const (
	manifestPathFlag = "manifest"
	jsonRPCFlag      = "json-rpc"

	defaultManifestPath = "./manifest.json"
)

type initContractsParams struct {
	manifestPath   string
	accountDir     string
	accountConfig  string
	jsonRPCAddress string
	isTestMode     bool
}

func (ip *initContractsParams) validateFlags() error {
	if !ip.isTestMode {
		if err := sidechain.ValidateSecretFlags(ip.accountDir, ip.accountConfig); err != nil {
			return err
		}
	} else {
		if ip.accountDir != "" || ip.accountConfig != "" {
			return helper.ErrTestModeSecrets
		}
	}

	if _, err := os.Stat(ip.manifestPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided manifest path '%s' doesn't exist", ip.manifestPath)
	}

	return nil
}
