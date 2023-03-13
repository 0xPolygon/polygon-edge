package aarelayer

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/aarelayer/service"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/spf13/cobra"
)

const (
	addrFlag        = "addr"
	dbPathFlag      = "db-path"
	chainIDFlag     = "chain-id"
	invokerAddrFlag = "invoker-addr"

	defaultPort = 8198
)

type aarelayerParams struct {
	addr        string
	dbPath      string
	accountDir  string
	configPath  string
	chainID     int64
	invokerAddr string
}

func (rp *aarelayerParams) validateFlags() error {
	if !helper.ValidateIPPort(rp.addr) {
		return fmt.Errorf("invalid address: %s", rp.addr)
	}

	dir, fn := path.Split(rp.dbPath)
	if dir != "" {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return err
		}
	}

	if fn == "" {
		return errors.New("file name for boltdb not specified")
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
		&params.dbPath,
		dbPathFlag,
		"aa.db",
		"path to bolt db",
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

	cmd.Flags().StringVar(
		&params.invokerAddr,
		invokerAddrFlag,
		service.DefaultAAInvokerAddress.String(),
		"address of invoker smart contract",
	)

	helper.RegisterJSONRPCFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.ConfigFlag, polybftsecrets.DataPathFlag)
}
