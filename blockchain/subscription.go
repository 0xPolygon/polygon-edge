package blockchain

import "github.com/0xPolygon/minimal/types"

// TODO:
// Include who generated this event?
// - Sync protocol
// - Sealer
// The type of event
// - Fork
// - Head

type Event struct {
	// Old chain removed if there was a reorg
	OldChain []*types.Header

	// New part of the chain
	NewChain []*types.Header
}

type Subscription struct {
}

func (s *Subscription) Close() {

}

func (s *Subscription) Watch() chan Event {
	return nil
}
