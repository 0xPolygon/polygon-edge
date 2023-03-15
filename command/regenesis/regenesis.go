package regenesis

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func RegenesisCMD() *cobra.Command {
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
		"snapshotPath",
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

	return genesisCmd
}

type ReGenesisResult struct {
	Message string `json:"message"`
}

func (r *ReGenesisResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[Trie copy SUCCESS]\n")
	buffer.WriteString(r.Message)

	return buffer.String()
}
