package generator

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

type ERC721Generator struct {
	BaseGenerator

	contractBytecode []byte
	encodedParams    []byte
}

func NewERC721Generator(params *GeneratorParams) (*ERC721Generator, error) {
	gen := &ERC721Generator{}

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

	if gen.encodedParams, err = params.ContractArtifact.ABI.Methods["createNFT"].Encode([]string{"https://realy-valuable-nft-page.io"}); err != nil {
		return nil, fmt.Errorf("cannot encode createNFT method params: %w", err)
	}

	return gen, nil
}

//	Returns contract deployment tx if contractAddress is empty, otherwise returns
//	a token transfer tx
func (gen *ERC721Generator) GetExampleTransaction() (*types.Transaction, error) {
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

func (gen *ERC721Generator) GenerateTransaction() (*types.Transaction, error) {
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
