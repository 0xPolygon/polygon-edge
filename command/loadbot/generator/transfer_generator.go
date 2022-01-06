package generator

import (
	"fmt"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/types"
	"math/big"
	"sync"
	"sync/atomic"
)

type TransferGenerator struct {
	failedTxns     []*FailedTxnInfo
	failedTxnsLock sync.RWMutex

	params          *GeneratorParams
	signer          *crypto.EIP155Signer
	receiverAddress types.Address
	estimatedGas    uint64
}

func NewTransferGenerator(params *GeneratorParams) (*TransferGenerator, error) {
	transferGenerator := &TransferGenerator{
		failedTxns: make([]*FailedTxnInfo, 0),
		params:     params,
		signer:     crypto.NewEIP155Signer(params.ChainID),
	}

	if genErr := transferGenerator.generateReceiver(); genErr != nil {
		return nil, genErr
	}

	return transferGenerator, nil
}

func (tg *TransferGenerator) GetExampleTransaction() (*types.Transaction, error) {
	return tg.signer.SignTx(&types.Transaction{
		From:     tg.params.SenderAddress,
		To:       &tg.receiverAddress,
		Value:    tg.params.Value,
		GasPrice: tg.params.GasPrice,
		V:        big.NewInt(1), // it is necessary to encode in rlp
	}, tg.params.SenderKey)
}

func (tg *TransferGenerator) generateReceiver() error {
	key, err := crypto.GenerateKey()
	if err != nil {
		return err
	}

	tg.receiverAddress = crypto.PubKeyToAddress(&key.PublicKey)
	return nil
}

func (tg *TransferGenerator) GenerateTransaction() (*types.Transaction, error) {
	newNextNonce := atomic.AddUint64(&tg.params.Nonce, 1)

	txn, err := tg.signer.SignTx(&types.Transaction{
		From:     tg.params.SenderAddress,
		To:       &tg.receiverAddress,
		Gas:      tg.estimatedGas,
		Value:    tg.params.Value,
		GasPrice: tg.params.GasPrice,
		Nonce:    newNextNonce - 1,
		V:        big.NewInt(1), // it is necessary to encode in rlp
	}, tg.params.SenderKey)

	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %v", err)
	}

	return txn, nil
}

func (tg *TransferGenerator) GetTransactionErrors() []*FailedTxnInfo {
	tg.failedTxnsLock.RLock()
	defer tg.failedTxnsLock.RUnlock()

	return tg.failedTxns
}

func (tg *TransferGenerator) MarkFailedTxn(failedTxn *FailedTxnInfo) {
	tg.failedTxnsLock.Lock()
	defer tg.failedTxnsLock.Unlock()

	tg.failedTxns = append(tg.failedTxns, failedTxn)
}

func (tg *TransferGenerator) SetGasEstimate(gasEstimate uint64) {
	tg.estimatedGas = gasEstimate
}
