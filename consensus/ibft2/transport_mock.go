package ibft2

import (
	"sync"

	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/types"
)

type mockNetwork struct {
	peersLock sync.Mutex
	peers     map[types.Address]*mockTransport
}

func (m *mockNetwork) gossipImpl(from types.Address, to []types.Address, msg *proto.MessageReq) error {
	m.peersLock.Lock()
	defer m.peersLock.Unlock()

	for _, target := range to {
		p, ok := m.peers[target]
		if ok {
			// TODO: make a copy
			p.ch <- msg
		} else {
			panic("X")
		}
	}
	return nil
}

func (m *mockNetwork) newTransport(i *Ibft2) (transport, error) {
	if m.peers == nil {
		m.peers = map[types.Address]*mockTransport{}
	}
	tt := &mockTransport{
		network: m,
		addr:    i.validatorKeyAddr,
		ch:      make(chan *proto.MessageReq, 10),
	}
	m.peers[i.validatorKeyAddr] = tt
	return tt, nil
}

type mockTransport struct {
	network *mockNetwork
	addr    types.Address
	ch      chan *proto.MessageReq
}

func (m *mockTransport) Gossip(target []types.Address, msg *proto.MessageReq) error {
	// We send the message to ourselves as well!! Because we need to compute those votes too
	msg.From = m.addr.String()
	return m.network.gossipImpl(m.addr, target, msg)
}

func (m *mockTransport) Listen() chan *proto.MessageReq {
	return m.ch
}
