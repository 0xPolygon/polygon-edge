package subscribe

import "github.com/0xPolygon/polygon-edge/txpool/proto"

var (
	params = &subscribeParams{
		eventSubscriptionMap: make(map[proto.EventType]*bool),
	}
)

type subscribeParams struct {
	eventSubscriptionMap map[proto.EventType]*bool
	supportedEvents      []proto.EventType
}

func (sp *subscribeParams) init() {
	sp.setSpecifiedEvents()

	if !sp.areEventsSpecified() {
		sp.setAllEvents()
	}
}

func (sp *subscribeParams) setSpecifiedEvents() {
	sp.supportedEvents = make([]proto.EventType, 0)

	for eventType, eventIsSupported := range sp.eventSubscriptionMap {
		if *eventIsSupported {
			sp.supportedEvents = append(sp.supportedEvents, eventType)
		}
	}
}

func (sp *subscribeParams) areEventsSpecified() bool {
	return len(sp.supportedEvents) != 0
}

func (sp *subscribeParams) setAllEvents() {
	sp.supportedEvents = []proto.EventType{
		proto.EventType_ADDED,
		proto.EventType_ENQUEUED,
		proto.EventType_PROMOTED,
		proto.EventType_DROPPED,
		proto.EventType_DEMOTED,
	}
}
