package sampool

import (
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/types"
)

type samSet struct {
	messages []rootchain.SAM
	isAdded  map[string]bool
}

func newSet() samSet {
	return samSet{
		messages: make([]rootchain.SAM, 0),
		isAdded:  make(map[string]bool),
	}
}

func (s *samSet) add(msg rootchain.SAM) {
	strSignature := string(msg.Signature)

	if s.isAdded[strSignature] {
		return
	}

	s.messages = append(s.messages, msg)
	s.isAdded[strSignature] = true
}

func (s *samSet) getMessages() []rootchain.SAM {
	return s.messages
}

type samBucket map[types.Hash]samSet

func newBucket() samBucket {
	return make(map[types.Hash]samSet)
}

func (b samBucket) add(msg rootchain.SAM) {
	messages, ok := b[msg.Hash]
	if !ok {
		messages = newSet()
	}

	messages.add(msg)
	b[msg.Hash] = messages
}

func (b samBucket) getMessagesWithMostSignatures() []rootchain.SAM {
	var (
		max  = 0
		hash = types.Hash{}
	)

	for h, set := range b {
		if msgCount := len(set.getMessages()); msgCount > max {
			hash = h
			max = msgCount
		}
	}

	if max == 0 {
		return nil
	}

	maxSet := b[hash]

	return maxSet.getMessages()
}
