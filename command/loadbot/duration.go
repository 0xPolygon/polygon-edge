package loadbot

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/umbracle/ethgo"
)

type ExecDuration struct {
	// turnAroundMap maps the transaction hash -> turn around time for passing transactions
	turnAroundMap     sync.Map
	turnAroundMapSize uint64

	// blockTransactions maps how many transactions went into a block
	blockTransactions map[uint64]uint64

	// Arrival Time - Time at which the transaction is added
	// Completion Time -Time at which the transaction is sealed
	// Turn around time - Completion Time – Arrival Time

	// AverageTurnAround is the average turn around time for all passing transactions
	AverageTurnAround time.Duration

	// FastestTurnAround is the fastest turn around time recorded for a transaction
	FastestTurnAround time.Duration

	// SlowestTurnAround is the slowest turn around time recorded for a transaction
	SlowestTurnAround time.Duration

	// TotalExecTime is the total execution time for a single loadbot run
	TotalExecTime time.Duration
}

// calcTurnAroundMetrics updates the turn around metrics based on the turnAroundMap
func (ed *ExecDuration) calcTurnAroundMetrics() {
	// Set the initial values
	fastestTurnAround := defaultFastestTurnAround
	slowestTurnAround := defaultSlowestTurnAround
	totalPassing := atomic.LoadUint64(&ed.turnAroundMapSize)

	var (
		zeroTime  time.Time // Zero time
		totalTime time.Time // Zero time used for tracking
	)

	if totalPassing == 0 {
		// No data to show, use zero data
		zeroDuration := time.Duration(0)
		ed.SlowestTurnAround = zeroDuration
		ed.FastestTurnAround = zeroDuration
		ed.AverageTurnAround = zeroDuration

		return
	}

	ed.turnAroundMap.Range(func(_, value interface{}) bool {
		data, ok := value.(*metadata)
		if !ok {
			return false
		}

		turnAroundTime := data.turnAroundTime

		// Update the duration metrics
		if turnAroundTime < fastestTurnAround {
			fastestTurnAround = turnAroundTime
		}

		if turnAroundTime > slowestTurnAround {
			slowestTurnAround = turnAroundTime
		}

		totalTime = totalTime.Add(turnAroundTime)

		ed.blockTransactions[data.blockNumber]++

		return true
	})

	averageDuration := (totalTime.Sub(zeroTime)) / time.Duration(totalPassing)

	ed.SlowestTurnAround = slowestTurnAround
	ed.FastestTurnAround = fastestTurnAround
	ed.AverageTurnAround = averageDuration
}

// reportTurnAroundTime reports the turn around time for a transaction
// for a single loadbot run
func (ed *ExecDuration) reportTurnAroundTime(
	txHash ethgo.Hash,
	data *metadata,
) {
	ed.turnAroundMap.Store(txHash, data)
	atomic.AddUint64(&ed.turnAroundMapSize, 1)
}
