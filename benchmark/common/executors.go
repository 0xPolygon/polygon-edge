package common

import (
	"sync"
	"testing"

	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

// TxTestCase represents a test case data to be run with txTestCasesExecutor
type TxTestCase struct {
	Name         string
	Relayer      txrelayer.TxRelayer
	ContractAddr ethgo.Address
	Input        [][]byte
	Sender       ethgo.Key
	TxNumber     int
}

// TxTestCasesExecutor executes transactions from testInput and waits in separate
// go routins for each tx receipt
func TxTestCasesExecutor(b *testing.B, testInput TxTestCase) {
	b.Helper()
	b.Run(testInput.Name, func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		var wg sync.WaitGroup

		// submit all tx 'repeatCall' times
		for i := 0; i < testInput.TxNumber; i++ {
			// call contract for the all inputs
			for j := 0; j < len(testInput.Input); j++ {
				// the tx is submitted to the blockchain without waiting for the receipt,
				// since we want to have multiple tx in one block
				txHash, err := testInput.Relayer.SumbitTransaction(
					&ethgo.Transaction{
						To:    &testInput.ContractAddr,
						Input: testInput.Input[j],
					}, testInput.Sender)
				require.NoError(b, err)
				require.NotEqual(b, ethgo.ZeroHash, txHash)

				wg.Add(1)

				// wait for receipt of submitted tx in a separate routine, and continue with the next tx
				func(hash ethgo.Hash) {
					defer wg.Done()

					receipt, err := testInput.Relayer.WaitForReceipt(hash)
					require.NoError(b, err)
					require.Equal(b, uint64(types.ReceiptSuccess), receipt.Status)
				}(txHash)
			}
		}

		wg.Wait()
	})
}
