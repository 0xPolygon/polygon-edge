package benchmark

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
)

func Benchmark_TransitionMultiContractMemDb(b *testing.B) {
	multiContractTest(b, false)
}

func Benchmark_TransitionMultiContractLevelDb(b *testing.B) {
	multiContractTest(b, true)
}

func multiContractTest(b *testing.B, disk bool) {
	b.Helper()

	senderAddr := types.Address{1} // account that sends transactions

	alloc := map[types.Address]*chain.GenesisAccount{
		senderAddr: {Balance: ethgo.Ether(100)}, // give some ethers to sender
	}

	transition := newTestTransition(b, alloc, disk)

	// deploy contracts
	singleContractAddr := transitionDeployContract(b, transition, contractsapi.TestBenchmarkSingle.Bytecode, senderAddr)
	contractAAddr := transitionDeployContract(b, transition, contractsapi.TestBenchmarkA.Bytecode, senderAddr)
	contractBAddr := transitionDeployContract(b, transition, contractsapi.TestBenchmarkB.Bytecode, senderAddr)
	contractCAddr := transitionDeployContract(b, transition, contractsapi.TestBenchmarkC.Bytecode, senderAddr)

	//set multi contracts dependency address
	input := getTxInput(b, multiContSetAAddrFunc, []interface{}{contractBAddr})
	transitionCallContract(b, transition, contractAAddr, senderAddr, input)
	input = getTxInput(b, multiContSetBAddrFunc, []interface{}{contractCAddr})
	transitionCallContract(b, transition, contractBAddr, senderAddr, input)

	testCases := []struct {
		contractAddr types.Address
		input        []byte
	}{
		{
			contractAddr: singleContractAddr,
			input:        getTxInput(b, singleContSetFunc, []interface{}{big.NewInt(10)}),
		},
		{
			contractAddr: singleContractAddr,
			input:        getTxInput(b, singleContGetFunc, nil),
		},
		{
			contractAddr: singleContractAddr,
			input:        getTxInput(b, singleContCalcFunc, []interface{}{big.NewInt(50), big.NewInt(150)}),
		},
		{
			contractAddr: contractAAddr,
			input:        getTxInput(b, multiContFnA, nil),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	// execute transactions
	for i := 0; i < b.N; i++ {
		for j := 0; j < 400; j++ {
			for _, tc := range testCases {
				transitionCallContract(b, transition, tc.contractAddr, senderAddr, tc.input)
			}
		}
	}
}

func Benchmark_SampleContractMemDb(b *testing.B) {
	sampleContractTest(b, false)
}

func Benchmark_SampleContractLevelDb(b *testing.B) {
	sampleContractTest(b, true)
}

func sampleContractTest(b *testing.B, disk bool) {
	b.Helper()

	senderAddr := types.Address{1} // account that sends transactions
	alloc := map[types.Address]*chain.GenesisAccount{
		senderAddr: {Balance: ethgo.Ether(100)}, // give some ethers to sender
	}
	transition := newTestTransition(b, alloc, disk)
	code := contractsapi.SampleContract.Bytecode
	cpurchase := getTxInput(b, contractsapi.SampleContract.Abi.Methods["confirmPurchase"], nil)
	creceived := getTxInput(b, contractsapi.SampleContract.Abi.Methods["confirmReceived"], nil)
	refund := getTxInput(b, contractsapi.SampleContract.Abi.Methods["refund"], nil)

	// deploy contracts
	contractAddr := transitionDeployContract(b, transition, code, senderAddr)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for j := 0; j < 400; j++ {
			transitionCallContract(b, transition, contractAddr, senderAddr, cpurchase)
			transitionCallContract(b, transition, contractAddr, senderAddr, creceived)
			transitionCallContract(b, transition, contractAddr, senderAddr, refund)
		}
	}
}
