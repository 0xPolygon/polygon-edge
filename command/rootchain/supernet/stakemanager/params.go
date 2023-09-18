package stakemanager

import (
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/command/helper"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/types"
)

type stakeManagerDeployParams struct {
	accountDir          string
	accountConfig       string
	privateKey          string
	jsonRPC             string
	genesisPath         string
	stakeTokenAddress   string
	proxyContractsAdmin string
	isTestMode          bool
}

func (s *stakeManagerDeployParams) validateFlags() error {
	if !s.isTestMode {
		// private key is mandatory
		if s.privateKey == "" {
			return sidechainHelper.ValidateSecretFlags(s.accountDir, s.accountConfig)
		}

		// stake token address is mandatory
		if s.stakeTokenAddress == "" {
			return rootHelper.ErrMandatoryStakeToken
		}

		if err := types.IsValidAddress(s.stakeTokenAddress); err != nil {
			return fmt.Errorf("invalid stake token address is provided: %w", err)
		}
	}

	// check if provided genesis path is valid
	if _, err := os.Stat(s.genesisPath); err != nil {
		return fmt.Errorf("provided genesis path '%s' is invalid. Error: %w ", s.genesisPath, err)
	}

	// validate jsonrpc address
	_, err := helper.ParseJSONRPCAddress(s.jsonRPC)
	if err != nil {
		return err
	}

	return helper.ValidateProxyContractsAdmin(s.proxyContractsAdmin)
}
