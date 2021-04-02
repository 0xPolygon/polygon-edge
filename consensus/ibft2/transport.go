package ibft2

import "github.com/0xPolygon/minimal/consensus/ibft2/proto"

type transportFactory func(i *Ibft2) (transport, error)

type transport interface {
	Gossip(msg *proto.MessageReq) error
	Listen() chan *proto.MessageReq
}
