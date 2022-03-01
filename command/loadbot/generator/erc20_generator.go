package generator

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

type ERC20Generator struct {
	BaseGenerator

	contractBytecode []byte
	encodedParams    []byte

	contractAddress *types.Address
}

func NewERC20Generator(params *GeneratorParams) (*ERC20Generator, error) {
	gen := &ERC20Generator{}

	gen.BaseGenerator = BaseGenerator{
		failedTxns: make([]*FailedTxnInfo, 0),
		params:     params,
		signer:     crypto.NewEIP155Signer(params.ChainID),
	}

	buf, err := hex.DecodeString(params.ContractArtifact.Bytecode)
	if err != nil {
		return nil, fmt.Errorf("unable to decode bytecode, %w", err)
	}

	gen.contractBytecode = buf
	gen.contractBytecode = append(gen.contractBytecode, params.ConstructorArgs...)

	if gen.encodedParams, err = params.ContractArtifact.ABI.Methods["transfer"].Encode(
		[]string{params.RecieverAddress.String(),
			"3",
		}); err != nil {
		return nil, fmt.Errorf("cannot encode transfer method params: %w", err)
	}

	return gen, nil
}

func (gen *ERC20Generator) SetContractAddress(addr types.Address) {
	gen.contractAddress = &addr
}

//	Returns contract deployment tx if contractAddress is empty, otherwise returns
//	a token transfer tx
func (gen *ERC20Generator) GetExampleTransaction() (*types.Transaction, error) {
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

func (gen *ERC20Generator) GenerateTransaction() (*types.Transaction, error) {
	newNextNonce := atomic.AddUint64(&gen.params.Nonce, 1)

	if gen.contractAddress == nil {
		// fmt.Println("generating deployment tx")
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
