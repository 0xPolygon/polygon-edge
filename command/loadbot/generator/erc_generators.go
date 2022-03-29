package generator

import (
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"
)

// call ERC20 contract method and encode parameters
func NewERC20Generator(params *GeneratorParams) (*ContractTxnsGenerator, error) {
	gen := &ContractTxnsGenerator{}

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
			"0.001", // token has 5 decimals
		}); err != nil {
		return nil, fmt.Errorf("cannot encode ERC20 transfer method params: %w", err)
	}

	return gen, nil
}

// call ERC721 contract method and encode parameters
func NewERC721Generator(params *GeneratorParams) (*ContractTxnsGenerator, error) {
	gen := &ContractTxnsGenerator{}

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

	if gen.encodedParams, err = params.ContractArtifact.ABI.Methods["createNFT"].Encode(
		[]string{"https://really-valuable-nft-page.io"}); err != nil {
		return nil, fmt.Errorf("cannot encode ERC721 createNFT method params: %w", err)
	}

	return gen, nil
}
