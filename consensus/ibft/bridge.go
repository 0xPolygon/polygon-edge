package ibft

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/bridge"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

// BridgeMechanism defines specific hooks for Bridge
type BridgeMechanism struct {
	BaseConsensusMechanism
	bridge bridge.Bridge
}

// BridgeFactory initializes Bridge Mechanism
func BridgeFactory(ibft *Ibft) (ConsensusMechanism, error) {
	bridge := &BridgeMechanism{
		BaseConsensusMechanism: BaseConsensusMechanism{
			mechanismType: Bridge,
			ibft:          ibft,
		},
		bridge: ibft.bridge,
	}

	bridge.initializeHookMap()

	return bridge, nil
}

// IsAvailable returns indicates if mechanism should be called at given height
func (b *BridgeMechanism) IsAvailable(_hookType HookType, _height uint64) bool {
	return b.bridge != nil
}

// updateValidatorsHook updates validator set in Bridge
func (b *BridgeMechanism) updateValidatorsHook(snapParam interface{}) error {
	// Cast the param to a *Snapshot
	snap, ok := snapParam.(*Snapshot)
	if !ok {
		return ErrInvalidHookParam
	}

	b.bridge.SetValidators(snap.Set, b.calculateSignatureThreshold(snap.Set))

	return nil
}

// verifyStateTransactionsHook verify state transactions in the block
func (b *BridgeMechanism) verifyStateTransactionsHook(blockParam interface{}) error {
	block, ok := blockParam.(*types.Block)
	if !ok {
		return ErrInvalidHookParam
	}

	for _, tx := range block.Transactions {
		if tx.Type != types.TxTypeState {
			continue
		}

		if err := b.bridge.StateSync().ValidateTx(tx); err != nil {
			b.ibft.logger.Error("block verification failed, block has invalid state transactions", "err", err)

			return errBlockVerificationFailed
		}
	}

	return nil
}

// insertTransactionHookParams are the params passed into the InsertTransactionsHook
type insertTransactionHookParams struct {
	header       *types.Header
	transition   *state.Transition
	transactions *[]*types.Transaction
}

// insertStateTransactionsHook applyes state transactions and adds to block
func (b *BridgeMechanism) insertStateTransactionsHook(rawParams interface{}) error {
	params, ok := rawParams.(*insertTransactionHookParams)
	if !ok {
		return ErrInvalidHookParam
	}

	msgs, err := b.bridge.StateSync().GetReadyMessages()
	if err != nil {
		b.ibft.logger.Warn("failed to get ready messages from bridge", "err", err)

		return nil
	}

	signatureThreshold := b.calculateSignatureThreshold(b.ibft.state.validators)
	for _, msg := range msgs {
		if uint64(len(msg.Signatures)) < signatureThreshold {
			b.ibft.logger.Warn(
				"message doesn't have enough signatures",
				"wanted", signatureThreshold,
				"actual", len(msg.Signatures),
			)

			continue
		}

		signer := crypto.NewSigner(
			b.ibft.config.Params.Forks.At(b.ibft.state.view.Sequence),
			uint64(b.ibft.config.Params.ChainID),
		)

		tx, err := signer.SignTx(msg.Transaction, b.ibft.validatorKey)
		if err != nil {
			return fmt.Errorf("failed to sign state transaction: %w", err)
		}

		if err := params.transition.Write(tx); err != nil {
			b.ibft.logger.Warn("failed to apply state transaction", "tx", tx, "err", err)

			continue
		}

		*params.transactions = append(*params.transactions, tx)
	}

	return nil
}

// insertBlockHook runs hook from syncState and insertBlock
func (b *BridgeMechanism) insertBlockHook(numberParam interface{}) error {
	block, ok := numberParam.(*types.Block)
	if !ok {
		return ErrInvalidHookParam
	}

	err := b.consumeStateTransactions(block)
	if err != nil {
		return err
	}

	err = b.startCheckpointProcess(block)
	if err != nil {
		return err
	}

	return nil
}

// startCheckpointProcess starts checkpoint process on every epoch end from insertBlock
func (b *BridgeMechanism) startCheckpointProcess(block *types.Block) error {
	// On every epoch end, create the checkpoint
	if b.ibft.state.getState() == SyncState && !b.ibft.IsLastOfEpoch(block.Number()) {
		return nil
	}

	return b.bridge.StartNewCheckpoint(b.ibft.epochSize)
}

// consumeStateTransactionsHook consumes all state transactions added in the block
func (b *BridgeMechanism) consumeStateTransactions(block *types.Block) error {
	for _, tx := range block.Transactions {
		if tx.Type != types.TxTypeState {
			continue
		}

		b.bridge.StateSync().Consume(tx)
	}

	return nil
}

func (b *BridgeMechanism) calculateSignatureThreshold(set ValidatorSet) uint64 {
	return uint64(b.ibft.state.NumValid())
}

// initializeHookMap registers the hooks that the Bridge mechanism
// should have
func (b *BridgeMechanism) initializeHookMap() {
	// Create the hook map
	b.hookMap = make(map[HookType]func(interface{}) error)

	// Register the AcceptStateLogHook
	b.hookMap[AcceptStateLogHook] = b.updateValidatorsHook

	// Register the VerifyBlockHook
	b.hookMap[VerifyBlockHook] = b.verifyStateTransactionsHook

	// Register the InsertTransactionsHook
	b.hookMap[InsertTransactionsHook] = b.insertStateTransactionsHook

	// Register the InsertBlockHook
	b.hookMap[InsertBlockHook] = b.insertBlockHook
}

// ShouldWriteTransactions indicates if transactions should be written to a block
// Bridge will not affect this flag for now
func (b *BridgeMechanism) ShouldWriteTransactions(_blockNumber uint64) bool {
	return false
}
