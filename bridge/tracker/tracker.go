package tracker

import "github.com/0xPolygon/polygon-edge/types"

type tracker struct {
	// logger

	// header channel

	// cancel funcs

	// requiredConfirmations

	// db storage for last processed block num

	// subscribe channel

	// ethclient - hardcoded

	// ABI events - hardcoded (StateSender.sol, remix for POC)
}

func newTracker(requiredConfirmations uint) (*tracker, error) {

	return nil, nil
}

func (t *tracker) Start() <-chan []byte {

}

func (t *tracker) Stop() {

}

func (t *tracker) processHeader(header *types.Header) {

}

func (t *tracker) subscribe() {

}
