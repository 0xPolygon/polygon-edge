package generator

import (
	"encoding/hex"
	"fmt"
	"log"
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
	dg.contractBytecode = []byte(dg.params.ContractArtifact.Bytecode)

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

	return deployGenerator, nil
}

func (dg *DeployGenerator) GenerateTokenTransferTransaction(
	mode string,
	contractAddress *types.Address,
) (*types.Transaction, error) {
	var (
		txn    *types.Transaction
		txnErr error
	)

	if mode == "erc20Transfer" {
		newNextNonce := atomic.AddUint64(&dg.params.Nonce, 1)

		encodedParams, err := dg.params.ContractArtifact.ABI.Methods["transfer"].Encode(
			[]string{dg.params.RecieverAddress.String(),
				"30000",
			})
		if err != nil {
			log.Fatalln("Could not encode parameters for transfer method: ", err.Error())
		}

		dg.contractBytecode = encodedParams

		// token transaction txn
		txn, txnErr = dg.signer.SignTx(&types.Transaction{
			From:     dg.params.SenderAddress,
			To:       contractAddress,
			Gas:      dg.estimatedGas,
			Value:    big.NewInt(0),
			GasPrice: dg.params.GasPrice,
			Nonce:    newNextNonce - 1,
			Input:    dg.contractBytecode,
			V:        big.NewInt(1), // it is necessary to encode in rlp
		}, dg.params.SenderKey)

		if txnErr != nil {
			return nil, fmt.Errorf("failed to sign transaction: %w", err)
		}
	}

	return txn, nil
}

func (dg *DeployGenerator) GenerateTransaction(mode string) (*types.Transaction, error) {
	newNextNonce := atomic.AddUint64(&dg.params.Nonce, 1)

	var (
		txn    *types.Transaction
		txnErr error
	)

	if mode == "contract" {
		buf, err := hex.DecodeString(dg.params.ContractArtifact.Bytecode)
		if err != nil {
			return nil, fmt.Errorf("unable to decode bytecode, %w", err)
		}

		dg.contractBytecode = buf
		dg.contractBytecode = append(dg.contractBytecode, dg.params.ConstructorArgs...)

		// contract deployment txn
		txn, txnErr = dg.signer.SignTx(&types.Transaction{
			From:     dg.params.SenderAddress,
			Gas:      dg.estimatedGas,
			Value:    big.NewInt(0),
			GasPrice: dg.params.GasPrice,
			Nonce:    newNextNonce - 1,
			Input:    dg.contractBytecode,
			V:        big.NewInt(1), // it is necessary to encode in rlp
		}, dg.params.SenderKey)

		if txnErr != nil {
			return nil, fmt.Errorf("failed to sign transaction: %w", err)
		}
	}

	return txn, nil
}
