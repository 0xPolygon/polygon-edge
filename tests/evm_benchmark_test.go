package tests

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	benchmarksDir = "evm-benchmarks/benchmarks"
	chainID       = 10
)

func BenchmarkEVM(b *testing.B) {
	folders, err := listFolders([]string{benchmarksDir})
	require.NoError(b, err)

	for _, folder := range folders {
		files, err := listFiles(folder, ".json")
		require.NoError(b, err)

		for _, file := range files {
			name := getTestName(file)

			b.Run(name, func(b *testing.B) {
				data, err := os.ReadFile(file)
				require.NoError(b, err)

				var testCases map[string]testCase
				if err = json.Unmarshal(data, &testCases); err != nil {
					b.Fatalf("failed to unmarshal %s: %v", file, err)
				}

				for _, tc := range testCases {
					for fork, postState := range tc.Post {
						forks, exists := Forks[fork]
						if !exists {
							b.Logf("%s fork is not supported, skipping test case.", fork)
							continue
						}

						fc := &forkConfig{name: fork, forks: forks}

						for idx, postStateEntry := range postState {
							err := runBenchmarkTest(b, tc, fc, postStateEntry)
							require.NoError(b, err, fmt.Sprintf("test %s (case#%d) execution failed", name, idx))
						}
					}
				}
			})
		}
	}
}

func runBenchmarkTest(b *testing.B, c testCase, fc *forkConfig, p postEntry) error {
	env := c.Env.ToEnv(b)
	forks := fc.forks

	var baseFee *big.Int

	if forks.IsActive(chain.London, 0) {
		if c.Env.BaseFee != "" {
			baseFee = stringToBigIntT(b, c.Env.BaseFee)
		} else {
			// Retesteth uses `10` for genesis baseFee. Therefore, it defaults to
			// parent - 2 : 0xa as the basefee for 'this' context.
			baseFee = big.NewInt(testGenesisBaseFee)
		}
	}

	msg, err := c.Transaction.At(p.Indexes, baseFee)
	if err != nil {
		return err
	}

	s, _, parentRoot, err := buildState(c.Pre)
	if err != nil {
		return err
	}

	currentForks := forks.At(uint64(env.Number))

	// try to recover tx with current signer
	if len(p.TxBytes) != 0 {
		tx := &types.Transaction{}
		err := tx.UnmarshalRLP(p.TxBytes)
		if err != nil {
			return err
		}

		signer := crypto.NewSigner(currentForks, chainID)

		_, err = signer.Sender(tx)
		if err != nil {
			return err
		}
	}

	executor := state.NewExecutor(&chain.Params{
		Forks:   forks,
		ChainID: chainID,
		BurnContract: map[uint64]types.Address{
			0: types.ZeroAddress,
		},
	}, s, hclog.NewNullLogger())

	executor.GetHash = func(*types.Header) func(i uint64) types.Hash {
		return vmTestBlockHash
	}

	transition, err := executor.BeginTxn(parentRoot, c.Env.ToHeader(b), env.Coinbase)
	if err != nil {
		return err
	}

	if currentForks.Berlin {
		transition.PopulateAccessList(msg.From(), msg.To(), msg.AccessList())
	}

	var (
		gasUsed uint64
		elapsed uint64
		refund  uint64
	)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		snapshotID := transition.Snapshot()

		b.StartTimer()
		start := time.Now()

		// execute the message
		result := transition.Call2(msg.From(), *msg.To(), msg.Input(), msg.Value(), msg.Gas())
		if result.Err != nil {
			return result.Err
		}

		b.StopTimer()
		elapsed += uint64(time.Since(start))
		refund += transition.GetRefund()
		gasUsed += msg.Gas() - result.GasLeft

		err = transition.RevertToSnapshot(snapshotID)
		if err != nil {
			return err
		}
	}

	if elapsed < 1 {
		elapsed = 1
	}

	// Keep it as uint64, multiply 100 to get two digit float later
	mgasps := (100 * 1000 * (gasUsed - refund)) / elapsed
	b.ReportMetric(float64(mgasps)/100, "mgas/s")

	return nil
}
