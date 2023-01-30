package ibft

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"

	"github.com/0xPolygon/go-ibft/messages"
	protoIBFT "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

// Verifier impl for go-ibft
// calculateProposalHashFromBlockBytes is a helper method to marshal ethereum block in bytes
// and pass to calculateProposalHash
func (i *backendIBFT) calculateProposalHashFromBlockBytes(
	proposal []byte,
	round *uint64,
) (types.Hash, error) {
	block := &types.Block{}
	if err := block.UnmarshalRLP(proposal); err != nil {
		return types.ZeroHash, err
	}

	signer, err := i.forkManager.GetSigner(block.Number())
	if err != nil {
		return types.ZeroHash, err
	}

	return i.calculateProposalHash(
		signer,
		block.Header,
		round,
	)
}

// calculateProposalHash is new hash calculation for proposal in go-ibft,
// which includes round number block is finalized at
func (i *backendIBFT) calculateProposalHash(
	signer signer.Signer,
	header *types.Header,
	round *uint64,
) (types.Hash, error) {
	if round == nil {
		// legacy hash calculation
		return header.Hash, nil
	}

	roundBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(roundBytes, *round)

	return types.BytesToHash(
		crypto.Keccak256(
			header.Hash.Bytes(),
			roundBytes,
		),
	), nil
}

func (i *backendIBFT) IsValidProposal(rawProposal []byte) bool {
	var (
		latestHeader      = i.blockchain.Header()
		latestBlockNumber = latestHeader.Number
		newBlock          = &types.Block{}
	)

	// retrieve the newBlock proposal
	if err := newBlock.UnmarshalRLP(rawProposal); err != nil {
		i.logger.Error("IsValidProposal: fail to unmarshal block", "err", err)

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

	if err := i.verifyHeaderImpl(
		latestHeader,
		newBlock.Header,
		i.currentSigner,
		i.currentValidators,
		i.currentHooks,
		true,
	); err != nil {
		i.logger.Error("block header verification failed", "err", err)

		return false
	}

	if err := i.blockchain.VerifyPotentialBlock(newBlock); err != nil {
		i.logger.Error("block verification failed", "err", err)

		return false
	}

	if err := i.currentHooks.VerifyBlock(newBlock); err != nil {
		i.logger.Error("additional block verification failed", "err", err)

		return false
	}

	return true
}

func (i *backendIBFT) IsValidValidator(msg *protoIBFT.Message) bool {
	msgNoSig, err := msg.PayloadNoSig()
	if err != nil {
		return false
	}

	signerAddress, err := i.currentSigner.EcrecoverFromIBFTMessage(
		msg.Signature,
		msgNoSig,
	)

	if err != nil {
		i.logger.Error("failed to ecrecover message", "err", err)

		return false
	}

	// verify the signature came from the sender
	if !bytes.Equal(msg.From, signerAddress.Bytes()) {
		i.logger.Error(
			"signer address doesn't match with From",
			"from", hex.EncodeToString(msg.From),
			"signer", signerAddress,
			"err", err,
		)

		return false
	}

	validators, err := i.forkManager.GetValidators(msg.View.Height)
	if err != nil {
		return false
	}

	// verify the sender is in the active validator set
	if !validators.Includes(signerAddress) {
		i.logger.Error(
			"signer address doesn't included in validators",
			"signer", signerAddress,
		)

		return false
	}

	return true
}

func (i *backendIBFT) IsProposer(id []byte, height, round uint64) bool {
	previousHeader, exists := i.blockchain.GetHeaderByNumber(height - 1)
	if !exists {
		i.logger.Error("header not found", "height", height-1)

		return false
	}

	previousProposer, err := i.extractProposer(previousHeader)
	if err != nil {
		i.logger.Error("failed to extract the last proposer", "height", height-1, "err", err)

		return false
	}

	nextProposer := CalcProposer(
		i.currentValidators,
		round,
		previousProposer,
	)

	return types.BytesToAddress(id) == nextProposer.Addr()
}

func (i *backendIBFT) IsValidProposalHash(proposal *protoIBFT.Proposal, hash []byte) bool {
	proposalHash, err := i.calculateProposalHashFromBlockBytes(proposal.RawProposal, &proposal.Round)
	if err != nil {
		return false
	}

	return bytes.Equal(proposalHash.Bytes(), hash)
}

func (i *backendIBFT) IsValidCommittedSeal(
	proposalHash []byte,
	committedSeal *messages.CommittedSeal,
) bool {
	err := i.currentSigner.VerifyCommittedSeal(
		i.currentValidators,
		types.BytesToAddress(committedSeal.Signer),
		committedSeal.Signature,
		proposalHash,
	)

	if err != nil {
		i.logger.Error("IsValidCommittedSeal: failed to verify committed seal", "err", err)

		return false
	}

	return true
}

func (i *backendIBFT) extractProposer(header *types.Header) (types.Address, error) {
	if header.Number == 0 {
		return types.ZeroAddress, nil
	}

	signer, err := i.forkManager.GetSigner(header.Number)
	if err != nil {
		return types.ZeroAddress, err
	}

	proposer, err := signer.EcrecoverFromHeader(header)
	if err != nil {
		return types.ZeroAddress, err
	}

	return proposer, nil
}
