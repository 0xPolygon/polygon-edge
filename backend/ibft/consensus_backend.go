package ibft

import (
	"errors"
	"github.com/0xPolygon/polygon-edge/types"
)

//	backend impl for go-ibft

func (i *backendIBFT) BuildProposal(blockNumber uint64) ([]byte, error) {
	i.logger.Debug("building proposal")
	defer i.logger.Debug("done building")

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
		panic("BuildProposal: cannot find snapshot")

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
		return nil, errors.New("build block fail")
	}

	return block.MarshalRLP(), nil
}

func (i *backendIBFT) InsertBlock(proposal []byte, committedSeals [][]byte) error {
	newBlock := &types.Block{}
	if err := newBlock.UnmarshalRLP(proposal); err != nil {
		panic("InsertBlock: cannot unmarshal block")
		return err
	}

	//	TODO: HEADER header mutated here
	// Push the committed seals to the header
	header, err := writeCommittedSeals(newBlock.Header, committedSeals)
	if err != nil {
		return err
	}

	// The hash needs to be recomputed since the extra data was changed
	newBlock.Header = header
	newBlock.Header.ComputeHash() // TODO: this is not needed

	// Save the block locally
	if err := i.blockchain.WriteBlock(newBlock, "consensus"); err != nil {
		return err
	}

	if err := i.runHook(InsertBlockHook, header.Number, header.Number); err != nil {
		return err
	}

	i.updateMetrics(newBlock)

	//	TODO: move log to go-ibft
	i.logger.Info(
		"block committed",
		"number", newBlock.Number(),
		"hash", newBlock.Hash(),
		"validators", len(i.currentValidatorSet),
		"committed", len(committedSeals),
	)

	// after the block has been written we reset the txpool so that
	// the old transactions are removed
	i.txpool.ResetWithHeaders(newBlock.Header)

	return nil
}

func (i *backendIBFT) ID() []byte {
	return i.validatorKeyAddr.Bytes()
}

func (i *backendIBFT) MaximumFaultyNodes() uint64 {
	return uint64(i.currentValidatorSet.MaxFaultyNodes())
}

func (i *backendIBFT) Quorum(blockNumber uint64) uint64 {
	var (
		validators = i.currentValidatorSet
		quorumFn   = i.quorumSize(blockNumber)
	)

	return uint64(quorumFn(validators))
}
