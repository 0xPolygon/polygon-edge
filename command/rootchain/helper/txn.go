package helper

import (
	"encoding/hex"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/types"
)

const (
	defaultGasPrice = 1879048192 // 0x70000000
	defaultGasLimit = 5242880    // 0x500000
)

var (
	// TODO: @Stefan-Ethernal Use either private key provided through CLI input or this (denoting dev vs prod mode)
	// use a deterministic wallet/private key so that the address of the deployed contracts
	// are deterministic
	rootchainAdminKey *wallet.Key
)

func init() {
	dec, err := hex.DecodeString("aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d")
	if err != nil {
		panic(err)
	}

	rootchainAdminKey, err = wallet.NewWalletFromPrivKey(dec)
	if err != nil {
		panic(err)
	}
}

func GetRootchainAdminAddr() types.Address {
	return types.Address(rootchainAdminKey.Address())
}

func GetRootchainAdminKey() ethgo.Key {
	return rootchainAdminKey
}
