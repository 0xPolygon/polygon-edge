//go:build !doubleSigner

package polybft

import ibftProto "github.com/0xPolygon/go-ibft/messages/proto"

func (p *Polybft) ibftMsgMulticast(msg *ibftProto.Message) {
}
