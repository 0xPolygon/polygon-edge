package network

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	testproto "github.com/0xPolygon/polygon-edge/network/proto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func NumSubscribers(srv *Server, topic string) int {
	return len(srv.ps.ListPeers(topic))
}

func WaitForSubscribers(ctx context.Context, srv *Server, topic string, expectedNumPeers int) error {
	for {
		if n := NumSubscribers(srv, topic); n >= expectedNumPeers {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("canceled")
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}
}

func TestSimpleGossip(t *testing.T) {
	numServers := 9
	sentMessage := fmt.Sprintf("%d", time.Now().UTC().Unix())

	servers, createErr := createServers(numServers, nil)
	require.NoError(t, createErr, "Unable to create servers")

	messageCh := make(chan *testproto.GenericMessage)

	t.Cleanup(func() {
		close(messageCh)
		closeTestServers(t, servers)
	})

	joinErrors := MeshJoin(servers...)
	require.Empty(t, joinErrors, "Unable to join servers [%d], %v", len(joinErrors), joinErrors)

	topicName := "msg-pub-sub"
	serverTopics := make([]*Topic, numServers)

	for i := 0; i < numServers; i++ {
		topic, topicErr := servers[i].NewTopic(topicName, &testproto.GenericMessage{})
		require.NoError(t, topicErr, "Unable to create topic")

		serverTopics[i] = topic

		subscribeErr := topic.Subscribe(func(obj interface{}, _ peer.ID) {
			// Everyone should relay they got the message
			genericMessage, ok := obj.(*testproto.GenericMessage)
			require.True(t, ok, "invalid type assert")

			messageCh <- genericMessage
		})
		require.NoError(t, subscribeErr, "Unable to subscribe to topic")
	}

	publisher := servers[0]
	publisherTopic := serverTopics[0]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := WaitForSubscribers(ctx, publisher, topicName, len(servers)-1)
	require.NoError(t, err, "Unable to wait for subscribers")

	err = publisherTopic.Publish(
		&testproto.GenericMessage{
			Message: sentMessage,
		})
	require.NoError(t, err, "Unable to publish message")

	messagesGossiped := 0

	for {
		select {
		case <-time.After(time.Second * 15):
			t.Fatalf("Multicast messages not received before timeout")
		case message := <-messageCh:
			if message.Message == sentMessage {
				messagesGossiped++
				if messagesGossiped == len(servers) {
					return
				}
			}
		}
	}
}

func Test_RepeatedClose(t *testing.T) {
	topic := &Topic{
		closeCh: make(chan struct{}),
	}

	// Call Close() twice to ensure that underlying logic (e.g. channel close) is
	// only executed once.
	topic.Close()
	topic.Close()
}
