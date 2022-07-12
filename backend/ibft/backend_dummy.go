package ibft

import "errors"

//	backend impl for go-ibft

func (i *Ibft) BuildProposal(blockNumber uint64) ([]byte, error) {
	var (
		latestHeader      = i.blockchain.Header()
		latestBlockNumber = latestHeader.Number
	)

	if blockNumber != latestBlockNumber+1 {
		return nil, errors.New("invalid block number given")
	}

	snap, err := i.getSnapshot(latestBlockNumber)
	if err != nil {
		i.logger.Error("cannot find snapshot", "num", latestBlockNumber)

		return nil, errors.New("snapshot not found")
	}

	//	TODO: remove this check since this method
	//		cannot be invoked from a non-proposer (validator)
	//		that is not participating in consensus
	//if !snap.Set.Includes(i.validatorKeyAddr) {
	//	// we are not a validator anymore, move back to sync state
	//	i.logger.Info("we are not a validator anymore")
	//	i.setState(SyncState)
	//
	//	return
	//}

	block, err := i.buildBlock(snap, latestHeader)
	if err != nil {
		return nil, errors.New("build block failed")
	}

	return block.MarshalRLP(), nil
}

func (i *Ibft) InsertBlock(proposal []byte, committedSeals [][]byte) error {
	return nil
}

func (i *Ibft) ID() []byte {
	return nil
}

func (i *Ibft) MaximumFaultyNodes() uint64 {
	return 0
}

func (i *Ibft) Quorum(blockNumber uint64) uint64 {
	return 0
}
