// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package utils

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"

	"github.com/ChainSafe/chainbridge-utils/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const DefaultGasLimit = 6721975
const DefaultMaxGasPrice = 20000000000
const DefaultGasMultiplier = 1

var ExpectedBlockTime = time.Second

type Client struct {
	Client    *ethclient.Client
	Opts      *bind.TransactOpts
	CallOpts  *bind.CallOpts
	nonceLock sync.Mutex
}

func NewClient(endpoint string, kp *secp256k1.Keypair) (*Client, error) {
	ctx := context.Background()
	rpcClient, err := rpc.DialWebsocket(ctx, endpoint, "/ws")
	if err != nil {
		return nil, err
	}
	client := ethclient.NewClient(rpcClient)

	id, err := client.ChainID(ctx)
	if err != nil {
		return nil, err
	}

	opts, err := bind.NewKeyedTransactorWithChainID(kp.PrivateKey(), id)
	if err != nil {
		return nil, err
	}
	opts.Nonce = big.NewInt(0)
	opts.Value = big.NewInt(0)              // in wei
	opts.GasLimit = uint64(DefaultGasLimit) // in units
	opts.GasPrice = big.NewInt(DefaultMaxGasPrice)
	opts.Context = ctx

	return &Client{
		Client: client,
		Opts:   opts,
		CallOpts: &bind.CallOpts{
			From: opts.From,
		},
	}, nil
}

func (c *Client) LockNonceAndUpdate() error {
	c.nonceLock.Lock()
	nonce, err := c.Client.PendingNonceAt(context.Background(), c.Opts.From)
	if err != nil {
		c.nonceLock.Unlock()
		return err
	}
	c.Opts.Nonce.SetUint64(nonce)
	return nil
}

func (c *Client) UnlockNonce() {
	c.nonceLock.Unlock()
}

// WaitForTx will query the chain at ExpectedBlockTime intervals, until a receipt is returned.
// Returns an error if the tx failed.
func WaitForTx(client *Client, tx *ethtypes.Transaction) error {
	retry := 10
	for retry > 0 {
		receipt, err := client.Client.TransactionReceipt(context.Background(), tx.Hash())
		if err != nil {
			if errors.Is(err, ethereum.NotFound) {
				retry--
				time.Sleep(ExpectedBlockTime)
				continue
			} else {
				return err
			}
		}

		if receipt.Status != 1 {
			return fmt.Errorf("transaction failed on chain")
		}
		return nil
	}
	return nil
}
