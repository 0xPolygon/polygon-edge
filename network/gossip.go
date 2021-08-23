package network

import (
	"context"
	"reflect"

	"github.com/hashicorp/go-hclog"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"google.golang.org/protobuf/proto"
)

type Topic struct {
	logger hclog.Logger

	topic   *pubsub.Topic
	typ     reflect.Type
	closeCh chan struct{}
}

func (t *Topic) createObj() proto.Message {
	return reflect.New(t.typ).Interface().(proto.Message)
}

func (t *Topic) Publish(obj proto.Message) error {
	data, err := proto.Marshal(obj)
	if err != nil {
		return err
	}

	return t.topic.Publish(context.Background(), data)
}

func (t *Topic) Subscribe(handler func(obj interface{})) error {
	sub, err := t.topic.Subscribe()
	if err != nil {
		return err
	}

	go t.readLoop(sub, handler)

	return nil
}

func (t *Topic) readLoop(sub *pubsub.Subscription, handler func(obj interface{})) {
	ctx, cancelFn := context.WithCancel(context.Background())
	go func() {
		<-t.closeCh
		cancelFn()
	}()

	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			t.logger.Error("failed to get topic", "err", err)
			continue
		}

		obj := t.createObj()
		if err := proto.Unmarshal(msg.Data, obj); err != nil {
			t.logger.Error("failed to unmarshal topic", "err", err)
			continue
		}
		handler(obj)
	}
}

func (s *Server) NewTopic(protoID string, obj proto.Message) (*Topic, error) {
	topic, err := s.ps.Join(protoID)
	if err != nil {
		return nil, err
	}

	tt := &Topic{
		logger: s.logger.Named(protoID),
		topic:  topic,
		typ:    reflect.TypeOf(obj).Elem(),
	}

	return tt, nil
}
