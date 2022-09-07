package sampool

import (
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/types"
)

type uniqueSAMs struct {
	messages   []rootchain.SAM
	signatures map[string]bool
}

func newUniqueSAMs() uniqueSAMs {
	return uniqueSAMs{
		messages:   make([]rootchain.SAM, 0),
		signatures: make(map[string]bool),
	}
}

func (s *uniqueSAMs) add(msg rootchain.SAM) {
	strSignature := string(msg.Signature)

	if s.signatures[strSignature] {
		return
	}

	s.messages = append(s.messages, msg)
	s.signatures[strSignature] = true
}

func (s *uniqueSAMs) get() []rootchain.SAM {
	return s.messages
}

type samBucket map[types.Hash]uniqueSAMs

func newBucket() samBucket {
	return make(map[types.Hash]uniqueSAMs)
}

func (b samBucket) add(msg rootchain.SAM) {
	messages, ok := b[msg.Hash]
	if !ok {
		messages = newUniqueSAMs()
	}

	messages.add(msg)
	b[msg.Hash] = messages
}

func (b samBucket) exists(msg rootchain.SAM) bool {
	_, ok := b[msg.Hash]

	return ok
}

type countFunc func(uint64) bool

func (b samBucket) getReadyMessages(count countFunc) []rootchain.SAM {
	for _, messages := range b {
		unique := messages.get()

		if count(uint64(len(unique))) {
			return unique
		}
	}

	return nil
}
