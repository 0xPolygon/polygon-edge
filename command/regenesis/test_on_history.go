package regenesis

import (
	"errors"
	"fmt"
	"time"

	leveldb2 "github.com/0xPolygon/polygon-edge/blockchain/storage/leveldb"
	"github.com/0xPolygon/polygon-edge/command"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/types"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	ldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
)

var (
	triePath  string
	chainPath string
	toBlock   uint64
	fromBlock uint64
)

/*
Run: ./polygon-edge regenesis history --triedb "path_to_triedb" --chaindb "path_to_blockchain_db"
*/
func HistoryTestCmd() *cobra.Command {
	historyTestCMD := &cobra.Command{
		Use:   "history",
		Short: "run history test",
	}

	historyTestCMD.Flags().StringVar(&triePath, "triedb", "", "path to trie db")
	historyTestCMD.Flags().StringVar(&chainPath, "chaindb", "", "path to chain db")
	historyTestCMD.Flags().Uint64Var(&toBlock, "to", 0, "upper bound of regenesis test(default is head)")
	historyTestCMD.Flags().Uint64Var(&fromBlock, "from", 0, "lower bound of regenesis test(default is 0)")

	historyTestCMD.Run = func(cmd *cobra.Command, args []string) {
		outputter := command.InitializeOutputter(historyTestCMD)
		defer outputter.WriteOutput()

		trieDB, err := leveldb.OpenFile(triePath, &opt.Options{ReadOnly: true})
		if err != nil {
			outputter.SetError(err)

			return
		}

		st, err := leveldb2.NewLevelDBStorage(chainPath, hclog.NewNullLogger())
		if err != nil {
			outputter.SetError(err)

			return
		}

		if toBlock == 0 {
			var ok bool

			toBlock, ok = st.ReadHeadNumber()
			if !ok {
				outputter.SetError(errors.New("can't read head"))

				return
			}
		}

		_, err = outputter.Write([]byte(fmt.Sprintf("running test from %d to %d", toBlock, fromBlock)))
		if err != nil {
			outputter.SetError(err)

			return
		}

		lastStateRoot := types.Hash{}

		for i := toBlock; i > fromBlock; i-- {
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

			tmpDB, err := leveldb.Open(ldbStorageNew, nil)
			if err != nil {
				outputter.SetError(err)

				return
			}

			tmpStorage := itrie.NewKV(tmpDB)
			tt := time.Now().UTC()

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

			err = tmpDB.Close()
			if err != nil {
				outputter.SetError(err)

				return
			}

			_, err = outputter.Write([]byte(fmt.Sprintf("block %v checked successfully, time %v", i, time.Since(tt).String())))
			if err != nil {
				outputter.SetError(err)

				return
			}
		}
	}

	return historyTestCMD
}
