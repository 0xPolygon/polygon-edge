package generator

import (
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/types"
)

type ContractTxnsGenerator struct {
	BaseGenerator

	contractBytecode []byte
	encodedParams    []byte

	contractAddress        *types.Address
	failedContractTxns     []*FailedContractTxnInfo
	failedContractTxnsLock sync.RWMutex
}

//	Returns contract deployment tx if contractAddress is empty, otherwise returns
//	a token transfer tx
func (gen *ContractTxnsGenerator) GetExampleTransaction() (*types.Transaction, error) {
	if gen.contractAddress == nil {
		//	contract not deployed yet
		//	generate contract deployment tx
		return gen.signer.SignTx(&types.Transaction{
			From:     gen.params.SenderAddress,
			Value:    big.NewInt(0),
			GasPrice: gen.params.GasPrice,
			Input:    gen.contractBytecode,
			V:        big.NewInt(1), // it is necessary to encode in rlp
		}, gen.params.SenderKey)
	}

	//	return token transfer tx
	return gen.signer.SignTx(&types.Transaction{
		From:     gen.params.SenderAddress,
		To:       gen.contractAddress,
		Value:    big.NewInt(0),
		GasPrice: gen.params.GasPrice,
		Input:    gen.encodedParams,
		V:        big.NewInt(1), // it is necessary to encode in rlp
	}, gen.params.SenderKey)
}

func (gen *ContractTxnsGenerator) GenerateTransaction() (*types.Transaction, error) {
	newNextNonce := atomic.AddUint64(&gen.params.Nonce, 1)

	if gen.contractAddress == nil {
		//	contract not deployed yet
		//	generate contract deployment tx
		return gen.signer.SignTx(&types.Transaction{
			From:     gen.params.SenderAddress,
			Value:    big.NewInt(0),
			Gas:      gen.estimatedGas,
			GasPrice: gen.params.GasPrice,
			Nonce:    newNextNonce - 1,
			Input:    gen.contractBytecode,
			V:        big.NewInt(1), // it is necessary to encode in rlp
		}, gen.params.SenderKey)
	}

	//	return token transfer tx
	return gen.signer.SignTx(&types.Transaction{
		From:     gen.params.SenderAddress,
		To:       gen.contractAddress,
		Value:    big.NewInt(0),
		Gas:      gen.estimatedGas,
		GasPrice: gen.params.GasPrice,
		Nonce:    newNextNonce - 1,
		Input:    gen.encodedParams,
		V:        big.NewInt(1), // it is necessary to encode in rlp
	}, gen.params.SenderKey)
}

func (gen *ContractTxnsGenerator) MarkFailedContractTxn(failedContractTxn *FailedContractTxnInfo) {
	gen.failedContractTxnsLock.Lock()
	defer gen.failedContractTxnsLock.Unlock()

	gen.failedContractTxns = append(gen.failedContractTxns, failedContractTxn)
}

func (gen *ContractTxnsGenerator) SetContractAddress(addr types.Address) {
	gen.contractAddress = &addr
}
