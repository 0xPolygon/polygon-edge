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
	manifestPath      string
	secretsDataPath   string
	secretsConfigPath string
	jsonRPCAddress    string
	isTestMode        bool
}

func (ip *initContractsParams) validateFlags() error {
	if !ip.isTestMode {
		if err := sidechain.ValidateSecretFlags(ip.secretsDataPath, ip.secretsConfigPath); err != nil {
			return err
		}
	} else {
		if ip.secretsDataPath != "" || ip.secretsConfigPath != "" {
			return helper.ErrTestModeSecrets
		}
	}

	if _, err := os.Stat(ip.manifestPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided manifest path '%s' doesn't exist", ip.manifestPath)
	}

	return nil
}
