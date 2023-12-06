package deploy

import (
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
)

const (
	deployerKeyFlag = "deployer-key"
	jsonRPCFlag     = "json-rpc"
)

type deployParams struct {
	genesisPath         string
	deployerKey         string
	jsonRPCAddress      string
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

	return helper.ValidateProxyContractsAdmin(ip.proxyContractsAdmin)
}
