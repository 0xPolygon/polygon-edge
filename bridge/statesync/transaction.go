package statesync

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/go-web3/abi"
)

const (
	StateReceiverABI = `[
    {
      "anonymous": false,
      "inputs": [
        {
          "indexed": false,
          "internalType": "bytes",
          "name": "data",
          "type": "bytes"
        }
      ],
      "name": "StateReceived",
      "type": "event"
    },
    {
      "inputs": [
        {
          "internalType": "bytes",
          "name": "data",
          "type": "bytes"
        }
      ],
      "name": "onStateReceived",
      "outputs": [],
      "stateMutability": "nonpayable",
      "type": "function"
    }
  ]`
)

var (
	ParsedStateReceiverABI = abi.MustNewABI(StateReceiverABI)
)

func NewStateSyncedTx(event *StateSyncEvent) *types.Transaction {
	method := ParsedStateReceiverABI.Methods["onStateReceived"]
	data, _ := method.Encode(map[string]interface{}{
		"data": event.Data,
	})

	return &types.Transaction{
		Payload: &types.StateTransaction{
			To:    &event.ContractAddress,
			Input: data,
			Nonce: event.ID.Uint64(),
		},
	}
}

func getTransactionHash(tx *types.Transaction) (types.Hash, error) {
	stateTx, ok := tx.Payload.(*types.StateTransaction)
	if !ok {
		return types.Hash{}, fmt.Errorf("wrong tx type, expected=%s, have=%d", types.TxTypeState, tx.Type())
	}

	msgTx := &types.Transaction{
		Payload: &types.StateTransaction{
			To:    stateTx.To,
			Input: stateTx.Input,
			Nonce: stateTx.Nonce,
		},
	}

	return msgTx.ComputeHash().Hash(), nil
}
