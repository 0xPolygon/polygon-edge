package stakemanager

import (
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

type stakeManagerDeployParams struct {
	accountDir        string
	accountConfig     string
	privateKey        string
	jsonRPC           string
	genesisPath       string
	stakeTokenAddress string
	isTestMode        bool
}

func (s *stakeManagerDeployParams) validateFlags() error {
	if !s.isTestMode && s.privateKey == "" {
		return sidechainHelper.ValidateSecretFlags(s.accountDir, s.accountConfig)
	}

	if _, err := os.Stat(s.genesisPath); err != nil {
		return fmt.Errorf("provided genesis path '%s' is invalid. Error: %w ", s.genesisPath, err)
	}

	// validate jsonrpc address
	_, err := helper.ParseJSONRPCAddress(s.jsonRPC)

	return err
}
