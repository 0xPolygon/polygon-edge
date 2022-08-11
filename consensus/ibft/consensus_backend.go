package ibft

import (
	"fmt"
	"time"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/polygon-edge/consensus"
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

	snap := i.getSnapshot(latestBlockNumber)
	if snap == nil {
		i.logger.Error("cannot find snapshot", "num", latestBlockNumber)

		return nil
	}

	block, err := i.buildBlock(snap, latestHeader)
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

	seals := make([][]byte, len(committedSeals))
	for idx := range committedSeals {
		seals[idx] = committedSeals[idx].Signature
	}

	// Push the committed seals to the header
	header, err := writeCommittedSeals(newBlock.Header, seals)
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

	if err := i.runHook(InsertBlockHook, header.Number, header.Number); err != nil {
		i.logger.Error("cannot run hook", "name", string(InsertBlockHook), "err", err)

		return
	}

	i.updateMetrics(newBlock)

	i.logger.Info(
		"block committed",
		"number", newBlock.Number(),
		"hash", newBlock.Hash(),
		"validators", len(i.activeValidatorSet),
		"committed", len(committedSeals),
	)

	// after the block has been written we reset the txpool so that
	// the old transactions are removed
	i.txpool.ResetWithHeaders(newBlock.Header)
}

func (i *backendIBFT) ID() []byte {
	return i.validatorKeyAddr.Bytes()
}

func (i *backendIBFT) MaximumFaultyNodes() uint64 {
	return uint64(i.activeValidatorSet.MaxFaultyNodes())
}

func (i *backendIBFT) Quorum(blockNumber uint64) uint64 {
	var (
		validators = i.activeValidatorSet
		quorumFn   = i.quorumSize(blockNumber)
	)

	return uint64(quorumFn(validators))
}

// buildBlock builds the block, based on the passed in snapshot and parent header
func (i *backendIBFT) buildBlock(snap *Snapshot, parent *types.Header) (*types.Block, error) {
	header := &types.Header{
		ParentHash: parent.Hash,
		Number:     parent.Number + 1,
		Miner:      types.Address{},
		Nonce:      types.Nonce{},
		MixHash:    IstanbulDigest,
		// this is required because blockchain needs difficulty to organize blocks and forks
		Difficulty: parent.Number + 1,
		StateRoot:  types.EmptyRootHash, // this avoids needing state for now
		Sha3Uncles: types.EmptyUncleHash,
		GasLimit:   parent.GasLimit, // Inherit from parent for now, will need to adjust dynamically later.
	}

	// calculate gas limit based on parent header
	gasLimit, err := i.blockchain.CalculateGasLimit(header.Number)
	if err != nil {
		return nil, err
	}

	header.GasLimit = gasLimit

	if hookErr := i.runHook(CandidateVoteHook, header.Number, &candidateVoteHookParams{
		header: header,
		snap:   snap,
	}); hookErr != nil {
		i.logger.Error(fmt.Sprintf("Unable to run hook %s, %v", CandidateVoteHook, hookErr))
	}

	// set the timestamp
	header.Timestamp = uint64(time.Now().Unix())

	// we need to include in the extra field the current set of validators
	putIbftExtraValidators(header, snap.Set)

	transition, err := i.executor.BeginTxn(parent.StateRoot, header, i.validatorKeyAddr)
	if err != nil {
		return nil, err
	}
	// If the mechanism is PoS -> build a regular block if it's not an end-of-epoch block
	// If the mechanism is PoA -> always build a regular block, regardless of epoch

	txs := i.writeTransactions(gasLimit, header.Number, transition)

	if err := i.PreStateCommit(header, transition); err != nil {
		return nil, err
	}

	_, root := transition.Commit()
	header.StateRoot = root
	header.GasUsed = transition.TotalGas()

	// build the block
	block := consensus.BuildBlock(consensus.BuildBlockParams{
		Header:   header,
		Txns:     txs,
		Receipts: transition.Receipts(),
	})

	// write the seal of the block after all the fields are completed
	header, err = writeProposerSeal(i.validatorKey, block.Header)
	if err != nil {
		return nil, err
	}

	block.Header = header

	// compute the hash, this is only a provisional hash since the final one
	// is sealed after all the committed seals
	block.Header.ComputeHash()

	i.logger.Info("build block", "number", header.Number, "txs", len(txs))

	return block, nil
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

	if !i.shouldWriteTransactions(blockNumber) {
		return
	}

	var (
		blockTimer    = time.NewTimer(i.blockTime)
		stopExecution = false

		successful = 0
		failed     = 0
		skipped    = 0
	)

	defer func() {
		blockTimer.Stop()

		i.logger.Info(
			"executed txs",
			"successful", successful,
			"failed", failed,
			"skipped", skipped,
			"remaining", i.txpool.Length(),
		)
	}()

	i.txpool.Prepare()

	for {
		select {
		case <-blockTimer.C:
			return
		default:
			if stopExecution {
				// wait for the timer to expire
				continue
			}

			// execute transactions one by one
			result, ok := i.writeTransaction(
				i.txpool.Peek(),
				transition,
				gasLimit,
			)

			if !ok {
				stopExecution = true

				continue
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
