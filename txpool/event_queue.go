package txpool

import (
	"sync"

	"github.com/0xPolygon/polygon-edge/txpool/proto"
)

type eventQueue struct {
	events []*proto.TxPoolEvent
	sync.Mutex
}

func (es *eventQueue) push(event *proto.TxPoolEvent) {
	es.Lock()
	defer es.Unlock()

	es.events = append(es.events, event)
}

func (es *eventQueue) pop() *proto.TxPoolEvent {
	es.Lock()
	defer es.Unlock()

	if len(es.events) == 0 {
		return nil
	}

	event := es.events[0]
	es.events = es.events[1:]

	return event
}
