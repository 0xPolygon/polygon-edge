package ibft

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/types"
)

//	Verifier impl for go-ibft

func (i *Ibft) IsValidBlock(proposal []byte) bool {
	var (
		latestHeader      = i.blockchain.Header()
		latestBlockNumber = latestHeader.Number
		newBlock          = &types.Block{}
	)

	// retrieve the newBlock proposal
	if err := newBlock.UnmarshalRLP(proposal); err != nil {
		i.logger.Error("failed to unmarshal newBlock", "err", err)

		return false
	}

	//	TODO: latestBlockNumber was i.state.Sequence
	if newBlock.Number() != latestBlockNumber+1 {
		i.logger.Error(
			"sequence not correct",
			"block", newBlock.Number,
			"sequence", latestBlockNumber+1,
		)

		return false
	}

	snap, err := i.getSnapshot(latestBlockNumber)
	if err != nil {
		i.logger.Error("snapshot not found", "err", err)

		return false
	}

	//	TODO: just verify the header, not the proposer (again)
	if err := i.verifyHeaderImpl(snap, latestHeader, newBlock.Header); err != nil {
		i.logger.Error("block header verification failed", "err", err)

		return false
	}

	if err := i.blockchain.VerifyPotentialBlock(newBlock); err != nil {
		i.logger.Error("newBlock verification failed", "err", err)
		i.handleStateErr(errBlockVerificationFailed)

		return false
	}

	if hookErr := i.runHook(VerifyBlockHook, newBlock.Number(), newBlock); hookErr != nil {
		if errors.As(hookErr, &errBlockVerificationFailed) {
			i.logger.Error("block verification failed, block at the end of epoch has transactions")
		} else {
			i.logger.Error(fmt.Sprintf("Unable to run hook %s, %v", VerifyBlockHook, hookErr))
		}

		return false
	}

	return true
}

//func (i *Ibft) IsValidSender(msg *proto.Message) bool {
//
//}

func (i *Ibft) IsProposer(id []byte, height, round uint64) bool {
	return false
}

func (i *Ibft) IsValidProposalHash(proposal, hash []byte) bool {
	return false
}

func (i *Ibft) IsValidCommittedSeal(proposal, seal []byte) bool {
	return false
}
