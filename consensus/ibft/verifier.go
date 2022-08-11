package ibft

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/0xPolygon/go-ibft/messages"
	protoIBFT "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/types"
)

// Verifier impl for go-ibft

func (i *backendIBFT) IsValidBlock(proposal []byte) bool {
	var (
		latestHeader      = i.blockchain.Header()
		latestBlockNumber = latestHeader.Number
		newBlock          = &types.Block{}
	)

	// retrieve the newBlock proposal
	if err := newBlock.UnmarshalRLP(proposal); err != nil {
		i.logger.Error("IsValidBlock: fail to unmarshal block", "err", err)

		return false
	}

	if latestBlockNumber+1 != newBlock.Number() {
		i.logger.Error(
			"sequence not correct",
			"block", newBlock.Number,
			"sequence", latestBlockNumber+1,
		)

		return false
	}

	snap := i.getSnapshot(latestBlockNumber)
	if snap == nil {
		i.logger.Error("snapshot not found", "num", latestBlockNumber)

		return false
	}

	if err := i.verifyHeaderImpl(snap, latestHeader, newBlock.Header); err != nil {
		i.logger.Error("block header verification failed", "err", err)

		return false
	}

	if err := i.blockchain.VerifyPotentialBlock(newBlock); err != nil {
		i.logger.Error("block verification failed", "err", err)

		return false
	}

	if err := i.runHook(VerifyBlockHook, newBlock.Number(), newBlock); err != nil {
		//nolint:govet
		if errors.As(err, &errBlockVerificationFailed) {
			i.logger.Error("block verification fail, block at the end of epoch has transactions")
		} else {
			i.logger.Error(fmt.Sprintf("Unable to run hook %s, %v", VerifyBlockHook, err))
		}

		return false
	}

	return true
}

func (i *backendIBFT) IsValidSender(msg *protoIBFT.Message) bool {
	msgNoSig, err := msg.PayloadNoSig()
	if err != nil {
		return false
	}

	validatorAddress, err := i.signer.Ecrecover(
		msg.Signature,
		msgNoSig,
	)
	if err != nil {
		return false
	}

	// verify the signature came from the sender
	if !bytes.Equal(msg.From, validatorAddress.Bytes()) {
		return false
	}

	// verify the sender is in the active validator set
	return i.activeValidatorSet.Includes(validatorAddress)
}

func (i *backendIBFT) IsProposer(id []byte, height, round uint64) bool {
	previousHeader, exists := i.blockchain.GetHeaderByNumber(height - 1)
	if !exists {
		i.logger.Error("header not found", "height", height-1)

		return false
	}

	nextProposer := CalcProposer(
		i.activeValidatorSet,
		round,
		i.extractProposer(previousHeader),
	)

	return bytes.Equal(nextProposer.Bytes(), id)
}

func (i *backendIBFT) IsValidProposalHash(proposal, hash []byte) bool {
	newBlock := &types.Block{}
	if err := newBlock.UnmarshalRLP(proposal); err != nil {
		i.logger.Error("unable to unmarshal proposal", "err", err)

		return false
	}

	blockHash := newBlock.Header.Hash.Bytes()

	return bytes.Equal(blockHash, hash)
}

func (i *backendIBFT) IsValidCommittedSeal(
	proposal []byte,
	committedSeal *messages.CommittedSeal,
) bool {
	err := i.signer.VerifyCommittedSeal(
		i.activeValidatorSet,
		types.BytesToAddress(committedSeal.Signer),
		committedSeal.Signature,
		proposal,
	)

	return err == nil
}

func (i *backendIBFT) extractProposer(header *types.Header) types.Address {
	if header.Number == 0 {
		return types.Address{}
	}

	proposer, _ := i.signer.EcrecoverFromHeader(header)

	return proposer
}
