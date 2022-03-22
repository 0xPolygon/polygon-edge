package transport

import (
	"github.com/0xPolygon/polygon-edge/bridge/statesync/transport/proto"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/hashicorp/go-hclog"
)

const (
	checkpointProto = "/bridge/topicCheckpoint/0.1"
	ackProto        = "/bridge/topicAck/0.1"
	noAckProto      = "/bridge/topicNoAck/0.1"
)

type CheckpointTransport interface {
	Start() error
	ProposeCheckpoint(*CheckpointProposalMessage) error
	SendAck(*AckMessage) error
	SendNoAck(*NoAckMessage) error
}

type libp2pGossipTransport struct {
	logger  hclog.Logger
	network *network.Server

	//	topics for different gossip messages
	topicCheckpoint, topicAck, topicNoAck *network.Topic
}

func NewLibp2pGossipTransport(logger hclog.Logger, network *network.Server) CheckpointTransport {
	return &libp2pGossipTransport{
		logger:  logger,
		network: network,
	}
}

func (t *libp2pGossipTransport) Start() error {
	var err error

	//	TODO: generate proto message
	if t.topicCheckpoint, err = t.network.NewTopic(
		checkpointProto,
		&proto.SignedMessage{},
	); err != nil {
		return err
	}

	//	TODO: generate proto message
	if t.topicAck, err = t.network.NewTopic(
		ackProto,
		&proto.SignedMessage{},
	); err != nil {
		return err
	}

	//	TODO: generate proto message
	if t.topicNoAck, err = t.network.NewTopic(
		noAckProto,
		&proto.SignedMessage{},
	); err != nil {
		return err
	}

	return nil
}

func (t *libp2pGossipTransport) ProposeCheckpoint(proposal *CheckpointProposalMessage) error {
	//return t.topicCheckpoint.Publish(...)

	return nil
}

func (t *libp2pGossipTransport) SendAck(ack *AckMessage) error {
	//return t.topicAck.Publish(...)

	return nil
}

func (t *libp2pGossipTransport) SendNoAck(noAck *NoAckMessage) error {
	//return t.topicNoAck.Publish(...)

	return nil
}

//	TODO: multiple subscribe
func (t *libp2pGossipTransport) Subscribe(handler func(interface{})) error {
	//return t.topic.Subscribe(func(obj interface{}) {
	// protoMessage, ok := obj.(*proto.SignedMessage)
	// if !ok {
	// 	t.logger.Warn("received unexpected typed message", "message", obj)

	// 	return
	// }

	// message := toSignedMessage(protoMessage)

	// handler(message)
	//})

	return nil
}
