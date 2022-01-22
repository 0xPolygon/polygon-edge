package generator

import (
	"github.com/0xPolygon/polygon-edge/crypto"
	"sync"
)

type BaseGenerator struct {
	failedTxns     []*FailedTxnInfo
	failedTxnsLock sync.RWMutex

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

func (bg *BaseGenerator) SetGasEstimate(gasEstimate uint64) {
	bg.estimatedGas = gasEstimate
}
