package ibft

import (
	"bytes"
	"encoding/hex"

	"github.com/0xPolygon/go-ibft/messages"
	protoIBFT "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"google.golang.org/protobuf/proto"
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

func (i *backendIBFT) IsValidSender(msg *protoIBFT.Message) bool {
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

func (i *backendIBFT) IsValidProposalHash(proposal *protoIBFT.ProposedBlock, hash []byte) bool {
	proposalHash, err := i.getProposalHashFromBlock(proposal.EthereumBlock, proposal.Round)
	if err != nil {
		return false
	}

	return bytes.Equal(proposalHash, hash)
}

func (i *backendIBFT) getProposalHashFromBlock(ethereumBlock []byte, round uint64) ([]byte, error) {
	block := &types.Block{}
	if err := block.UnmarshalRLP(ethereumBlock); err != nil {
		return nil, err
	}

	filteredHeader, err := i.currentSigner.FilterHeaderForHash(block.Header)
	if err != nil {
		return nil, err
	}

	block.Header = filteredHeader.ComputeHash()

	proposedBlockForHash := &protoIBFT.ProposedBlock{
		EthereumBlock: block.MarshalRLP(),
		Round:         round,
	}

	proposedBlockRaw, err := proto.Marshal(proposedBlockForHash)
	if err != nil {
		return nil, err
	}

	return crypto.Keccak256(proposedBlockRaw), nil
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
