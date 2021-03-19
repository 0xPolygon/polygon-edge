package aux

import (
	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/types"
)

type BlockchainInterface interface {
	GetHeaderByNumber(n uint64) (*types.Header, bool)
	SubscribeEvents() blockchain.Subscription
	GetHeaderByHash(hash types.Hash) (*types.Header, bool)
	Header() *types.Header
	Genesis() types.Hash
}

type MockBlockchain struct {
	headers []*types.Header
	sub     *blockchain.MockSubscription
}

func NewMockBlockchain() *MockBlockchain {
	return &MockBlockchain{
		headers: []*types.Header{},
	}
}

func (m *MockBlockchain) SubscribeEvents() blockchain.Subscription {
	sub := blockchain.NewMockSubscription()
	return sub
}

func (m *MockBlockchain) GetGenesis() *types.Header {
	return m.headers[0]
}

func (m *MockBlockchain) Append(h *types.Header) {
	if m.headers == nil {
		m.headers = []*types.Header{}
	}
	m.headers = append(m.headers, h)
	if m.sub != nil {
		m.sub.Push(&blockchain.Event{
			NewChain: []*types.Header{
				h.Copy(),
			},
		})
	}
}

func (m *MockBlockchain) findHeader(f func(i *types.Header) bool) (*types.Header, bool) {
	for _, i := range m.headers {
		if f(i) {
			return i, true
		}
	}
	return nil, false
}

func (m *MockBlockchain) GetHeaderByNumber(n uint64) (*types.Header, bool) {
	return m.findHeader(func(i *types.Header) bool {
		return i.Number == n
	})
}

func (m *MockBlockchain) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	return m.findHeader(func(i *types.Header) bool {
		return i.Hash == hash
	})
}

func (m *MockBlockchain) Header() *types.Header {
	return m.headers[len(m.headers)-1]
}

func (m *MockBlockchain) Genesis() types.Hash {
	return m.headers[0].Hash
}

type StoreInterface interface {
	Set(p []byte, v []byte) error
	Get(p []byte) ([]byte, bool, error)
}
