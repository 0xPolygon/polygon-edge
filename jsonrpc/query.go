package jsonrpc

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
)

// LogQuery is a query to filter logs
type LogQuery struct {
	BlockHash *types.Hash

	fromBlock BlockNumber
	toBlock   BlockNumber

	Addresses []types.Address
	Topics    [][]types.Hash
}

// addTopicSet adds specific topics to the log filter topics
func (q *LogQuery) addTopicSet(set ...string) error {
	if q.Topics == nil {
		q.Topics = [][]types.Hash{}
	}

	res := []types.Hash{}

	for _, i := range set {
		item := types.Hash{}
		if err := item.UnmarshalText([]byte(i)); err != nil {
			return err
		}

		res = append(res, item)
	}

	q.Topics = append(q.Topics, res)

	return nil
}

// addAddress Adds the address to the log filter
func (q *LogQuery) addAddress(raw string) error {
	if q.Addresses == nil {
		q.Addresses = []types.Address{}
	}

	addr := types.Address{}

	if err := addr.UnmarshalText([]byte(raw)); err != nil {
		return err
	}

	q.Addresses = append(q.Addresses, addr)

	return nil
}

func decodeLogQueryFromInterface(i interface{}) (*LogQuery, error) {
	// once the log filter is decoded as map[string]interface we cannot use unmarshal json
	raw, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}

	query := &LogQuery{}
	if err := json.Unmarshal(raw, &query); err != nil {
		return nil, err
	}

	return query, nil
}

// UnmarshalJSON decodes a json object
func (q *LogQuery) UnmarshalJSON(data []byte) error {
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

	q.BlockHash = obj.BlockHash

	// pending from/to blocks or "" is treated as a latest block
	if q.fromBlock, err = stringToBlockNumberSafe(obj.FromBlock); err != nil {
		return err
	}

	if q.toBlock, err = stringToBlockNumberSafe(obj.ToBlock); err != nil {
		return err
	}

	if obj.Address != nil {
		// decode address, either "" or [""]
		switch raw := obj.Address.(type) {
		case string:
			// ""
			if err := q.addAddress(raw); err != nil {
				return err
			}

		case []interface{}:
			// ["", ""]
			for _, addr := range raw {
				if item, ok := addr.(string); ok {
					if err := q.addAddress(item); err != nil {
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
				if err := q.addTopicSet(raw); err != nil {
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

				if err := q.addTopicSet(res...); err != nil {
					return err
				}

			case nil:
				// null
				if err := q.addTopicSet(); err != nil {
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
func (q *LogQuery) Match(log *types.Log) bool {
	// check addresses
	if len(q.Addresses) > 0 {
		match := false

		for _, addr := range q.Addresses {
			if addr == log.Address {
				match = true
			}
		}

		if !match {
			return false
		}
	}
	// check topics
	if len(q.Topics) > len(log.Topics) {
		return false
	}

	for i, sub := range q.Topics {
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
