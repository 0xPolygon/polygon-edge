package generator

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

type TransferGenerator struct {
	BaseGenerator

	receiverAddress types.Address
}

func NewTransferGenerator(params *GeneratorParams) (*TransferGenerator, error) {
	transferGenerator := &TransferGenerator{}

	transferGenerator.BaseGenerator = BaseGenerator{
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
	key, err := crypto.GenerateECDSAKey()
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
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return txn, nil
}
