package ibft2

import (
	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/types"
)

type transportFactory func(i *Ibft2) (transport, error)

type transport interface {
	Gossip(target []types.Address, msg *proto.MessageReq) error
	Listen() chan *proto.MessageReq
}
