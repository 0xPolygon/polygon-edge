package network

import (
	"context"
	"errors"
	"testing"
	"time"

	testproto "github.com/0xPolygon/polygon-sdk/network/proto/test"
	"github.com/stretchr/testify/assert"
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

func TestGossip(t *testing.T) {
	srv0 := CreateServer(t, nil)
	srv1 := CreateServer(t, nil)

	MultiJoin(t, srv0, srv1)

	topicName := "topic/0.1"

	topic0, err := srv0.NewTopic(topicName, &testproto.AReq{})
	assert.NoError(t, err)

	topic1, err := srv1.NewTopic(topicName, &testproto.AReq{})
	assert.NoError(t, err)

	// subscribe in topic1
	msgCh := make(chan *testproto.AReq)
	topic1.Subscribe(func(obj interface{}) {
		msgCh <- obj.(*testproto.AReq)
	})

	// wait until build mesh
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	WaitForSubscribers(ctx, srv0, topicName, 1)

	// publish in topic0
	assert.NoError(t, topic0.Publish(&testproto.AReq{Msg: "a"}))

	select {
	case msg := <-msgCh:
		assert.Equal(t, msg.Msg, "a")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}
