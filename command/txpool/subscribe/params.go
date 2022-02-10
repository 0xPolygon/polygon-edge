package subscribe

import "github.com/0xPolygon/polygon-edge/txpool/proto"

var (
	params = &subscribeParams{}
)

var (
	addedFlag          = "added"
	promotedFlag       = "promoted"
	demotedFlag        = "demoted"
	enqueuedFlag       = "enqueued"
	droppedFlag        = "dropped"
	prunedPromotedFlag = "pruned-promoted"
	prunedEnqueuedFlag = "pruned-enqueued"
)

type subscribeParams struct {
	eventSubscriptionMap map[proto.EventType]*bool
	supportedEvents      []proto.EventType
}

func (sp *subscribeParams) initEventMap() {
	falseRaw := false
	sp.eventSubscriptionMap = map[proto.EventType]*bool{
		proto.EventType_ADDED:           &falseRaw,
		proto.EventType_ENQUEUED:        &falseRaw,
		proto.EventType_PROMOTED:        &falseRaw,
		proto.EventType_DROPPED:         &falseRaw,
		proto.EventType_DEMOTED:         &falseRaw,
		proto.EventType_PRUNED_PROMOTED: &falseRaw,
		proto.EventType_PRUNED_ENQUEUED: &falseRaw,
	}
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
		proto.EventType_PRUNED_PROMOTED,
		proto.EventType_PRUNED_ENQUEUED,
	}
}
