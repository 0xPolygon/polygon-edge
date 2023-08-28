package jsonrpc

import (
	"reflect"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
)

var (
	addr1 = types.StringToAddress("1")
	addr2 = types.StringToAddress("2")

	hash1 = types.StringToHash("1")
	hash2 = types.StringToHash("2")
	hash3 = types.StringToHash("3")
	hash4 = types.StringToHash("4")
)

func TestFilterDecode(t *testing.T) {
	cases := []struct {
		str string
		res *LogQuery
	}{
		{
			`{}`,
			&LogQuery{
				fromBlock: LatestBlockNumber,
				toBlock:   LatestBlockNumber,
			},
		},
		{
			`{
				"address": "1"
			}`,
			nil,
		},
		{
			`{
				"address": "` + addr1.String() + `"
			}`,
			&LogQuery{
				fromBlock: LatestBlockNumber,
				toBlock:   LatestBlockNumber,
				Addresses: []types.Address{
					addr1,
				},
			},
		},
		{
			`{
				"address": [
					"` + addr1.String() + `",
					"` + addr2.String() + `"
				]
			}`,
			&LogQuery{
				fromBlock: LatestBlockNumber,
				toBlock:   LatestBlockNumber,
				Addresses: []types.Address{
					addr1,
					addr2,
				},
			},
		},
		{
			`{
				"topics": [
					"` + hash1.String() + `",
					[
						"` + hash1.String() + `"
					],
					[
						"` + hash1.String() + `",
						"` + hash2.String() + `"
					],
					null,
					"` + hash1.String() + `"
				]
			}`,
			&LogQuery{
				fromBlock: LatestBlockNumber,
				toBlock:   LatestBlockNumber,
				Topics: [][]types.Hash{
					{
						hash1,
					},
					{
						hash1,
					},
					{
						hash1,
						hash2,
					},
					{},
					{
						hash1,
					},
				},
			},
		},
		{
			`{
				"fromBlock": "pending",
				"toBlock": "earliest"
			}`,
			&LogQuery{
				fromBlock: PendingBlockNumber,
				toBlock:   EarliestBlockNumber,
			},
		},
		{
			`{
				"blockHash": "` + hash1.String() + `"
			}`,
			&LogQuery{
				BlockHash: &hash1,
				fromBlock: LatestBlockNumber,
				toBlock:   LatestBlockNumber,
			},
		},
	}

	for indx, c := range cases {
		res := &LogQuery{}
		err := res.UnmarshalJSON([]byte(c.str))

		if err != nil && c.res != nil {
			t.Fatal(err)
		}

		if err == nil && c.res == nil {
			t.Fatal("it should fail")
		}

		if c.res != nil {
			if !reflect.DeepEqual(res, c.res) {
				t.Fatalf("bad %d", indx)
			}
		}
	}
}

func TestFilterMatch(t *testing.T) {
	cases := []struct {
		filter LogQuery
		log    *types.Log
		match  bool
	}{
		{
			// correct, exact match
			LogQuery{
				Topics: [][]types.Hash{
					{
						hash1,
					},
				},
			},
			&types.Log{
				Topics: []types.Hash{
					hash1,
				},
			},
			true,
		},
		{
			// bad, the filter has two hashes
			LogQuery{
				Topics: [][]types.Hash{
					{
						hash1,
					},
					{
						hash1,
					},
				},
			},
			&types.Log{
				Topics: []types.Hash{
					hash1,
				},
			},
			false,
		},
		{
			// correct, wildcard in one hash
			LogQuery{
				Topics: [][]types.Hash{
					{},
					{
						hash2,
					},
				},
			},
			&types.Log{
				Topics: []types.Hash{
					hash1,
					hash2,
				},
			},
			true,
		},
		{
			// correct, more topics than in filter
			LogQuery{
				Topics: [][]types.Hash{
					{
						hash1,
					},
					{
						hash2,
					},
				},
			},
			&types.Log{
				Topics: []types.Hash{
					hash1,
					hash2,
					hash3,
				},
			},
			true,
		},
	}

	for indx, c := range cases {
		if c.filter.Match(c.log) != c.match {
			t.Fatalf("bad %d", indx)
		}
	}
}
