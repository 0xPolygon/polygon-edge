package ibft

import (
	"math"
	"time"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/polygon-edge/blockbuilder"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/types"
)

func (i *backendIBFT) BuildProposal(blockNumber uint64) []byte {
	var (
		latestHeader      = i.blockchain.Header()
		latestBlockNumber = latestHeader.Number
	)

	if latestBlockNumber+1 != blockNumber {
		i.logger.Error(
			"unable to build block, due to lack of parent block",
			"num",
			latestBlockNumber,
		)

		return nil
	}

	block, err := i.buildBlock(latestHeader)
	if err != nil {
		i.logger.Error("cannot build block", "num", blockNumber, "err", err)

		return nil
	}

	return block.MarshalRLP()
}

func (i *backendIBFT) InsertBlock(
	proposal []byte,
	committedSeals []*messages.CommittedSeal,
) {
	newBlock := &types.Block{}
	if err := newBlock.UnmarshalRLP(proposal); err != nil {
		i.logger.Error("cannot unmarshal proposal", "err", err)

		return
	}

	committedSealsMap := make(map[types.Address][]byte, len(committedSeals))

	for _, cm := range committedSeals {
		committedSealsMap[types.BytesToAddress(cm.Signer)] = cm.Signature
	}

	// Push the committed seals to the header
	header, err := i.currentSigner.WriteCommittedSeals(newBlock.Header, committedSealsMap)
	if err != nil {
		i.logger.Error("cannot write committed seals", "err", err)

		return
	}

	newBlock.Header = header

	// Save the block locally
	if err := i.blockchain.WriteBlock(newBlock, "consensus"); err != nil {
		i.logger.Error("cannot write block", "err", err)

		return
	}

	i.updateMetrics(newBlock)

	i.logger.Info(
		"block committed",
		"number", newBlock.Number(),
		"hash", newBlock.Hash(),
		"validation_type", i.currentSigner.Type(),
		"validators", i.currentValidators.Len(),
		"committed", len(committedSeals),
	)

	if err := i.currentHooks.PostInsertBlock(newBlock); err != nil {
		i.logger.Error(
			"failed to call PostInsertBlock hook",
			"height", newBlock.Number(),
			"hash", newBlock.Hash(),
			"err", err,
		)

		return
	}

	// after the block has been written we reset the txpool so that
	// the old transactions are removed
	i.txpool.ResetWithHeaders(newBlock.Header)
}

func (i *backendIBFT) ID() []byte {
	return i.currentSigner.Address().Bytes()
}

func (i *backendIBFT) MaximumFaultyNodes() uint64 {
	return uint64(CalcMaxFaultyNodes(i.currentValidators))
}

func (i *backendIBFT) Quorum(blockNumber uint64) uint64 {
	validators, err := i.forkManager.GetValidators(blockNumber)
	if err != nil {
		i.logger.Error(
			"failed to get validators when calculation quorum",
			"height", blockNumber,
			"err", err,
		)

		// return Math.MaxInt32 to prevent overflow when casting to int in go-ibft package
		return math.MaxInt32
	}

	quorumFn := i.quorumSize(blockNumber)

	return uint64(quorumFn(validators))
}

// buildBlock builds the block, based on the passed in snapshot and parent header
func (i *backendIBFT) buildBlock(parent *types.Header) (*types.Block, error) {
	params := &blockbuilder.BlockBuilderParams{
		Parent:         parent,
		Executor:       i.executor,
		Coinbase:       i.currentSigner.Address(),
		BlockGasTarget: i.blockchain.Config().BlockGasTarget,
		Logger:         i.logger,
		TxPool:         i.txpool,
	}
	bbuilder, err := blockbuilder.NewBlockBuilder(params)
	if err != nil {
		return nil, err
	}

	parentCommittedSeals, err := i.extractParentCommittedSeals(parent)
	if err != nil {
		return nil, err
	}

	// fill with transactions
	bbuilder.Fill()

	stateBlock := bbuilder.Build(func(header *types.Header) {
		if err := i.currentHooks.ModifyHeader(header, i.currentSigner.Address()); err != nil {
			panic(err)
		}

		// set the timestamp
		header.Timestamp = uint64(time.Now().Unix())

		i.currentSigner.InitIBFTExtra(header, i.currentValidators, parentCommittedSeals)

		header, err = i.currentSigner.WriteProposerSeal(header)
		if err != nil {
			panic(err)
		}
	})

	i.logger.Info("build block", "number", stateBlock.Block.Header.Number, "txs", len(stateBlock.Block.Transactions))

	return stateBlock.Block, nil
}

// extractCommittedSeals extracts CommittedSeals from header
func (i *backendIBFT) extractCommittedSeals(
	header *types.Header,
) (signer.Seals, error) {
	signer, err := i.forkManager.GetSigner(header.Number)
	if err != nil {
		return nil, err
	}

	extra, err := signer.GetIBFTExtra(header)
	if err != nil {
		return nil, err
	}

	return extra.CommittedSeals, nil
}

// extractParentCommittedSeals extracts ParentCommittedSeals from header
func (i *backendIBFT) extractParentCommittedSeals(
	header *types.Header,
) (signer.Seals, error) {
	if header.Number == 0 {
		return nil, nil
	}

	return i.extractCommittedSeals(header)
}
