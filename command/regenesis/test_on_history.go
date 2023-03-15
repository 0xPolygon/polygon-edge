package regenesis

import (
	"errors"
	"fmt"
	leveldb2 "github.com/0xPolygon/polygon-edge/blockchain/storage/leveldb"
	"github.com/0xPolygon/polygon-edge/command"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	ldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
	"path/filepath"
	"time"
)

func HistoryTestCmd() *cobra.Command {
	historyTestCMD := &cobra.Command{
		Use:   "history",
		Short: "run history test",
	}
	historyTestCMD.Run = func(cmd *cobra.Command, args []string) {
		outputter := command.InitializeOutputter(historyTestCMD)
		defer outputter.WriteOutput()

		path := "/Users/boris/Downloads/edge-ip-10-230-1-165"
		trie := filepath.Join(path, "trie.bak")
		chainPath := filepath.Join(path, "blockchain.bak")

		trieDB, err := leveldb.OpenFile(trie, &opt.Options{ReadOnly: true})
		if err != nil {
			outputter.SetError(err)
			return
		}

		st, err := leveldb2.NewLevelDBStorage(chainPath, hclog.NewNullLogger())
		if err != nil {
			outputter.SetError(err)
			return
		}

		lastBlockNumber, ok := st.ReadHeadNumber()
		if !ok {
			outputter.SetError(errors.New("can't read head"))
			return
		}

		lastStateRoot := types.Hash{}
		for i := lastBlockNumber - 100; i > lastBlockNumber-100000; i-- {
			canonicalHash, ok := st.ReadCanonicalHash(i)
			if !ok {
				outputter.SetError(errors.New("can't read canonical hash"))
				return
			}

			header, err := st.ReadHeader(canonicalHash)
			if !ok {
				outputter.SetError(fmt.Errorf("can't read header %w", err))
				return
			}

			if lastStateRoot == header.StateRoot {
				//state root is the same,as in previous block
				continue
			}

			lastStateRoot = header.StateRoot
			ldbStorageNew := ldbstorage.NewMemStorage()
			tmpDb, err := leveldb.Open(ldbStorageNew, nil)
			if err != nil {
				outputter.SetError(err)
				return
			}

			tmpStorage := itrie.NewKV(tmpDb)
			tt := time.Now()
			err = itrie.CopyTrie(header.StateRoot.Bytes(), itrie.NewKV(trieDB), tmpStorage, []byte{}, false)
			if err != nil {
				outputter.SetError(fmt.Errorf("copy trie for block %v returned error %w", i, err))
				return
			}

			hash, err := itrie.HashChecker(header.StateRoot.Bytes(), tmpStorage)
			if err != nil {
				outputter.SetError(fmt.Errorf("check trie root for block %v returned error %w", i, err))
				return
			}
			if hash != header.StateRoot {
				outputter.SetError(fmt.Errorf("check trie root for block %v returned another hash"+
					"expected: %s, got: %s", i, header.StateRoot.String(), hash.String()))
				return
			}
			tmpDb.Close()
			_, err = outputter.Write([]byte(fmt.Sprintf("block %v checked successfully, time %v", i, time.Since(tt).String())))
			if err != nil {
				outputter.SetError(err)
			}
		}
	}
	return historyTestCMD
}
