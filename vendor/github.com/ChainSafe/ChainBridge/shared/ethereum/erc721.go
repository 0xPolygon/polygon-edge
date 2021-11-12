// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package utils

import (
	"math/big"

	"github.com/ChainSafe/ChainBridge/bindings/ERC721Handler"
	"github.com/ChainSafe/ChainBridge/bindings/ERC721MinterBurnerPauser"
	"github.com/ethereum/go-ethereum/common"
)

// DeployMintAndApprove deploys a new erc721 contract, mints to the deployer, and approves the erc20 handler to transfer those token.
func DeployErc721(client *Client) (common.Address, error) {
	err := client.LockNonceAndUpdate()
	if err != nil {
		return ZeroAddress, err
	}

	// Deploy
	addr, tx, _, err := ERC721MinterBurnerPauser.DeployERC721MinterBurnerPauser(client.Opts, client.Client, "", "", "")
	if err != nil {
		return ZeroAddress, err
	}

	err = WaitForTx(client, tx)
	if err != nil {
		return ZeroAddress, err
	}

	client.UnlockNonce()

	return addr, nil
}

func Erc721Mint(client *Client, erc721Contract common.Address, id *big.Int, metadata []byte) error {
	instance, err := ERC721MinterBurnerPauser.NewERC721MinterBurnerPauser(erc721Contract, client.Client)
	if err != nil {
		return err
	}

	err = client.LockNonceAndUpdate()
	if err != nil {
		return err
	}

	// Mint
	tx, err := instance.Mint(client.Opts, client.Opts.From, id, string(metadata))
	if err != nil {
		return err
	}

	err = WaitForTx(client, tx)
	if err != nil {
		return err
	}

	client.UnlockNonce()

	return nil
}

func ApproveErc721(client *Client, contractAddress, recipient common.Address, tokenId *big.Int) error {
	err := client.LockNonceAndUpdate()
	if err != nil {
		return err
	}

	instance, err := ERC721MinterBurnerPauser.NewERC721MinterBurnerPauser(contractAddress, client.Client)
	if err != nil {
		return err
	}

	tx, err := instance.Approve(client.Opts, recipient, tokenId)
	if err != nil {
		return err
	}

	err = WaitForTx(client, tx)
	if err != nil {
		return err
	}

	client.UnlockNonce()

	return nil
}

func FundErc721Handler(client *Client, handlerAddress, erc721Address common.Address, tokenId *big.Int) error {
	err := ApproveErc721(client, erc721Address, handlerAddress, tokenId)
	if err != nil {
		return err
	}

	instance, err := ERC721Handler.NewERC721Handler(handlerAddress, client.Client)
	if err != nil {
		return err
	}

	err = client.LockNonceAndUpdate()
	if err != nil {
		return err
	}

	tx, err := instance.FundERC721(client.Opts, erc721Address, client.Opts.From, tokenId)
	if err != nil {
		return err
	}

	err = WaitForTx(client, tx)
	if err != nil {
		return err
	}

	client.UnlockNonce()

	return nil
}

func OwnerOf(client *Client, erc721Contract common.Address, tokenId *big.Int) (common.Address, error) {
	instance, err := ERC721MinterBurnerPauser.NewERC721MinterBurnerPauser(erc721Contract, client.Client)
	if err != nil {
		return ZeroAddress, err
	}
	return instance.OwnerOf(client.CallOpts, tokenId)
}

func Erc721GetTokenURI(client *Client, erc721Contract common.Address, tokenId *big.Int) (string, error) {
	instance, err := ERC721MinterBurnerPauser.NewERC721MinterBurnerPauser(erc721Contract, client.Client)
	if err != nil {
		return "", err
	}

	return instance.TokenURI(client.CallOpts, tokenId)
}

func Erc721AddMinter(client *Client, erc721Contract common.Address, minter common.Address) error {
	instance, err := ERC721MinterBurnerPauser.NewERC721MinterBurnerPauser(erc721Contract, client.Client)
	if err != nil {
		return err
	}

	err = client.LockNonceAndUpdate()
	if err != nil {
		return err
	}

	role, err := instance.MINTERROLE(client.CallOpts)
	if err != nil {
		return err
	}

	tx, err := instance.GrantRole(client.Opts, role, minter)
	if err != nil {
		return err
	}

	err = WaitForTx(client, tx)
	if err != nil {
		return err
	}

	client.UnlockNonce()

	return nil
}
