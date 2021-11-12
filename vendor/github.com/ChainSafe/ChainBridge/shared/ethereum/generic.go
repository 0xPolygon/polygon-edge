// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package utils

import (
	"github.com/ChainSafe/ChainBridge/bindings/GenericHandler"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ethereum/go-ethereum/common"
)

var StoreFunctionSig = CreateFunctionSignature("store(bytes32)")

// CreateFunctionSignature hashes the function signature and returns the first 4 bytes
func CreateFunctionSignature(sig string) [4]byte {
	var res [4]byte
	hash := Hash([]byte(sig))
	copy(res[:], hash[:])
	return res
}

func GetGenericResourceAddress(client *Client, handler common.Address, rId msg.ResourceId) (common.Address, error) {
	instance, err := GenericHandler.NewGenericHandler(handler, client.Client)
	if err != nil {
		return ZeroAddress, err
	}

	addr, err := instance.ResourceIDToContractAddress(client.CallOpts, rId)
	if err != nil {
		return ZeroAddress, err
	}
	return addr, nil
}
