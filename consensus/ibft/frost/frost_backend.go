package frost

import (
	"github.com/0xPolygon/go-ibft/core"
	"github.com/topos-protocol/go-topos-sequencer-client/frostclient"
	protofrost "github.com/topos-protocol/go-topos-sequencer-client/frostclient/proto"
)

type FrostTransport interface {
	MulticastFrost(msg *protofrost.FrostMessage)
}

type FrostBackend struct {
	// GRPC address of the topos sequencer service
	address string

	// Public validator account address
	validatorID string

	// log is the logger instance
	log core.Logger

	// grpc client for topos-sequencer service
	client *frostclient.FrostServiceClient

	// Network interface for frost topic
	transport FrostTransport
}

func (fb *FrostBackend) GetToposSequencerAddr() string {
	return fb.address
}

func NewFrostBackend(toposSequencerAddr string, logger core.Logger) (*FrostBackend, error) {
	return &FrostBackend{
		address: toposSequencerAddr,
		log:     logger,
	}, nil
}

func (fb *FrostBackend) Initialize(serverAddress string, validatorAccount string, transport FrostTransport) error {
	fb.transport = transport

	frostServiceClient, err := frostclient.NewFrostServiceClient(serverAddress, validatorAccount)
	if err != nil {
		fb.log.Error("could not instantiate frost client: %v", err)

		return err
	}

	fb.client = frostServiceClient

	// Start loop for listening messages from frost-sequencer
	go func() {
		for {
			select {
			case message := <-frostServiceClient.Inbox:
				fb.log.Info("Received message from frost-sequencer:", message)

				switch op := message.Event.(type) {
				case *protofrost.WatchFrostMessagesResponse_FrostMessagePushed_:
					// New froost message received from local topos sequencer
					err := fb.PublishFrost(op.FrostMessagePushed.FrostMessage)
					if err != nil {
						fb.log.Error("unable to publish frost message: %v", err)
					}
				case *protofrost.WatchFrostMessagesResponse_StreamOpened_:
					// Connection to local topos sequencer is opened
					fb.log.Info("stream opened to service ", serverAddress)
				}
			}
		}
	}()

	return nil
}

func (fb *FrostBackend) PublishFrost(message *protofrost.FrostMessage) error {
	fb.transport.MulticastFrost(message)

	return nil
}

func (fb *FrostBackend) ProcessGossipedMessages(message *protofrost.FrostMessage) error {
	request := &protofrost.SubmitFrostMessageRequest{
		FrostMessage: message,
	}

	_, err := fb.client.Client.SubmitFrostMessage(fb.client.Ctx, request)
	if err != nil {
		fb.log.Error("unable to submit frost message: %v", err)
	}

	return nil
}
