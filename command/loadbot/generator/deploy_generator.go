package generator

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

type DeployGenerator struct {
	BaseGenerator

	contractBytecode []byte
}

func (dg *DeployGenerator) GetExampleTransaction() (*types.Transaction, error) {
	return dg.signer.SignTx(&types.Transaction{
		From:     dg.params.SenderAddress,
		Value:    big.NewInt(0),
		GasPrice: dg.params.GasPrice,
		Input:    dg.contractBytecode,
		V:        big.NewInt(1), // it is necessary to encode in rlp
	}, dg.params.SenderKey)
}

func NewDeployGenerator(params *GeneratorParams) (*DeployGenerator, error) {
	deployGenerator := &DeployGenerator{}

	deployGenerator.BaseGenerator = BaseGenerator{
		failedTxns: make([]*FailedTxnInfo, 0),
		params:     params,
		signer:     crypto.NewEIP155Signer(params.ChainID),
	}

	buf, err := hex.DecodeString(params.ContractArtifact.Bytecode)
	if err != nil {
		return nil, fmt.Errorf("unable to decode bytecode, %w", err)
	}

	deployGenerator.contractBytecode = buf

	return deployGenerator, nil
}

func (dg *DeployGenerator) GenerateTransaction() (*types.Transaction, error) {
	newNextNonce := atomic.AddUint64(&dg.params.Nonce, 1)

	txn, err := dg.signer.SignTx(&types.Transaction{
		From:     dg.params.SenderAddress,
		Gas:      dg.estimatedGas,
		Value:    big.NewInt(0),
		GasPrice: dg.params.GasPrice,
		Nonce:    newNextNonce - 1,
		Input:    dg.contractBytecode,
		V:        big.NewInt(1), // it is necessary to encode in rlp
	}, dg.params.SenderKey)

	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return txn, nil
}
