package network

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	testproto "github.com/0xPolygon/polygon-sdk/network/proto"
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
	numServers := 2
	sentMessage := fmt.Sprintf("%d", time.Now().Unix())
	servers, createErr := createServers(numServers, []*CreateServerParams{nil, nil})
	if createErr != nil {
		t.Fatalf("Unable to create servers, %v", createErr)
	}

	MultiJoin(t, servers[0], servers[1])
	time.Sleep(time.Second * 2) // TODO add mesh comment

	topicName := "msg-pub-sub"
	serverTopics := make([]*Topic, numServers)

	messageCh := make(chan *testproto.GenericMessage)
	for i := 0; i < numServers; i++ {
		topic, topicErr := servers[i].NewTopic(topicName, &testproto.GenericMessage{})
		if topicErr != nil {
			t.Fatalf("Unable to create topic, %v", topicErr)
		}

		serverTopics[i] = topic

		if subscribeErr := topic.Subscribe(func(obj interface{}) {
			if i != 0 {
				// Everyone should relay they got the message apart from the publisher
				messageCh <- obj.(*testproto.GenericMessage)
			}
		}); subscribeErr != nil {
			t.Fatalf("Unable to subscribe to topic, %v", subscribeErr)
		}
	}
	publisher := servers[0]
	publisherTopic := serverTopics[0]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if waitErr := WaitForSubscribers(ctx, publisher, topicName, len(servers)); waitErr != nil {
		t.Fatalf("Unable to wait for subscribers, %v", waitErr)
	}

	if publishErr := publisherTopic.Publish(
		&testproto.GenericMessage{
			Message: sentMessage,
		}); publishErr != nil {
		t.Fatalf("Unable to publish message, %v", publishErr)
	}

	messagesGossiped := 0
	for {
		select {
		case <-time.After(time.Second * 5):
			t.Fatalf("Gossip messages not received before timeout")
		case message := <-messageCh:
			if message.Message == sentMessage {
				messagesGossiped++
				if messagesGossiped == len(servers)-1 {
					break
				}
			}
		}
	}
}
