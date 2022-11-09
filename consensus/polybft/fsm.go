package polybft

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	hcf "github.com/hashicorp/go-hclog"
)

type blockBuilder interface {
	Reset() error
	WriteTx(*types.Transaction) error
	Fill() error
	Build(func(h *types.Header)) (*StateBlock, error)
	GetState() *state.Transition
}

const maxBundlesPerSprint = 50

type fsm struct {
	// PolyBFT consensus protocol configuration
	config *PolyBFTConfig

	// parent block header
	parent *types.Header

	// backend implements methods for retrieving data from block chain
	backend blockchainBackend

	// polybftBackend implements methods needed from the polybft
	polybftBackend polybftBackend

	// validators is the list of validators for this round
	validators ValidatorSet

	// blockBuilder is the block builder for proposers
	blockBuilder blockBuilder

	// uptimeCounter holds info about number of times validators sealed a block (only present if isEndOfEpoch is true)
	uptimeCounter *CommitEpoch

	// isEndOfEpoch indicates if epoch reached its end
	isEndOfEpoch bool

	// isEndOfSprint indicates if sprint reached its end
	isEndOfSprint bool

	// proposerCommitmentToRegister is a commitment that is registered via state transaction by proposer
	proposerCommitmentToRegister *CommitmentMessageSigned

	// commitmentToSaveOnRegister is a commitment that is verified on register and needs to be saved in db
	commitmentToSaveOnRegister *CommitmentMessageSigned

	// bundleProofs is an array of bundles to be executed on end of sprint
	bundleProofs []*BundleProof

	// commitmentsToVerifyBundles is an array of commitment messages that were not executed yet,
	// but are used to verify any bundles if they are included in state transactions
	commitmentsToVerifyBundles []*CommitmentMessageSigned

	// stateSyncExecutionIndex is the next state sync execution index in smart contract
	stateSyncExecutionIndex uint64

	logger hcf.Logger // The logger object
}

// BuildProposal builds a proposal for the current round (used if proposer)
func (f *fsm) BuildProposal() ([]byte, error) {
	parent := f.parent

	extraParent, err := GetIbftExtra(parent.ExtraData)
	if err != nil {
		return nil, err
	}

	// TODO: we will need to revisit once slashing is implemented
	extra := &Extra{Parent: extraParent.Committed}

	if f.isEndOfEpoch {
		tx, err := f.createValidatorsUptimeTx()
		if err != nil {
			return nil, err
		}

		if err := f.blockBuilder.WriteTx(tx); err != nil {
			return nil, fmt.Errorf("failed to commit validators uptime transaction: %w", err)
		}

		validatorsDelta, err := f.getValidatorSetDelta(f.blockBuilder.GetState())
		if err != nil {
			return nil, err
		}

		extra.Validators = validatorsDelta
		f.logger.Trace("[FSM Build Proposal]", "Validators Delta", validatorsDelta)
	}

	if f.config.IsBridgeEnabled() {
		if f.isEndOfEpoch && f.proposerCommitmentToRegister != nil {
			// since proposer does not execute Validate (when we see the commitment to register in state transactions)
			// we need to set commitment to save so that the proposer also saves its commitment that he registered
			f.commitmentToSaveOnRegister = f.proposerCommitmentToRegister
			f.logger.Debug("[FSM] Registering commitment",
				"from", f.proposerCommitmentToRegister.Message.FromIndex,
				"toIndex", f.proposerCommitmentToRegister.Message.ToIndex)
		}

		for _, tx := range f.stateTransactions() {
			if err := f.blockBuilder.WriteTx(tx); err != nil {
				return nil, fmt.Errorf("failed to commit state transaction. Error: %w", err)
			}
		}
	}

	// fill the block with transactions
	if err := f.blockBuilder.Fill(); err != nil {
		return nil, err
	}

	// set the timestamp
	parentTime := time.Unix(int64(parent.Timestamp), 0)
	headerTime := parentTime.Add(f.config.BlockTime)

	if headerTime.Before(time.Now()) {
		headerTime = time.Now()
	}

	stateBlock, err := f.blockBuilder.Build(func(h *types.Header) {
		h.Timestamp = uint64(headerTime.Unix())
		h.ExtraData = append(make([]byte, signer.IstanbulExtraVanity), extra.MarshalRLPTo(nil)...)
		h.MixHash = PolyMixDigest
	})

	if err != nil {
		return nil, err
	}

	f.logger.Debug("[FSM Build Proposal]",
		"txs", len(stateBlock.Block.Transactions),
		"hash", stateBlock.Block.Hash().String())

	return stateBlock.Block.MarshalRLP(), nil
}

func (f *fsm) stateTransactions() []*types.Transaction {
	var txns []*types.Transaction

	if f.isEndOfEpoch {
		if f.proposerCommitmentToRegister != nil {
			// add register commitment transaction
			inputData, err := f.proposerCommitmentToRegister.EncodeAbi()
			if err != nil {
				f.logger.Error("StateTransactions failed to encode input data for state sync commitment registration", "Error", err)

				return nil
			}

			txns = append(txns,
				createStateTransactionWithData(f.config.StateReceiverAddr, inputData))
		}
	}

	if f.isEndOfSprint {
		for _, bundle := range f.bundleProofs {
			inputData, err := bundle.EncodeAbi()
			if err != nil {
				f.logger.Error("stateTransactions failed to encode input data for state sync execution", "Error", err)

				return nil
			}

			txns = append(txns,
				createStateTransactionWithData(f.config.StateReceiverAddr, inputData))

			f.logger.Debug("[fsm] Executing bundle",
				"fromIndex", bundle.StateSyncs[0].ID,
				"toIndex", bundle.StateSyncs[len(bundle.StateSyncs)-1].ID)
		}
	}

	f.logger.Debug("Apply state transaction", "num", len(txns))

	return txns
}

// createValidatorsUptimeTx create a StateTransaction, which invokes ValidatorSet smart contract
// and sends all the necessary metadata to it.
func (f *fsm) createValidatorsUptimeTx() (*types.Transaction, error) {
	input, err := f.uptimeCounter.EncodeAbi()
	if err != nil {
		return nil, err
	}

	return createStateTransactionWithData(f.config.ValidatorSetAddr, input), nil
}

// ValidateCommit is used to validate that a given commit is valid
func (f *fsm) ValidateCommit(signer []byte, seal []byte, proposalHash []byte) error {
	from := types.BytesToAddress(signer)

	validator := f.validators.Accounts().GetValidatorMetadata(from)
	if validator == nil {
		return fmt.Errorf("unable to resolve validator %s", from)
	}

	signature, err := bls.UnmarshalSignature(seal)
	if err != nil {
		return fmt.Errorf("failed to unmarshall signature: %w", err)
	}

	if !signature.Verify(validator.BlsKey, proposalHash) {
		return fmt.Errorf("incorrect commit signature from %s", from)
	}

	return nil
}

// Validate validates a raw proposal (used if non-proposer)
func (f *fsm) Validate(proposal []byte) error {
	var block types.Block
	if err := block.UnmarshalRLP(proposal); err != nil {
		return fmt.Errorf("failed to validate, cannot decode block data. Error: %w", err)
	}

	f.logger.Debug("[FSM Validate]", "hash", block.Hash().String())

	// validate header fields
	if err := validateHeaderFields(f.parent, block.Header); err != nil {
		return fmt.Errorf(
			"failed to validate header (parent header# %d, current header#%d): %w",
			f.parent.Number,
			block.Number(),
			err,
		)
	}

	blockExtra, err := GetIbftExtra(block.Header.ExtraData)
	if err != nil {
		return fmt.Errorf("cannot get extra data:%w", err)
	}

	// TODO: Validate validator set delta?

	blockNumber := block.Number()
	if blockNumber > 1 {
		// verify parent signature
		// We skip block 0 (genesis) and block 1 (parent is genesis)
		// since those blocks do not include any parent information with signatures
		validators, err := f.polybftBackend.GetValidators(blockNumber-2, nil)
		if err != nil {
			return fmt.Errorf("cannot get validators:%w", err)
		}

		f.logger.Trace("[FSM Validate]", "Block", blockNumber, "parent validators", validators)

		if err := blockExtra.Parent.VerifyCommittedFields(validators, f.parent.Hash); err != nil {
			return fmt.Errorf(
				"failed to verify signatures for (parent) block#%d. Block hash: %v, block#%d. Error: %w",
				f.parent.Number,
				f.parent.Hash,
				blockNumber,
				err,
			)
		}
	}

	if err := f.VerifyStateTransactions(block.Transactions); err != nil {
		return err
	}

	if _, err = f.backend.ProcessBlock(f.parent, &block); err != nil {
		return err
	}

	f.logger.Debug("[FSM Validate]", "txs", len(block.Transactions), "hash", block.Hash().String())

	return nil
}

// Validate sender address and signature
func (f *fsm) ValidateSender(msg *proto.Message) error {
	msgNoSig, err := msg.PayloadNoSig()
	if err != nil {
		return err
	}

	signerAddress, err := wallet.RecoverAddressFromSignature(msg.Signature, msgNoSig)
	if err != nil {
		return fmt.Errorf("failed to ecrecover message: %w", err)
	}

	// verify the signature came from the sender
	if !bytes.Equal(msg.From, signerAddress.Bytes()) {
		return fmt.Errorf("signer address %s doesn't match From field", signerAddress.String())
	}

	// verify the sender is in the active validator set
	if !f.validators.Includes(signerAddress) {
		return fmt.Errorf("signer address %s is not included in validator set", signerAddress.String())
	}

	return nil
}

func (f *fsm) VerifyStateTransactions(transactions []*types.Transaction) error {
	if f.isEndOfEpoch {
		err := f.verifyValidatorsUptimeTx(transactions)
		if err != nil {
			return err
		}

		if len(transactions) > 0 {
			transactions = transactions[1:]
		}
	}

	commitmentMessageSignedExists := false
	nextStateSyncBundleIndex := f.stateSyncExecutionIndex

	for _, tx := range transactions {
		if tx.Type != types.StateTx {
			continue
		}

		if !f.isEndOfSprint {
			return fmt.Errorf("state transaction in block which should not contain it: tx = %v", tx.Hash)
		}

		decodedStateTx, err := decodeStateTransaction(tx.Input) // used to be Data
		if err != nil {
			return fmt.Errorf("state transaction error while decoding: tx = %v, err = %w", tx.Hash, err)
		}

		switch stateTxData := decodedStateTx.(type) {
		case *CommitmentMessageSigned:
			if commitmentMessageSignedExists {
				return fmt.Errorf("only one commitment is allowed per block: %v", tx.Hash)
			}

			commitmentMessageSignedExists = true
			signers, err := f.validators.Accounts().GetFilteredValidators(stateTxData.AggSignature.Bitmap)

			if err != nil {
				return fmt.Errorf("error for state transaction while retrieving signers: tx = %v, error = %w", tx.Hash, err)
			}

			if len(signers) < getQuorumSize(f.validators.Len()) {
				return fmt.Errorf("quorum size not reached for state tx: %v", tx.Hash)
			}

			aggs, err := bls.UnmarshalSignature(stateTxData.AggSignature.AggregatedSignature)
			if err != nil {
				return fmt.Errorf("error for state transaction while unmarshaling signature: tx = %v, error = %w", tx.Hash, err)
			}

			hash, err := stateTxData.Message.Hash()
			if err != nil {
				return err
			}

			verified := aggs.VerifyAggregated(signers.GetBlsKeys(), hash.Bytes())
			if !verified {
				return fmt.Errorf("invalid signature for tx = %v", tx.Hash)
			}

			f.commitmentToSaveOnRegister = stateTxData
		case *BundleProof:
			// every other bundle has to be in sequential order
			if stateTxData.ID() != nextStateSyncBundleIndex {
				return fmt.Errorf("bundles to execute are not in sequential order "+
					"according to state execution index from smart contract: %v", f.stateSyncExecutionIndex)
			}

			nextStateSyncBundleIndex = stateTxData.StateSyncs[len(stateTxData.StateSyncs)-1].ID + 1

			isVerified := false

			for _, commitment := range f.commitmentsToVerifyBundles {
				if commitment.Message.ContainsStateSync(stateTxData.ID()) {
					isVerified = true

					if err := commitment.Message.VerifyProof(stateTxData); err != nil {
						return fmt.Errorf("state transaction error while validating proof: tx = %v, err = %w",
							tx.Hash, err)
					}

					break
				}
			}

			if !isVerified {
				return fmt.Errorf("state transaction error while validating proof. "+
					"No appropriate commitment found to verify proof. tx = %v", tx.Hash)
			}
		}
	}

	return nil
}

// Insert inserts the sealed proposal
func (f *fsm) Insert(proposal []byte, committedSeals []*messages.CommittedSeal) (*types.Block, error) {
	newBlock := &types.Block{}
	if err := newBlock.UnmarshalRLP(proposal); err != nil {
		return nil, fmt.Errorf("cannot unmarshal proposal: %w", err)
	}

	// In this function we should try to return little to no errors since
	// at this point everything we have to do is just commit something that
	// we should have already computed beforehand.
	extra, _ := GetIbftExtra(newBlock.Header.ExtraData)

	// create map for faster access to indexes
	nodeIDIndexMap := make(map[types.Address]int, f.validators.Len())
	for i, addr := range f.validators.Accounts().GetAddresses() {
		nodeIDIndexMap[addr] = i
	}

	// populated bitmap according to nodeId from validator set and committed seals
	// also populate slice of signatures
	bitmap := bitmap.Bitmap{}
	signatures := make(bls.Signatures, 0, len(committedSeals))

	for _, commSeal := range committedSeals {
		signerAddr := types.BytesToAddress(commSeal.Signer)

		index, exists := nodeIDIndexMap[signerAddr]
		if !exists {
			return nil, fmt.Errorf("invalid node id = %s", signerAddr.String())
		}

		s, err := bls.UnmarshalSignature(commSeal.Signature)
		if err != nil {
			return nil, fmt.Errorf("invalid signature = %s", commSeal.Signature)
		}

		signatures = append(signatures, s)

		bitmap.Set(uint64(index))
	}

	aggregatedSignature, err := signatures.Aggregate().Marshal()
	if err != nil {
		return nil, fmt.Errorf("could not aggregate seals: %w", err)
	}

	// include aggregated signature of all committed seals
	// also includes bitmap which contains all indexes from validator set which provides there seals
	extra.Committed = &Signature{
		AggregatedSignature: aggregatedSignature,
		Bitmap:              bitmap,
	}

	// Write extar data to header
	newBlock.Header.ExtraData = append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...)

	if err = f.backend.CommitBlock(newBlock); err != nil {
		return nil, err
	}

	return newBlock, nil
}

// Height returns the height for the current round
func (f *fsm) Height() uint64 {
	return f.parent.Number + 1
}

// ValidatorSet returns the validator set for the current round
func (f *fsm) ValidatorSet() ValidatorSet {
	return f.validators
}

// getValidatorSetDelta calculates validator set delta based on parent and current header
func (f *fsm) getValidatorSetDelta(pendingBlockState *state.Transition) (*ValidatorSetDelta, error) {
	provider := f.backend.GetStateProvider(pendingBlockState)
	systemState := f.backend.GetSystemState(f.config, provider)
	newValidators, err := systemState.GetValidatorSet()

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve validator set for current block: %w", err)
	}

	return createValidatorSetDelta(f.logger, f.validators.Accounts(), newValidators)
}

// verifyValidatorsUptimeTx creates uptime transaction and compares its hash with the one extracted from the block.
func (f *fsm) verifyValidatorsUptimeTx(transactions []*types.Transaction) error {
	var blockUptimeTx *types.Transaction
	if len(transactions) > 0 {
		blockUptimeTx = transactions[0]
	}

	createdUptimeTx, err := f.createValidatorsUptimeTx()
	if err != nil {
		return err
	}

	if f.isEndOfEpoch {
		if blockUptimeTx == nil {
			return errors.New("uptime transaction is not found in the epoch ending block")
		}

		if blockUptimeTx.Hash != createdUptimeTx.Hash {
			return fmt.Errorf(
				"invalid uptime transaction. Expected '%s', but got '%s' uptime transaction hash",
				blockUptimeTx.Hash,
				createdUptimeTx.Hash,
			)
		}
	} else {
		if blockUptimeTx != nil && blockUptimeTx.Hash == createdUptimeTx.Hash {
			return errors.New("didn't expect uptime transaction in the middle of an epoch")
		}
	}

	return nil
}

func validateHeaderFields(parent *types.Header, header *types.Header) error {
	// verify parent hash
	if parent.Hash != header.ParentHash {
		return fmt.Errorf("incorrect header parent hash (parent=%s, header parent=%s)", parent.Hash, header.ParentHash)
	}
	// verify parent number
	if header.Number != parent.Number+1 {
		return fmt.Errorf("invalid number")
	}
	// verify time has passed
	if header.Timestamp <= parent.Timestamp {
		return fmt.Errorf("timestamp older than parent")
	}
	// verify mix digest
	if header.MixHash != PolyMixDigest {
		return fmt.Errorf("mix digest is not correct")
	}
	// difficulty must be > 0
	if header.Difficulty <= 0 {
		return fmt.Errorf("difficulty should be greater than zero")
	}

	return nil
}

// createStateTransactionWithData creates a state transaction
// with provided target address and inputData parameter which is ABI encoded byte array.
func createStateTransactionWithData(target types.Address, inputData []byte) *types.Transaction {
	tx := &types.Transaction{
		From:     contracts.SystemCaller,
		To:       &target,
		Type:     types.StateTx,
		Input:    inputData,
		Gas:      types.StateTransactionGasLimit,
		GasPrice: big.NewInt(0),
	}

	tx.ComputeHash()

	return tx
}
