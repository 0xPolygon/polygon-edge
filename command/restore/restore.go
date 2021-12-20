package restore

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/consensus"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/server"
	"github.com/0xPolygon/polygon-sdk/state"
	itrie "github.com/0xPolygon/polygon-sdk/state/immutable-trie"
	"github.com/0xPolygon/polygon-sdk/state/runtime/evm"
	"github.com/0xPolygon/polygon-sdk/state/runtime/precompiled"
	"github.com/hashicorp/go-hclog"
)

type RestoreCommand struct {
	helper.Base
	Formatter *helper.FormatterFlag
}

// DefineFlags defines the command flags
func (c *RestoreCommand) DefineFlags() {
	c.Base.DefineFlags(c.Formatter)
}

// GetHelperText returns a simple description of the command
func (c *RestoreCommand) GetHelperText() string {
	return "Import backup blockchain data"
}

func (c *RestoreCommand) GetBaseCommand() string {
	return "restore"
}

// Help implements the cli.Command interface
func (c *RestoreCommand) Help() string {
	c.DefineFlags()

	c.FlagMap["data"] = helper.FlagDescriptor{
		Description: "Filepath the path of the backup data",
		Arguments: []string{
			"BACKUP_FILE",
		},
		ArgumentsOptional: false,
	}

	c.FlagMap["chain"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the genesis file used for starting the chain. Default: %s", helper.DefaultConfig().Chain),
		Arguments: []string{
			"GENESIS_FILE",
		},
		FlagOptional: true,
	}

	c.FlagMap["data-dir"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the data directory used for storing Polygon SDK client data. Default: %s", helper.DefaultConfig().DataDir),
		Arguments: []string{
			"DATA_DIRECTORY",
		},
		FlagOptional: true,
	}

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.FlagMap), c.FlagMap)
}

// Synopsis implements the cli.Command interface
func (c *RestoreCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *RestoreCommand) Run(args []string) int {
	flags := c.Base.NewFlagSet(c.GetBaseCommand(), c.Formatter)

	var backupFile, chain, dataDir string
	flags.StringVar(&backupFile, "data", "", "")
	flags.StringVar(&chain, "chain", helper.DefaultConfig().Chain, "")
	flags.StringVar(&dataDir, "data-dir", helper.DefaultConfig().DataDir, "")

	if err := flags.Parse(args); err != nil {
		c.Formatter.OutputError(err)
		return 1
	}

	blockchain, err := c.initializeBlockchain(chain, dataDir)
	if err != nil {
		c.Formatter.OutputError(err)
		return 1
	}

	from, to, err := ImportChain(blockchain, backupFile)
	if err != nil {
		c.Formatter.OutputError(err)
		return 1
	}

	res := &RestoreResult{
		File: backupFile,
		Num:  to - from + 1,
		From: from,
		To:   to,
	}
	c.Formatter.OutputResult(res)

	return 0
}

func (c *RestoreCommand) initializeBlockchain(genesisFilePath, dataDirPath string) (*blockchain.Blockchain, error) {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "restore",
		Level: hclog.LevelFromString("INFO"),
	})

	// Grab the default server config
	conf := server.DefaultConfig()

	// Decode the chain
	cc, err := chain.Import(genesisFilePath)
	if err != nil {
		return nil, err
	}
	conf.Chain = cc
	conf.DataDir = dataDirPath

	if err := common.SetupDataDir(dataDirPath, []string{"blockchain", "trie"}); err != nil {
		return nil, fmt.Errorf("failed to create data directories: %v", err)
	}

	stateStorage, err := itrie.NewLevelDBStorage(filepath.Join(dataDirPath, "trie"), logger)
	if err != nil {
		return nil, err
	}
	st := itrie.NewState(stateStorage)

	executor := state.NewExecutor(cc.Params, st, logger)
	executor.SetRuntime(precompiled.NewPrecompiled())
	executor.SetRuntime(evm.NewEVM())

	genesisRoot := executor.WriteGenesis(cc.Genesis.Alloc)
	cc.Genesis.StateRoot = genesisRoot

	bc, err := blockchain.NewBlockchain(logger, dataDirPath, cc, nil, executor)
	if err != nil {
		return nil, err
	}
	executor.GetHash = bc.GetHashHelper

	consensus, err := server.SetupConsensus(conf, nil, nil, bc, executor, nil, nil, logger, consensus.NilMetrics())
	if err != nil {
		return nil, err
	}
	bc.SetConsensus(consensus)

	if err := bc.ComputeGenesis(); err != nil {
		return nil, err
	}

	if err := consensus.Initialize(); err != nil {
		return nil, err
	}

	return bc, nil
}

type RestoreResult struct {
	File string `json:"file"`
	Num  uint64 `json:"num"`
	From uint64 `json:"from,omitempty"`
	To   uint64 `json:"to,omitempty"`
}

func (r *RestoreResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[RESTORE]\n")
	if r.From != 0 && r.To != 0 {
		buffer.WriteString("Imported blockchain data successfully\n")
		buffer.WriteString(helper.FormatKV([]string{
			fmt.Sprintf("File|%s", r.File),
			fmt.Sprintf("Number|%d", r.Num),
			fmt.Sprintf("From|%d", r.From),
			fmt.Sprintf("To|%d", r.To),
		}))
	} else {
		buffer.WriteString("No blocks are imported\n")
	}

	return buffer.String()
}
