package filter

import (
	"reflect"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
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
		res *LogFilter
	}{
		{
			`{}`,
			&LogFilter{},
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
			&LogFilter{
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
			&LogFilter{
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
			&LogFilter{
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
	}

	for _, c := range cases {
		res := &LogFilter{}
		err := res.UnmarshalJSON([]byte(c.str))
		if err != nil && c.res != nil {
			t.Fatal(err)
		}
		if err == nil && c.res == nil {
			t.Fatal("it should fail")
		}
		if c.res != nil {
			if !reflect.DeepEqual(res, c.res) {
				t.Fatal("bad")
			}
		}
	}
}

func TestFilterMatch(t *testing.T) {
	cases := []struct {
		filter LogFilter
		log    *types.Log
		match  bool
	}{
		{
			// correct, exact match
			LogFilter{
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
			LogFilter{
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
			LogFilter{
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
			LogFilter{
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

func TestLogFilter(t *testing.T) {

}

func TestBlockFilter(t *testing.T) {
	store := &mockStore{
		header: &types.Header{
			Hash: types.StringToHash("0"),
		},
	}

	m := NewFilterManager(hclog.NewNullLogger(), store)
	go m.Run()

	// add block filter
	id := m.addFilter(nil)

	// emit two events
	store.emitEvent(&blockchain.Event{
		NewChain: []*types.Header{
			{
				Hash: types.StringToHash("1"),
			},
			{
				Hash: types.StringToHash("2"),
			},
		},
	})

	store.emitEvent(&blockchain.Event{
		NewChain: []*types.Header{
			{
				Hash: types.StringToHash("3"),
			},
		},
	})

	// we need to wait for the manager to process the data
	time.Sleep(500 * time.Millisecond)

	m.GetFilterChanges(id)

	// emit one more event, it should not return the
	// first three hashes
	store.emitEvent(&blockchain.Event{
		NewChain: []*types.Header{
			{
				Hash: types.StringToHash("4"),
			},
		},
	})

	time.Sleep(500 * time.Millisecond)

	m.GetFilterChanges(id)
}

func TestTimeout(t *testing.T) {
	store := &mockStore{}

	m := NewFilterManager(hclog.NewNullLogger(), store)
	m.timeout = 2 * time.Second

	go m.Run()

	// add block filter
	id := m.addFilter(nil)

	assert.True(t, m.Exists(id))
	time.Sleep(2 * time.Second)
	assert.False(t, m.Exists(id))
}

func TestHeadStream(t *testing.T) {
	b := &blockStream{}

	b.push(types.StringToHash("1"))
	b.push(types.StringToHash("2"))

	cur := b.Head()

	b.push(types.StringToHash("3"))
	b.push(types.StringToHash("4"))

	// get the updates, there are two new entries
	updates, next := cur.getUpdates()

	assert.Equal(t, updates[0], types.StringToHash("3"))
	assert.Equal(t, updates[1], types.StringToHash("4"))

	// there are no new entries
	updates, _ = next.getUpdates()
	assert.Len(t, updates, 0)
}

type mockStore struct {
	header  *types.Header
	eventCh chan blockchain.Event
}

func (m *mockStore) emitEvent(evnt *blockchain.Event) {
	m.eventCh <- *evnt
}

func (m *mockStore) Header() *types.Header {
	return m.header
}

func (m *mockStore) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	return nil, nil
}

// Subscribe subscribes for chain head events
func (m *mockStore) Subscribe() subscription {
	return m
}

func (m *mockStore) Watch() chan blockchain.Event {
	m.eventCh = make(chan blockchain.Event)
	return m.eventCh
}

func (m *mockStore) Close() {

}
