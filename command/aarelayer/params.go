package aarelayer

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/spf13/cobra"
)

const (
	addrFlag    = "addr"
	chainIDFlag = "chain-id"

	defaultPort = 8198
)

type aarelayerParams struct {
	addr       string
	accountDir string
	configPath string
	chainID    int64
}

func (rp *aarelayerParams) validateFlags() error {
	if !helper.ValidateIPPort(rp.addr) {
		return fmt.Errorf("invalid address: %s", rp.addr)
	}

	return sidechainHelper.ValidateSecretFlags(rp.accountDir, rp.configPath)
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.addr,
		addrFlag,
		fmt.Sprintf("%s:%d", helper.AllInterfacesBinding, defaultPort),
		"rest server address [ip:port]",
	)

	cmd.Flags().StringVar(
		&params.accountDir,
		polybftsecrets.DataPathFlag,
		"",
		polybftsecrets.DataPathFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.configPath,
		polybftsecrets.ConfigFlag,
		"",
		polybftsecrets.ConfigFlagDesc,
	)

	cmd.Flags().Int64Var(
		&params.chainID,
		chainIDFlag,
		command.DefaultChainID,
		"the ID of the chain",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.ConfigFlag, polybftsecrets.DataPathFlag)
}
