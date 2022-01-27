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
	// create logger

	// required confirmations

	// create db for last processed block num
	// or instantiate from existing file

	// store hardcoded abi events

	return nil, nil
}

func (t *tracker) Start() <-chan []byte {
	//	connect to the rootchain

	//	create cancellable contexts

	//	start the header process first

	//	start rootchain subscription

	//	return the channel where matched events are sent to
}

func (t *tracker) Stop() {
	//	stop the subscription

	//	stop the header process

}

func (t *tracker) processHeader(header *types.Header) {
	//	determine range of blocks to query
	//	(and update db if necessary)

	//	construct log filter from given abis
	//	and issue eth_getLogs request to the rootchain

	//	for each event matches against an abi
	//	send the log (bytes) to the tracker's channel
}

func (t *tracker) subscribe() {

}
