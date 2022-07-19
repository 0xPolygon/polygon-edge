package ibft

import (
	"errors"
	"github.com/0xPolygon/polygon-edge/types"
)

//	backend impl for go-ibft

func (i *backendIBFT) BuildProposal(blockNumber uint64) ([]byte, error) {
	var (
		latestHeader      = i.blockchain.Header()
		latestBlockNumber = latestHeader.Number
	)

	if latestBlockNumber+1 != blockNumber {
		return nil, errors.New("invalid block number given")
	}

	snap, err := i.getSnapshot(latestBlockNumber)
	if err != nil {
		i.logger.Error("cannot find snapshot", "num", latestBlockNumber)

		return nil, err
	}

	block, err := i.buildBlock(snap, latestHeader)
	if err != nil {
		return nil, err
	}

	return block.MarshalRLP(), nil
}

func (i *backendIBFT) InsertBlock(proposal []byte, committedSeals [][]byte) error {
	newBlock := &types.Block{}
	if err := newBlock.UnmarshalRLP(proposal); err != nil {
		return err
	}

	// Push the committed seals to the header
	header, err := writeCommittedSeals(newBlock.Header, committedSeals)
	if err != nil {
		return err
	}

	newBlock.Header = header

	// Save the block locally
	if err := i.blockchain.WriteBlock(newBlock, "consensus"); err != nil {
		return err
	}

	if err := i.runHook(InsertBlockHook, header.Number, header.Number); err != nil {
		return err
	}

	i.updateMetrics(newBlock)

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
