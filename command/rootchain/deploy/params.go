package deploy

import (
	"errors"
	"fmt"
	"os"
)

const (
	genesisPathFlag = "genesis"
	deployerKeyFlag = "deployer-key"
	jsonRPCFlag     = "json-rpc"
	erc20AddrFlag   = "erc20-token"
	erc721AddrFlag  = "erc721-token"
	erc1155AddrFlag = "erc1155-token"

	defaultGenesisPath = "./genesis.json"
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
	if _, err := os.Stat(ip.genesisPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided genesis path '%s' doesn't exist", ip.genesisPath)
	}

	return nil
}
