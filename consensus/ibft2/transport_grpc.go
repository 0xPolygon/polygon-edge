package ibft2

import (
	"fmt"

	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/types"
)

var ibftProto = "/ibft/0.1"

func grpcTransportFactory(ibft *Ibft2) (transport, error) {
	// use a gossip protocol
	topic, err := ibft.network.NewTopic(ibftProto, &proto.MessageReq{})
	if err != nil {
		return nil, err
	}

	err = topic.Subscribe(func(obj interface{}) {
		msg := obj.(*proto.MessageReq)

		if msg.From == ibft.validatorKeyAddr.String() {
			// we are the sender, skip this message since we already
			// relay our own messages internally.
			return
		}

		fmt.Println("__ MSG __")
		// fmt.Println(msg)

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

func (g *grpcTransport) Gossip(target []types.Address, msg *proto.MessageReq) error {
	fmt.Println("__ GOSSIP __")
	return g.topic.Publish(msg)
}

func (g *grpcTransport) Listen() chan *proto.MessageReq {
	return nil
}
