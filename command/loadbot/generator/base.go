package generator

import (
	"sync"

	"github.com/0xPolygon/polygon-edge/crypto"
)

type BaseGenerator struct {
	failedTxns     []*FailedTxnInfo
	failedTxnsLock sync.RWMutex

	// failed contract deployment transactions
	failedContractTxns     []*FailedContractTxnInfo
	failedContractTxnsLock sync.RWMutex

	params       *GeneratorParams
	signer       *crypto.EIP155Signer
	estimatedGas uint64
}

func (bg *BaseGenerator) GetTransactionErrors() []*FailedTxnInfo {
	bg.failedTxnsLock.RLock()
	defer bg.failedTxnsLock.RUnlock()

	return bg.failedTxns
}

func (bg *BaseGenerator) MarkFailedTxn(failedTxn *FailedTxnInfo) {
	bg.failedTxnsLock.Lock()
	defer bg.failedTxnsLock.Unlock()

	bg.failedTxns = append(bg.failedTxns, failedTxn)
}

func (bg *BaseGenerator) MarkFailedContractTxn(failedContractTxn *FailedContractTxnInfo) {
	bg.failedContractTxnsLock.Lock()
	defer bg.failedContractTxnsLock.Unlock()

	bg.failedContractTxns = append(bg.failedContractTxns, failedContractTxn)
}

func (bg *BaseGenerator) SetGasEstimate(gasEstimate uint64) {
	bg.estimatedGas = gasEstimate
}
