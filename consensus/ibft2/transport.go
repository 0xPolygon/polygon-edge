package ibft2

import (
	"fmt"
	"sync"

	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/types"
)

type transportFactory func(i *Ibft2) (transport, error)

type transport interface {
	Gossip(msg *proto.MessageReq) error
}

var ibftProto = "/ibft/0.1"

func grpcTransportFactory(ibft *Ibft2) (transport, error) {
	// use a gossip protocol
	topic, err := ibft.network.NewTopic(ibftProto, &proto.MessageReq{})
	if err != nil {
		return nil, err
	}

	err = topic.Subscribe(func(obj interface{}) {
		msg := obj.(*proto.MessageReq)

		// decode signature
		if msg.From == ibft.validatorKeyAddr.String() {
			// we are the sender, skip this message since we already
			// relay our own messages internally.
			return
		}

		ibft.pushMessage(msg)
	})
	if err != nil {
		return nil, err
	}
	return &grpcTransport{topic: topic, ibft: ibft}, nil
}

type grpcTransport struct {
	proto.UnimplementedIbftServer

	topic *network.Topic
	ibft  *Ibft2
}

func (g *grpcTransport) Gossip(msg *proto.MessageReq) error {
	fmt.Println("__ GOSSIP __")
	return g.topic.Publish(msg)
}

func (g *grpcTransport) Listen() chan *proto.MessageReq {
	return nil
}

type mockNetwork struct {
	peersLock sync.Mutex
	peers     map[types.Address]*mockTransport
}

func (m *mockNetwork) gossipImpl(from types.Address, msg *proto.MessageReq) error {
	m.peersLock.Lock()
	defer m.peersLock.Unlock()

	for _, target := range m.peers {
		fmt.Println(target)
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

func (m *mockTransport) Gossip(msg *proto.MessageReq) error {
	// We send the message to ourselves as well!! Because we need to compute those votes too
	msg.From = m.addr.String()
	return m.network.gossipImpl(m.addr, msg)
}

func (m *mockTransport) Listen() chan *proto.MessageReq {
	return m.ch
}
