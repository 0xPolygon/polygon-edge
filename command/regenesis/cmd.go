package regenesis

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
)

var (
	params = &regenesisParams{}
)

type regenesisParams struct {
	GenesisPath        string
	TrieDBPath         string
	SnapshotTrieDBPath string
	TrieRoot           string
	JSONRPCAddress     string
	BlockNumber        int64
}

func GetCommand() *cobra.Command {
	genesisCmd := &cobra.Command{
		Use:   "regenesis",
		Short: "Copies trie for specific block to a separate folder",
	}

	genesisCmd.Run = func(cmd *cobra.Command, args []string) {
		outputter := command.InitializeOutputter(genesisCmd)
		defer outputter.WriteOutput()

		if params.SnapshotTrieDBPath == "" || params.TrieDBPath == "" || params.TrieRoot == "" {
			outputter.SetError(fmt.Errorf("not enough arguments"))

			return
		}

		trieDB, err := leveldb.OpenFile(params.TrieDBPath, &opt.Options{ReadOnly: true})
		if err != nil {
			outputter.SetError(fmt.Errorf("open trie trieDB error:%w", err))

			return
		}
		defer trieDB.Close()

		snapshotDB, err := leveldb.OpenFile(params.SnapshotTrieDBPath, nil)
		if err != nil {
			outputter.SetError(fmt.Errorf("open snapshotDB error:%w", err))

			return
		}
		defer snapshotDB.Close()

		snapshotStorage := itrie.NewKV(snapshotDB)

		err = itrie.CopyTrie(types.StringToHash(params.TrieRoot).Bytes(), itrie.NewKV(trieDB), snapshotStorage, nil, false)
		if err != nil {
			outputter.SetError(fmt.Errorf("copy trie error:%w", err))

			return
		}

		checkedHash, err := itrie.HashChecker(types.StringToHash(params.TrieRoot).Bytes(), snapshotStorage)
		if err != nil {
			outputter.SetError(fmt.Errorf("copy trie error:%w", err))

			return
		}

		if checkedHash != types.StringToHash(params.TrieRoot) {
			outputter.SetError(fmt.Errorf("incorrect trie root error:%w", err))

			return
		}

		outputter.WriteCommandResult(&ReGenesisResult{})
	}

	genesisCmd.Flags().StringVar(
		&params.SnapshotTrieDBPath,
		"target-path",
		"",
		"the directory of trie data of trie copy",
	)
	genesisCmd.Flags().StringVar(
		&params.TrieDBPath,
		"triedb",
		"",
		"the directory of trie data of old chain",
	)
	genesisCmd.Flags().StringVar(
		&params.TrieRoot,
		"stateRoot",
		"",
		"block state root of old chain",
	)

	getRootCmd := &cobra.Command{
		Use:   "getroot",
		Short: "returns state root of old chain",
		Run: func(cmd *cobra.Command, args []string) {
			outputter := command.InitializeOutputter(genesisCmd)
			defer outputter.WriteOutput()

			rpcClient, err := jsonrpc.NewClient(params.JSONRPCAddress)
			if err != nil {
				outputter.SetError(fmt.Errorf("connect to client error:%w", err))

				return
			}

			block, err := rpcClient.Eth().GetBlockByNumber(ethgo.BlockNumber(params.BlockNumber), false)
			if err != nil {
				outputter.SetError(fmt.Errorf("get block error:%w", err))

				return
			}

			outputter.WriteCommandResult(&ReGenesisResult{
				Message: fmt.Sprintf("state root %s for block %d", block.StateRoot, block.Number),
			})
		},
	}

	genesisCmd.AddCommand(getRootCmd)
	//genesisCmd.AddCommand(RegenesisCMD())
	getRootCmd.Flags().StringVar(
		&params.JSONRPCAddress,
		"rpc",
		"",
		"the JSON RPC IP address for old chain",
	)
	getRootCmd.Flags().Int64Var(
		&params.BlockNumber,
		"block",
		int64(ethgo.Latest),
		"Block number of trie snapshot",
	)

	return genesisCmd
}
