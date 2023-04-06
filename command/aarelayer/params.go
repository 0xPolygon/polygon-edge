package aarelayer

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"
)

const (
	addrFlag        = "addr"
	dbPathFlag      = "db-path"
	chainIDFlag     = "chain-id"
	invokerAddrFlag = "invoker-addr"
	logPathFlag     = "log-path"
	logLevelFlag    = "log-level"
	eip3074HashFlag = "hash-eip3074"

	defaultLogLevel = "info"
	defaultPort     = 8198
)

type aarelayerParams struct {
	addr        string
	dbPath      string
	accountDir  string
	configPath  string
	chainID     int64
	invokerAddr string
	logPath     string
	logLevel    string
	eip3074Hash bool
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

	if rp.invokerAddr == "" {
		return errors.New("address of invoker smart contract not specified")
	}

	return sidechainHelper.ValidateSecretFlags(rp.accountDir, rp.configPath)
}

func (rp *aarelayerParams) getLogger() (hclog.Logger, error) {
	if rp.logPath != "" {
		logFileWriter, err := os.Create(rp.logPath)
		if err != nil {
			return nil, fmt.Errorf("could not create log file, %w", err)
		}

		return hclog.New(&hclog.LoggerOptions{
			Name:       "aarelayer",
			Output:     logFileWriter,
			Level:      hclog.LevelFromString(rp.logLevel),
			JSONFormat: false,
		}), nil
	}

	return hclog.New(&hclog.LoggerOptions{
		Name:       "aarelayer",
		Level:      hclog.LevelFromString(rp.logLevel),
		JSONFormat: false,
	}), nil
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
		polybftsecrets.AccountDirFlag,
		"",
		polybftsecrets.AccountDirFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.configPath,
		polybftsecrets.AccountConfigFlag,
		"",
		polybftsecrets.AccountConfigFlagDesc,
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
		"",
		"address of the invoker smart contract",
	)

	cmd.Flags().StringVar(
		&params.logPath,
		logPathFlag,
		"",
		"path for file logger",
	)

	cmd.Flags().StringVar(
		&params.logLevel,
		logLevelFlag,
		defaultLogLevel,
		"logger log level",
	)

	cmd.Flags().BoolVar(
		&params.eip3074Hash,
		eip3074HashFlag,
		true,
		"enable EIP-3074 hashing",
	)

	helper.RegisterJSONRPCFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountConfigFlag, polybftsecrets.AccountDirFlag)
}
