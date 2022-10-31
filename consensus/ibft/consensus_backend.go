package ibft

import (
	"fmt"
	"math"
	"time"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/polygon-edge/blockbuilder"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/state"
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
	bb := blockbuilder.BlockBuilder{}

	parentCommittedSeals, err := i.extractParentCommittedSeals(parent)
	if err != nil {
		return nil, err
	}

	// fill with transactions
	bb.Fill()

	stateBlock := bb.Build(func(header *types.Header) {
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

type status uint8

const (
	success status = iota
	fail
	skip
)

type txExeResult struct {
	tx     *types.Transaction
	status status
}

type transitionInterface interface {
	Write(txn *types.Transaction) error
	WriteFailedReceipt(txn *types.Transaction) error
}

func (i *backendIBFT) writeTransactions(
	gasLimit,
	blockNumber uint64,
	transition transitionInterface,
) (executed []*types.Transaction) {
	executed = make([]*types.Transaction, 0)

	if !i.currentHooks.ShouldWriteTransactions(blockNumber) {
		return
	}

	var (
		blockTimer = time.NewTimer(i.blockTime)

		successful = 0
		failed     = 0
		skipped    = 0
	)

	defer func() {
		i.logger.Info(
			"executed txs",
			"successful", successful,
			"failed", failed,
			"skipped", skipped,
			"remaining", i.txpool.Length(),
		)
	}()

	i.txpool.Prepare()

write:
	for {
		select {
		case <-blockTimer.C:
			return
		default:
			// execute transactions one by one
			result, ok := i.writeTransaction(
				i.txpool.Peek(),
				transition,
				gasLimit,
			)

			if !ok {
				break write
			}

			tx := result.tx

			switch result.status {
			case success:
				executed = append(executed, tx)
				successful++
			case fail:
				failed++
			case skip:
				skipped++
			}
		}
	}

	//	wait for the timer to expire
	<-blockTimer.C

	return
}

func (i *backendIBFT) writeTransaction(
	tx *types.Transaction,
	transition transitionInterface,
	gasLimit uint64,
) (*txExeResult, bool) {
	if tx == nil {
		return nil, false
	}

	if tx.ExceedsBlockGasLimit(gasLimit) {
		i.txpool.Drop(tx)

		if err := transition.WriteFailedReceipt(tx); err != nil {
			i.logger.Error(
				fmt.Sprintf(
					"unable to write failed receipt for transaction %s",
					tx.Hash,
				),
			)
		}

		// continue processing
		return &txExeResult{tx, fail}, true
	}

	if err := transition.Write(tx); err != nil {
		if _, ok := err.(*state.GasLimitReachedTransitionApplicationError); ok { //nolint:errorlint
			// stop processing
			return nil, false
		} else if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable { //nolint:errorlint
			i.txpool.Demote(tx)

			return &txExeResult{tx, skip}, true
		} else {
			i.txpool.Drop(tx)

			return &txExeResult{tx, fail}, true
		}
	}

	i.txpool.Pop(tx)

	return &txExeResult{tx, success}, true
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
