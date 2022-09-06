package sampool

import (
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/types"
)

type samBucket map[types.Hash][]rootchain.SAM

func newBucket() samBucket {
	return make(map[types.Hash][]rootchain.SAM)
}

func (b samBucket) add(msg rootchain.SAM) {
	messages, ok := b[msg.Hash]
	if !ok {
		messages = make([]rootchain.SAM, 0)
		messages = append(messages, msg)

		b[msg.Hash] = messages
	}

	messages = append(messages, msg)
	b[msg.Hash] = messages
}

func (b samBucket) exists(msg rootchain.SAM) bool {
	_, ok := b[msg.Hash]

	return ok
}

type countFunc func(uint64) bool

func (b samBucket) getReadyMessages(count countFunc) []rootchain.SAM {
	for _, messages := range b {
		if count(uint64(len(messages))) {
			return messages
		}
	}

	return nil
}
