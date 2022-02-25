package bridge

import (
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
		Type:  types.TxTypeState,
		To:    &event.ContractAddress,
		Input: data,
	}
}
