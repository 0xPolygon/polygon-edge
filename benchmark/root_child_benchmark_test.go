package benchmark

import (
	"testing"
)

// The rootChildSendTx function executes test cases that measure transaction execution on both the root and child chains
// To do this, it first calls RootChildSendTxSetUp to set up the testing environment,
// which may include starting the cluster, deploying contracts, and building the test cases.
// After building the test cases, rootChildSendTx returns them along with a cleanup function that should be called
// after the test cases have been executed. The test cases are executed by the TxTestCasesExecutor.
func Benchmark_RootChildSendTx(b *testing.B) {
	// set up environment, get test cases and clean up fn
	testCases, cleanUpFn := RootChildSendTxSetUp(b, "", "", "", true)
	defer cleanUpFn()

	// Loop over the test cases and measure the execution time of the transactions
	for _, testInput := range testCases {
		TxTestCasesExecutor(b, testInput)
	}
}
