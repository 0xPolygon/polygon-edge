package txpool

import (
	"sync"

	"github.com/0xPolygon/polygon-edge/types"
)

// TxStatus indicates the status of tx that is added to TxPool
type TxStatus int8

const (
	TxSuccessful TxStatus = iota // Tx has been mined in a block successfully
	TxDropped                    // Tx was dropped for some reason
)

var (
	// txStatusToString is a mapping from TxStatus to text status
	txStatusToString = map[TxStatus]string{
		TxSuccessful: "success",
		TxDropped:    "dropped",
	}
)

// String returns text status from status code
func (s *TxStatus) String() string {
	if s == nil {
		return ""
	}

	return txStatusToString[*s]
}

// Journal records the statuses of transactions added into TxPool
type Journal interface {
	// logSuccessfulTx marked the Tx as mined
	logSuccessfulTx(txHash types.Hash)
	// logDroppedTx marked the Tx as dropped
	logDroppedTx(txHash types.Hash)
	// resetTxStatus removes the status of the tx from journal
	resetTxStatus(txHash types.Hash)
	// txStatus returns the Tx status recorded in Journal
	txStatus(txHash types.Hash) *TxStatus
}

// memoryJournal is an implementation of Journal and it saves data in-memory
type memoryJournal struct {
	txHashMap sync.Map
}

// newMemoryJournal initializes memoryJournal
func newMemoryJournal() *memoryJournal {
	return &memoryJournal{
		txHashMap: sync.Map{},
	}
}

// logSuccessfulTx marked the Tx as mined
func (j *memoryJournal) logSuccessfulTx(txHash types.Hash) {
	j.txHashMap.Store(txHash, true)
}

// logDroppedTx marked the Tx as dropped
func (j *memoryJournal) logDroppedTx(txHash types.Hash) {
	j.txHashMap.Store(txHash, false)
}

// resetTxStatus removes the status of the tx from journal
func (j *memoryJournal) resetTxStatus(txHash types.Hash) {
	j.txHashMap.Delete(txHash)
}

// txStatus returns the Tx status recorded in Journal
func (j *memoryJournal) txStatus(txHash types.Hash) *TxStatus {
	rawValue, ok := j.txHashMap.Load(txHash)
	if !ok {
		return nil
	}

	value, ok := rawValue.(bool)
	if !ok {
		return nil
	}

	txStatus := TxSuccessful
	if !value {
		txStatus = TxDropped
	}

	return &txStatus
}
