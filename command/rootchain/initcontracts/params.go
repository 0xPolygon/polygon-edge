package initcontracts

import (
	"errors"
	"fmt"
	"os"
)

const (
	manifestPathFlag = "manifest"
	deployerKeyFlag  = "deployer-key"
	jsonRPCFlag      = "json-rpc"
	erc20AddrFlag    = "erc20-token"
	erc721AddrFlag   = "erc721-token"
	erc1155AddrFlag  = "erc1155-token"

	defaultManifestPath = "./manifest.json"
)

type initContractsParams struct {
	manifestPath         string
	deployerKey          string
	jsonRPCAddress       string
	rootERC20TokenAddr   string
	rootERC721TokenAddr  string
	rootERC1155TokenAddr string
	isTestMode           bool
}

func (ip *initContractsParams) validateFlags() error {
	if _, err := os.Stat(ip.manifestPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided manifest path '%s' doesn't exist", ip.manifestPath)
	}

	return nil
}
