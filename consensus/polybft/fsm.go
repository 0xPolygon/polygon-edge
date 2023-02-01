package polybft

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
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
	Fill()
	Build(func(h *types.Header)) (*types.FullBlock, error)
	GetState() *state.Transition
	Receipts() []*types.Receipt
}

var (
	errUptimeTxDoesNotExist = errors.New("uptime transaction is not found in the epoch ending block")
)

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

	// proposerSnapshot keeps information about new proposer
	proposerSnapshot *ProposerSnapshot

	// blockBuilder is the block builder for proposers
	blockBuilder blockBuilder

	// epochNumber denotes current epoch number
	epochNumber uint64

	// uptimeCounter holds info about number of times validators sealed a block (only present if isEndOfEpoch is true)
	uptimeCounter *contractsapi.CommitEpochFunction

	// isEndOfEpoch indicates if epoch reached its end
	isEndOfEpoch bool

	// isEndOfSprint indicates if sprint reached its end
	isEndOfSprint bool

	// proposerCommitmentToRegister is a commitment that is registered via state transaction by proposer
	proposerCommitmentToRegister *CommitmentMessageSigned

	// logger instance
	logger hcf.Logger

	// target is the block being computed
	target *types.FullBlock

	// exitEventRootHash is the calculated root hash for given checkpoint block
	exitEventRootHash types.Hash
}

// BuildProposal builds a proposal for the current round (used if proposer)
func (f *fsm) BuildProposal(currentRound uint64) ([]byte, error) {
	parent := f.parent

	extraParent, err := GetIbftExtra(parent.ExtraData)
	if err != nil {
		return nil, err
	}

	// TODO: we will need to revisit once slashing is implemented
	extra := &Extra{Parent: extraParent.Committed}
	// for non-epoch ending blocks, currentValidatorsHash is the same as the nextValidatorsHash
	nextValidators := f.validators.Accounts()

	if err := f.blockBuilder.Reset(); err != nil {
		return nil, fmt.Errorf("failed to initialize block builder: %w", err)
	}

	if f.isEndOfEpoch {
		tx, err := f.createValidatorsUptimeTx()
		if err != nil {
			return nil, err
		}

		if err := f.blockBuilder.WriteTx(tx); err != nil {
			return nil, fmt.Errorf("failed to commit validators uptime transaction: %w", err)
		}

		nextValidators, err = f.getCurrentValidators(f.blockBuilder.GetState())
		if err != nil {
			return nil, err
		}

		validatorsDelta, err := createValidatorSetDelta(f.validators.Accounts(), nextValidators)
		if err != nil {
			return nil, fmt.Errorf("failed to create validator set delta: %w", err)
		}

		extra.Validators = validatorsDelta
		f.logger.Trace("[FSM Build Proposal]", "Validators Delta", validatorsDelta)
	}

	if f.config.IsBridgeEnabled() {
		for _, tx := range f.stateTransactions() {
			if err := f.blockBuilder.WriteTx(tx); err != nil {
				return nil, fmt.Errorf("failed to commit state transaction. Error: %w", err)
			}
		}
	}

	// fill the block with transactions
	f.blockBuilder.Fill()

	currentValidatorsHash, err := f.validators.Accounts().Hash()
	if err != nil {
		return nil, err
	}

	nextValidatorsHash, err := nextValidators.Hash()
	if err != nil {
		return nil, err
	}

	extra.Checkpoint = &CheckpointData{
		BlockRound:            currentRound,
		EpochNumber:           f.epochNumber,
		CurrentValidatorsHash: currentValidatorsHash,
		NextValidatorsHash:    nextValidatorsHash,
		EventRoot:             f.exitEventRootHash,
	}

	stateBlock, err := f.blockBuilder.Build(func(h *types.Header) {
		h.ExtraData = append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...)
		h.MixHash = PolyBFTMixDigest
	})

	if err != nil {
		return nil, err
	}

	checkpointHash, err := extra.Checkpoint.Hash(f.backend.GetChainID(), f.Height(), stateBlock.Block.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to calculate sign hash: %w", err)
	}

	f.logger.Debug("[FSM Build Proposal]",
		"txs", len(stateBlock.Block.Transactions),
		"hash", checkpointHash.String())

	f.target = stateBlock

	return stateBlock.Block.MarshalRLP(), nil
}

func (f *fsm) stateTransactions() []*types.Transaction {
	var txns []*types.Transaction

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

	// validate header fields
	if err := validateHeaderFields(f.parent, block.Header); err != nil {
		return fmt.Errorf(
			"failed to validate header (parent header# %d, current header#%d): %w",
			f.parent.Number,
			block.Number(),
			err,
		)
	}

	currentExtra, err := GetIbftExtra(block.Header.ExtraData)
	if err != nil {
		return fmt.Errorf("cannot get extra data:%w", err)
	}

	parentExtra, err := GetIbftExtra(f.parent.ExtraData)
	if err != nil {
		return err
	}

	if err := f.VerifyStateTransactions(block.Transactions); err != nil {
		return err
	}

	currentValidators := f.validators.Accounts()
	nextValidators := f.validators.Accounts()

	validateExtraData := func(transition *state.Transition) error {
		nextValidators, err = f.getCurrentValidators(transition)
		if err != nil {
			return err
		}

		if err := currentExtra.Validate(parentExtra, currentValidators, nextValidators); err != nil {
			return err
		}

		return nil
	}

	// TODO: Validate validator set delta?

	// TODO: Move signature validation logic to Extra
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

		parentCheckpointHash, err := parentExtra.Checkpoint.Hash(f.backend.GetChainID(), f.parent.Number, f.parent.Hash)
		if err != nil {
			return fmt.Errorf("failed to calculate parent block sign hash: %w", err)
		}

		if err := currentExtra.Parent.VerifyCommittedFields(validators, parentCheckpointHash, f.logger); err != nil {
			return fmt.Errorf(
				"failed to verify signatures for (parent) block#%d, parent signed hash: %v, current block#%d: %w",
				f.parent.Number,
				parentCheckpointHash,
				blockNumber,
				err,
			)
		}
	}

	stateBlock, err := f.backend.ProcessBlock(f.parent, &block, validateExtraData)
	if err != nil {
		return err
	}

	checkpointHash, err := currentExtra.Checkpoint.Hash(f.backend.GetChainID(), block.Number(), block.Hash())
	if err != nil {
		return fmt.Errorf("failed to calculate signed hash: %w", err)
	}

	f.logger.Debug("[FSM Validate]", "txs", len(block.Transactions), "signed hash", checkpointHash)

	f.target = stateBlock

	return nil
}

// ValidateSender validates sender address and signature
func (f *fsm) ValidateSender(msg *proto.Message) error {
	msgNoSig, err := msg.PayloadNoSig()
	if err != nil {
		return err
	}

	signerAddress, err := wallet.RecoverAddressFromSignature(msg.Signature, msgNoSig)
	if err != nil {
		return fmt.Errorf("failed to recover address from signature: %w", err)
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
	var (
		commitmentMessageSignedExists bool
		uptimeTransactionExists       bool
	)

	for _, tx := range transactions {
		if tx.Type != types.StateTx {
			continue
		}

		decodedStateTx, err := decodeStateTransaction(tx.Input) // used to be Data
		if err != nil {
			return fmt.Errorf("unknown state transaction: tx = %v, err = %w", tx.Hash, err)
		}

		switch stateTxData := decodedStateTx.(type) {
		case *CommitmentMessageSigned:
			if !f.isEndOfSprint {
				return fmt.Errorf("found commitment in block which should not contain it: tx = %v", tx.Hash)
			}

			if commitmentMessageSignedExists {
				return fmt.Errorf("only one commitment is allowed per block: %v", tx.Hash)
			}

			commitmentMessageSignedExists = true
			signers, err := f.validators.Accounts().GetFilteredValidators(stateTxData.AggSignature.Bitmap)

			if err != nil {
				return fmt.Errorf("error for state transaction while retrieving signers: tx = %v, error = %w", tx.Hash, err)
			}

			if !f.validators.HasQuorum(signers.GetAddressesAsSet()) {
				return fmt.Errorf("quorum size not reached for state tx: %v", tx.Hash)
			}

			aggs, err := bls.UnmarshalSignature(stateTxData.AggSignature.AggregatedSignature)
			if err != nil {
				return fmt.Errorf("error for state transaction while unmarshaling signature: tx = %v, error = %w", tx.Hash, err)
			}

			hash, err := stateTxData.Hash()
			if err != nil {
				return err
			}

			verified := aggs.VerifyAggregated(signers.GetBlsKeys(), hash.Bytes())
			if !verified {
				return fmt.Errorf("invalid signature for tx = %v", tx.Hash)
			}
		case *contractsapi.CommitEpochFunction:
			uptimeTransactionExists = true

			if err := f.verifyValidatorsUptimeTx(tx); err != nil {
				return fmt.Errorf("error while verifying uptime transaction. error: %w", err)
			}
		}
	}

	if f.isEndOfEpoch && !uptimeTransactionExists {
		return errUptimeTxDoesNotExist
	}

	return nil
}

// Insert inserts the sealed proposal
func (f *fsm) Insert(proposal []byte, committedSeals []*messages.CommittedSeal) (*types.FullBlock, error) {
	newBlock := f.target

	// In this function we should try to return little to no errors since
	// at this point everything we have to do is just commit something that
	// we should have already computed beforehand.
	extra, _ := GetIbftExtra(newBlock.Block.Header.ExtraData)

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

	// Write extra data to header
	newBlock.Block.Header.ExtraData = append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...)

	if err := f.backend.CommitBlock(newBlock); err != nil {
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

// getCurrentValidators queries smart contract on the given block height and returns currently active validator set
func (f *fsm) getCurrentValidators(pendingBlockState *state.Transition) (AccountSet, error) {
	provider := f.backend.GetStateProvider(pendingBlockState)
	systemState := f.backend.GetSystemState(f.config, provider)
	newValidators, err := systemState.GetValidatorSet()

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve validator set for current block: %w", err)
	}

	if f.logger.IsDebug() {
		f.logger.Debug("getCurrentValidators", "Validator set", newValidators.String())
	}

	return newValidators, nil
}

// verifyValidatorsUptimeTx creates uptime transaction and compares its hash with the one extracted from the block.
func (f *fsm) verifyValidatorsUptimeTx(blockUptimeTx *types.Transaction) error {
	if f.isEndOfEpoch {
		createdUptimeTx, err := f.createValidatorsUptimeTx()
		if err != nil {
			return err
		}

		if blockUptimeTx.Hash != createdUptimeTx.Hash {
			return fmt.Errorf(
				"invalid uptime transaction. Expected '%s', but got '%s' uptime transaction hash",
				blockUptimeTx.Hash,
				createdUptimeTx.Hash,
			)
		}

		return nil
	}

	return errors.New("didn't expect uptime transaction in a non ending epoch block")
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
	if header.MixHash != PolyBFTMixDigest {
		return fmt.Errorf("mix digest is not correct")
	}
	// difficulty must be > 0
	if header.Difficulty <= 0 {
		return fmt.Errorf("difficulty should be greater than zero")
	}
	// calculated header hash must be correct
	if header.Hash != types.HeaderHash(header) {
		return fmt.Errorf("invalid header hash")
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
