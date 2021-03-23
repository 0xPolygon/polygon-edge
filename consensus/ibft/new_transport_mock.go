package ibft

import "github.com/0xPolygon/minimal/consensus/ibft/proto"

type mockTransport struct {
}

func (m *mockTransport) broadcast(msg *proto.MessageReq) {

}
