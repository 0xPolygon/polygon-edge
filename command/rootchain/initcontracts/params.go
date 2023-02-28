package initcontracts

import (
	"errors"
	"fmt"
	"os"
)

const (
	manifestPathFlag = "manifest"
	jsonRPCFlag      = "json-rpc"
	adminKeyFlag     = "admin-key"

	defaultManifestPath = "./manifest.json"
)

type initContractsParams struct {
	manifestPath   string
	adminKey       string
	jsonRPCAddress string
}

func (ip *initContractsParams) validateFlags() error {
	if _, err := os.Stat(ip.manifestPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided manifest path '%s' doesn't exist", ip.manifestPath)
	}

	return nil
}
