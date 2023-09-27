package deploy

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
)

const (
	deployerKeyFlag = "deployer-key"
	jsonRPCFlag     = "json-rpc"
	erc20AddrFlag   = "erc20-token"
)

type deployParams struct {
	genesisPath         string
	deployerKey         string
	jsonRPCAddress      string
	stakeTokenAddr      string
	rootERC20TokenAddr  string
	stakeManagerAddr    string
	proxyContractsAdmin string
	isTestMode          bool
}

func (ip *deployParams) validateFlags() error {
	var err error

	if _, err = os.Stat(ip.genesisPath); err != nil {
		return fmt.Errorf("provided genesis path '%s' is invalid. Error: %w ", ip.genesisPath, err)
	}

	consensusCfg, err = polybft.LoadPolyBFTConfig(ip.genesisPath)
	if err != nil {
		return err
	}

	if consensusCfg.NativeTokenConfig == nil {
		return errors.New("native token configuration is undefined")
	}

	// when using mintable native token, child native token on root chain gets mapped automatically
	if consensusCfg.NativeTokenConfig.IsMintable && ip.rootERC20TokenAddr != "" {
		return errors.New("if child chain native token is mintable, root native token must not pre-exist on root chain")
	}

	if params.stakeTokenAddr == "" {
		return errors.New("stake token address is not provided")
	}

	return helper.ValidateProxyContractsAdmin(ip.proxyContractsAdmin)
}
