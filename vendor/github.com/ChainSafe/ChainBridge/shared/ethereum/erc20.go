// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package utils

import (
	"math/big"

	"github.com/ChainSafe/ChainBridge/bindings/ERC20Handler"
	ERC20 "github.com/ChainSafe/ChainBridge/bindings/ERC20PresetMinterPauser"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ethereum/go-ethereum/common"
)

// DeployMintAndApprove deploys a new erc20 contract, mints to the deployer, and approves the erc20 handler to transfer those token.
func DeployMintApproveErc20(client *Client, erc20Handler common.Address, amount *big.Int) (common.Address, error) {
	err := client.LockNonceAndUpdate()
	if err != nil {
		return ZeroAddress, err
	}

	// Deploy
	erc20Addr, tx, erc20Instance, err := ERC20.DeployERC20PresetMinterPauser(client.Opts, client.Client, "", "")
	if err != nil {
		return ZeroAddress, err
	}

	err = WaitForTx(client, tx)
	if err != nil {
		return ZeroAddress, err
	}

	client.UnlockNonce()

	// Mint
	err = client.LockNonceAndUpdate()
	if err != nil {
		return ZeroAddress, err
	}

	tx, err = erc20Instance.Mint(client.Opts, client.Opts.From, amount)
	if err != nil {
		return ZeroAddress, err
	}

	err = WaitForTx(client, tx)
	if err != nil {
		return ZeroAddress, err
	}

	client.UnlockNonce()

	// Approve
	err = client.LockNonceAndUpdate()
	if err != nil {
		return ZeroAddress, err
	}

	tx, err = erc20Instance.Approve(client.Opts, erc20Handler, amount)
	if err != nil {
		return ZeroAddress, err
	}

	err = WaitForTx(client, tx)
	if err != nil {
		return ZeroAddress, err
	}

	client.UnlockNonce()

	return erc20Addr, nil
}

func DeployAndMintErc20(client *Client, amount *big.Int) (common.Address, error) {
	err := client.LockNonceAndUpdate()
	if err != nil {
		return ZeroAddress, err
	}

	// Deploy
	erc20Addr, tx, erc20Instance, err := ERC20.DeployERC20PresetMinterPauser(client.Opts, client.Client, "", "")
	if err != nil {
		return ZeroAddress, err
	}

	err = WaitForTx(client, tx)
	if err != nil {
		return ZeroAddress, err
	}
	client.UnlockNonce()

	// Mint
	err = client.LockNonceAndUpdate()
	if err != nil {
		return ZeroAddress, err
	}

	mintTx, err := erc20Instance.Mint(client.Opts, client.Opts.From, amount)
	if err != nil {
		return ZeroAddress, err
	}

	err = WaitForTx(client, mintTx)
	if err != nil {
		return ZeroAddress, err
	}

	client.UnlockNonce()

	return erc20Addr, nil
}

func Erc20Approve(client *Client, erc20Contract, recipient common.Address, amount *big.Int) error {
	err := client.LockNonceAndUpdate()
	if err != nil {
		return err
	}

	instance, err := ERC20.NewERC20PresetMinterPauser(erc20Contract, client.Client)
	if err != nil {
		return err
	}

	tx, err := instance.Approve(client.Opts, recipient, amount)
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

func Erc20GetBalance(client *Client, erc20Contract, account common.Address) (*big.Int, error) { //nolint:unused,deadcode
	instance, err := ERC20.NewERC20PresetMinterPauser(erc20Contract, client.Client)
	if err != nil {
		return nil, err
	}

	bal, err := instance.BalanceOf(client.CallOpts, account)
	if err != nil {
		return nil, err

	}
	return bal, nil

}

func FundErc20Handler(client *Client, handlerAddress, erc20Address common.Address, amount *big.Int) error {
	err := Erc20Approve(client, erc20Address, handlerAddress, amount)
	if err != nil {
		return err
	}

	instance, err := ERC20Handler.NewERC20Handler(handlerAddress, client.Client)
	if err != nil {
		return err
	}

	client.Opts.Nonce = client.Opts.Nonce.Add(client.Opts.Nonce, big.NewInt(1))
	tx, err := instance.FundERC20(client.Opts, erc20Address, client.Opts.From, amount)
	if err != nil {
		return err
	}

	err = WaitForTx(client, tx)
	if err != nil {
		return err
	}

	return nil
}

func Erc20AddMinter(client *Client, erc20Contract, handler common.Address) error {
	err := client.LockNonceAndUpdate()
	if err != nil {
		return err
	}

	instance, err := ERC20.NewERC20PresetMinterPauser(erc20Contract, client.Client)
	if err != nil {
		return err
	}

	role, err := instance.MINTERROLE(client.CallOpts)
	if err != nil {
		return err
	}

	tx, err := instance.GrantRole(client.Opts, role, handler)
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

func Erc20GetAllowance(client *Client, erc20Contract, owner, spender common.Address) (*big.Int, error) {
	instance, err := ERC20.NewERC20PresetMinterPauser(erc20Contract, client.Client)
	if err != nil {
		return nil, err
	}

	amount, err := instance.Allowance(client.CallOpts, owner, spender)
	if err != nil {
		return nil, err
	}

	return amount, nil
}

func Erc20GetResourceId(client *Client, handler common.Address, rId msg.ResourceId) (common.Address, error) {
	instance, err := ERC20Handler.NewERC20Handler(handler, client.Client)
	if err != nil {
		return ZeroAddress, err
	}

	addr, err := instance.ResourceIDToTokenContractAddress(client.CallOpts, rId)
	if err != nil {
		return ZeroAddress, err
	}

	return addr, nil
}

func Erc20Mint(client *Client, erc20Address, recipient common.Address, amount *big.Int) error {
	err := client.LockNonceAndUpdate()
	if err != nil {
		return err
	}

	instance, err := ERC20.NewERC20PresetMinterPauser(erc20Address, client.Client)
	if err != nil {
		return err
	}

	tx, err := instance.Mint(client.Opts, recipient, amount)
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
