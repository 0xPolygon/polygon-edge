package statesync

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
)

const (
	StateSyncedEventABI = `event StateSynced(uint256 indexed id, address indexed contractAddress, bytes data)`
)

var (
	StateSyncedEvent = abi.MustNewEvent(StateSyncedEventABI)
)

var (
	ErrInvalidID              = errors.New("id isn't in event or wrong type")
	ErrInvalidContractAddress = errors.New("contractAddress isn't in event or wrong type")
	ErrInvalidData            = errors.New("data isn't in event or wrong type")
)

func eventToMessage(log *web3.Log) (*Message, error) {
	switch {
	case StateSyncedEvent.Match(log):
		event, err := ParseStateSyncEvent(log)
		if err != nil {
			return nil, err
		}

		tx := NewStateSyncedTx(event)

		return &Message{
			ID:          event.ID,
			Transaction: tx,
		}, nil
	}

	return nil, nil
}

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
		return nil, fmt.Errorf("failed to parse StateSyncEvent: %w", ErrInvalidID)
	}

	contractAddr, ok = event["contractAddress"].(web3.Address)
	if !ok {
		return nil, fmt.Errorf("failed to parse StateSyncEvent: %w", ErrInvalidContractAddress)
	}

	data, ok = event["data"].([]uint8)
	if !ok {
		return nil, fmt.Errorf("failed to parse StateSyncEvent: %w", ErrInvalidData)
	}

	return &StateSyncEvent{
		ID:              id,
		ContractAddress: types.BytesToAddress(contractAddr.Bytes()),
		Data:            data,
	}, nil
}
