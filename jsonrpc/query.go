package jsonrpc

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
)

// LogFilter is a filter for logs
type LogFilter struct {
	BlockHash *types.Hash

	fromBlock BlockNumber
	toBlock   BlockNumber

	Addresses []types.Address
	Topics    [][]types.Hash
}

// addTopicSet adds specific topics to the log filter topics
func (l *LogFilter) addTopicSet(set ...string) error {
	if l.Topics == nil {
		l.Topics = [][]types.Hash{}
	}

	res := []types.Hash{}

	for _, i := range set {
		item := types.Hash{}
		if err := item.UnmarshalText([]byte(i)); err != nil {
			return err
		}

		res = append(res, item)
	}

	l.Topics = append(l.Topics, res)

	return nil
}

// addAddress Adds the address to the log filter
func (l *LogFilter) addAddress(raw string) error {
	if l.Addresses == nil {
		l.Addresses = []types.Address{}
	}

	addr := types.Address{}

	if err := addr.UnmarshalText([]byte(raw)); err != nil {
		return err
	}

	l.Addresses = append(l.Addresses, addr)

	return nil
}

func decodeLogFilterFromInterface(i interface{}) (*LogFilter, error) {
	// once the log filter is decoded as map[string]interface we cannot use unmarshal json
	raw, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}

	filter := &LogFilter{}
	if err := json.Unmarshal(raw, &filter); err != nil {
		return nil, err
	}

	return filter, nil
}

// UnmarshalJSON decodes a json object
func (l *LogFilter) UnmarshalJSON(data []byte) error {
	var obj struct {
		BlockHash *types.Hash   `json:"blockHash"`
		FromBlock string        `json:"fromBlock"`
		ToBlock   string        `json:"toBlock"`
		Address   interface{}   `json:"address"`
		Topics    []interface{} `json:"topics"`
	}

	err := json.Unmarshal(data, &obj)

	if err != nil {
		return err
	}

	l.BlockHash = obj.BlockHash

	if obj.FromBlock == "" {
		l.fromBlock = LatestBlockNumber
	} else {
		if l.fromBlock, err = stringToBlockNumber(obj.FromBlock); err != nil {
			return err
		}
	}

	if obj.ToBlock == "" {
		l.toBlock = LatestBlockNumber
	} else {
		if l.toBlock, err = stringToBlockNumber(obj.ToBlock); err != nil {
			return err
		}
	}

	if obj.Address != nil {
		// decode address, either "" or [""]
		switch raw := obj.Address.(type) {
		case string:
			// ""
			if err := l.addAddress(raw); err != nil {
				return err
			}

		case []interface{}:
			// ["", ""]
			for _, addr := range raw {
				if item, ok := addr.(string); ok {
					if err := l.addAddress(item); err != nil {
						return err
					}
				} else {
					return fmt.Errorf("address expected")
				}
			}

		default:
			return fmt.Errorf("failed to decode address. Expected either '' or ['', '']")
		}
	}

	if obj.Topics != nil {
		// decode topics, either "" or ["", ""] or null
		for _, item := range obj.Topics {
			switch raw := item.(type) {
			case string:
				// ""
				if err := l.addTopicSet(raw); err != nil {
					return err
				}

			case []interface{}:
				// ["", ""]
				res := []string{}

				for _, i := range raw {
					if item, ok := i.(string); ok {
						res = append(res, item)
					} else {
						return fmt.Errorf("hash expected")
					}
				}

				if err := l.addTopicSet(res...); err != nil {
					return err
				}

			case nil:
				// null
				if err := l.addTopicSet(); err != nil {
					return err
				}

			default:
				return fmt.Errorf("failed to decode topics. Expected '' or [''] or null")
			}
		}
	}

	// decode topics
	return nil
}

// Match returns whether the receipt includes topics for this filter
func (l *LogFilter) Match(log *types.Log) bool {
	// check addresses
	if len(l.Addresses) > 0 {
		match := false

		for _, addr := range l.Addresses {
			if addr == log.Address {
				match = true
			}
		}

		if !match {
			return false
		}
	}
	// check topics
	if len(l.Topics) > len(log.Topics) {
		return false
	}

	for i, sub := range l.Topics {
		match := len(sub) == 0

		for _, topic := range sub {
			if log.Topics[i] == topic {
				match = true

				break
			}
		}

		if !match {
			return false
		}
	}

	return true
}
