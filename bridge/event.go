package bridge

import (
	"errors"

	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/go-web3"
)

type StateSyncEvent struct {
	ID              *big.Int
	ContractAddress types.Address
	Data            []byte
}

func ParseStateSyncEvent(log *web3.Log) (*StateSyncEvent, error) {
	event, err := StateSyncedEvent.ParseLog(log)
	if err != nil {
		return nil, err
	}

	var (
		id           *big.Int
		contractAddr web3.Address
		data         []byte
		ok           bool
	)

	id, ok = event["id"].(*big.Int)
	if !ok {
		return nil, errors.New("failed to parse ID in StateSyncedEvent")
	}

	contractAddr, ok = event["contractAddress"].(web3.Address)
	if !ok {
		return nil, errors.New("failed to parse contractAddress in StateSyncedEvent")
	}

	data, ok = event["data"].([]uint8)
	if !ok {
		return nil, errors.New("failed to parse data in StateSyncedEvent")
	}

	return &StateSyncEvent{
		ID:              id,
		ContractAddress: types.BytesToAddress(contractAddr.Bytes()),
		Data:            data,
	}, nil
}
