// Package prover contains structures and utility functions for the prover
package prover

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/state/runtime/tracer/structtracer"
	"github.com/0xPolygon/polygon-edge/types"
)

type StorageAccess struct {
	Slot        string
	MerkleProof []string
}

type Storage struct {
	Account     string
	StorageRoot string
	Storage     []StorageAccess
}

type ProverAccount struct {
	Balance  *big.Int
	Nonce    uint64
	Root     string
	CodeHash string
}

type ProverAccountProof struct {
	Account     string
	MerkleProof []string
}

type ProverData struct {
	ChainID             interface{}
	BlockHeader         types.Header
	PreviousBlockHeader types.Header
	Accounts            interface{}
	PreviousStorage     interface{}
	Transactions        interface{}
	Receipts            interface{}
	ContractCodes       interface{}
	PreviousState       interface{}
}

func ParseBlockAccounts(block *types.Block) ([]string, error) {
	var accounts = make([]string, 0)
	for _, tx := range block.Transactions {
		accounts = append(accounts, tx.From.String())
		accounts = append(accounts, (*tx.To).String())
	}

	return accounts, nil
}

func ParseContractCodeForAccounts(tracesJSON []interface{}) ([]string, error) {
	var accounts = make(map[string]int)

	for _, traceJSON := range tracesJSON {
		trace, ok := traceJSON.(*structtracer.StructTraceResult)
		if !ok {
			return nil, fmt.Errorf("invalid struct trace data conversion")
		}

		for _, log := range trace.StructLogs {
			if log.Op == "CALL" || log.Op == "STATICCALL" {
				accounts[log.Stack[len(log.Stack)-2]] = 1
			}
		}
	}

	result := make([]string, 0)
	for account := range accounts {
		result = append(result, account)
	}

	return result, nil
}

func ParseTraceForStorageAccess(tracesJSON []interface{}) (map[string][]structtracer.StorageAccess, error) {
	var storageChanges = make(map[string][]structtracer.StorageAccess)

	for _, traceJSON := range tracesJSON {
		trace, ok := traceJSON.(*structtracer.StructTraceResult)
		if !ok {
			return nil, fmt.Errorf("invalid struct trace data conversion")
		}

		for account, storage := range trace.StorageUpdates {
			for storageAccess := range storage {
				storageChanges[account.String()] = append(storageChanges[account.String()], storageAccess)
			}
		}
	}

	return storageChanges, nil
}
