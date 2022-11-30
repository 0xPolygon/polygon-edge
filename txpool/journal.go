package txpool

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	lru "github.com/hashicorp/golang-lru"
)

const (
	TxErrorJournalDefaultSize = 1000
)

// TxDiscardJournal records the reason of transaction discard
type TxDiscardJournal interface {
	// addDiscardTx records the reason of discard
	logDiscardedTx(txHash types.Hash, reason string)

	// getReason return the reason of transaction discard
	GetReason(txHash types.Hash) (*string, error)
}

// memoryJournal is an implementation of TxDiscardJournal and it saves data in-memory
type memoryTxErrorJournal struct {
	reasons *lru.Cache
}

// newMemoryJournal initializes memoryJournal
func newMemoryTxErrorJournal() (*memoryTxErrorJournal, error) {
	reasons, err := lru.New(TxErrorJournalDefaultSize)
	if err != nil {
		return nil, err
	}

	return &memoryTxErrorJournal{
		reasons: reasons,
	}, nil
}

// logDiscardedTx records the reason of transaction discard
func (j *memoryTxErrorJournal) logDiscardedTx(txHash types.Hash, reason string) {
	j.reasons.Add(txHash, reason)
}

// getReason returns the reason
func (j *memoryTxErrorJournal) GetReason(txHash types.Hash) (*string, error) {
	rawData, ok := j.reasons.Get(txHash)
	if !ok {
		return nil, nil
	}

	reason, ok := rawData.(string)
	if !ok {
		// should not reach here
		return nil, fmt.Errorf("wrong type conversion, expected=string actual=%T", rawData)
	}

	return &reason, nil
}
