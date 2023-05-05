package deploy

import (
	"fmt"
	"os"
)

const (
	deployerKeyFlag = "deployer-key"
	jsonRPCFlag     = "json-rpc"
	erc20AddrFlag   = "erc20-token"
	erc721AddrFlag  = "erc721-token"
	erc1155AddrFlag = "erc1155-token"
)

type deployParams struct {
	genesisPath          string
	deployerKey          string
	jsonRPCAddress       string
	rootERC20TokenAddr   string
	rootERC721TokenAddr  string
	rootERC1155TokenAddr string
	isTestMode           bool
}

func (ip *deployParams) validateFlags() error {
	if _, err := os.Stat(ip.genesisPath); err != nil {
		return fmt.Errorf("provided genesis path '%s' is invalid. Error: %w ", ip.genesisPath, err)
	}

	return nil
}
