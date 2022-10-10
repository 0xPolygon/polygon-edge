package polybft

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
)

var _ pbft.Backend = &fsm{}

type blockBuilder interface {
	Reset()
	CommitTransaction(tx *types.Transaction) error
	Fill(ctx context.Context) error
	Build(func(h *types.Header)) *StateBlock
	GetState() state.Snapshot
}

const (
	stateTransactionsGasLimit = 1000000 // some arbitrary default gas limit for state transactions
	maxBundlesPerSprint       = 50
)

type fsm struct {
	// PolyBFT consensus protocol configuration
	config *PolyBFTConfig

	// parent block header
	parent *types.Header

	// backend implements methods for retrieving data from block chain
	backend blockchain

	// polybftBackend implements methods needed from the polybft
	polybftBackend polybftBackend

	// validators is the list of pbft validators for this round
	validators ValidatorSet

	// blockBuilder is the block builder for proposers
	blockBuilder blockBuilder

	// block is the current block being process in this round.
	// It should be populated after the Accept state in pbft-consensus.
	block *StateBlock

	// proposal is the current proposal being processed
	proposal *pbft.Proposal

	// postInsertHook represents custom handler which is executed once fsm.Insert is invoked,
	// meaning that current block is inserted successfully
	postInsertHook func() error

	// uptimeCounter holds info about number of times validators sealed a block (only present if isEndOfEpoch is true)
	uptimeCounter *UptimeCounter

	// isEndOfEpoch indicates if epoch reached its end
	isEndOfEpoch bool

	// isEndOfSprint indicates if sprint reached its end
	isEndOfSprint bool

	// current epoch
	epoch uint64

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

func (f *fsm) Init(info *pbft.RoundInfo) {
	f.blockBuilder.Reset()
	f.commitmentToSaveOnRegister = nil
}

// BuildProposal builds a proposal for the current round (used if proposer)
func (f *fsm) BuildProposal() (*pbft.Proposal, error) {
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
		if err := f.blockBuilder.CommitTransaction(tx); err != nil {
			return nil, fmt.Errorf("failed to commit validators uptime transaction: %v", err)
		}

		validatorsDelta, err := f.getValidatorSetDelta(f.blockBuilder.GetState())
		if err != nil {
			return nil, err
		}
		extra.Validators = validatorsDelta
		f.logger.Trace("[FSM Build Proposal]", "Validators Delta", validatorsDelta)
	}

	if f.config.IsBridgeEnabled() {
		for _, tx := range f.stateTransactions() {
			if err := f.blockBuilder.CommitTransaction(tx); err != nil {
				return nil, fmt.Errorf("failed to commit state transaction. Error: %v", err)
			}
		}
	}

	// set the timestamp
	parentTime := time.Unix(int64(parent.Timestamp), 0)
	headerTime := parentTime.Add(f.config.BlockTime)

	if headerTime.Before(time.Now()) {
		headerTime = time.Now()
	}

	// fill the block with transactions
	now := time.Now()
	if err := f.blockBuilder.Fill(context.Background()); err != nil {
		return nil, err
	}

	f.logger.Debug("Fill block", "time", time.Since(now))

	buildBlock := f.blockBuilder.Build(func(h *types.Header) {
		h.Timestamp = uint64(headerTime.Unix())
		h.ExtraData = append(make([]byte, 32), extra.MarshalRLPTo(nil)...)
		h.MixHash = PolyMixDigest
	})

	f.block = buildBlock

	rlpBlock, err := buildBlock.EncodeRlpBlock()
	if err != nil {
		return nil, fmt.Errorf("failed to encode rlp block: %v", err)
	}

	proposal := &pbft.Proposal{
		Time: headerTime,
		Data: rlpBlock,
		Hash: f.block.Block.Hash().Bytes(),
	}
	f.proposal = proposal
	f.logger.Debug("[FSM Build Proposal]", "Proposal hash", hex.EncodeToHex(proposal.Hash))
	return proposal, nil
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
				createStateTransactionWithData(f.config.SidechainBridgeAddr, inputData, stateTransactionsGasLimit))

			// since proposer does not execute Validate (when we see the commitment to register in state transactions)
			// we need to set commitment to save so that the proposer also saves its commitment that he registered
			f.commitmentToSaveOnRegister = f.proposerCommitmentToRegister
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
				createStateTransactionWithData(f.config.SidechainBridgeAddr, inputData, stateTransactionsGasLimit))
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
	return createStateTransactionWithData(f.config.ValidatorSetAddr, input, stateTransactionsGasLimit), nil
}

// ValidateCommit is used to validate that a given commit is valid
func (f *fsm) ValidateCommit(from pbft.NodeID, seal []byte) error {
	if f.proposal == nil || f.proposal.Hash == nil {
		return fmt.Errorf("incorrect commit from %s. proposal unavailable", from)
	}

	fromAddress := types.Address(ethgo.HexToAddress(string(from)))
	validator := f.validators.Accounts().GetValidatorAccount(fromAddress)

	if validator == nil {
		return fmt.Errorf("unable to resolve validator %s", from)
	}

	signature, err := bls.UnmarshalSignature(seal)
	if err != nil {
		return fmt.Errorf("failed to unmarshall signature: %v", err)
	}
	if !signature.Verify(validator.BlsKey, f.proposal.Hash) {
		return fmt.Errorf("incorrect commit signature from %s", from)
	}

	return nil
}

// Validate validates a raw proposal (used if non-proposer)
func (f *fsm) Validate(proposal *pbft.Proposal) error {
	f.logger.Debug("[FSM Validate]", "Proposal hash", hex.EncodeToHex(proposal.Hash))

	var block types.Block
	if err := rlp.DecodeBytes(proposal.Data, &block); err != nil {
		return fmt.Errorf("failed to decode block data. Error: %v", err)
	}

	// validate proposal
	if block.Hash() != types.BytesToHash(proposal.Hash) {
		return fmt.Errorf("incorrect sign hash (current header#%d)", block.Number())
	}

	// validate header fields
	if err := validateHeaderFields(f.parent, block.Header); err != nil {
		return fmt.Errorf("failed to validate header (parent header#%d, current header#%d): %v",
			f.parent.Number, block.Number, err)
	}

	blockExtra, err := GetIbftExtra(block.Header.ExtraData)
	if err != nil {
		return err
	}

	// TODO: Validate validator set delta?

	blockNumber := block.Number()
	if blockNumber > 1 {
		// verify parent signature
		// We skip block 0 (genesis) and block 1 (parent is genesis)
		// since those blocks do not include any parent information with signatures
		validators, err := f.polybftBackend.GetValidators(blockNumber-2, nil)
		if err != nil {
			return err
		}
		f.logger.Trace("[FSM Validate]", "Block", blockNumber, "parent validators", validators)
		parentHash := f.parent.Hash
		if err := blockExtra.Parent.VerifyCommittedFields(validators, parentHash); err != nil {
			return fmt.Errorf(
				"failed to verify signatures for (parent) block#%d. Block hash: %v, block#%d",
				f.parent.Number,
				parentHash,
				blockNumber,
			)
		}
	}

	if err := f.VerifyStateTransactions(block.Transactions); err != nil {
		return err
	}

	builtBlock, err := f.backend.ProcessBlock(f.parent, &block)
	if err != nil {
		return err
	}
	f.block = builtBlock
	f.proposal = proposal
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
		// skip if transaction is not of types.StateTransactionType type
		if tx.Type != types.StateTransactionType {
			continue
		}

		if !f.isEndOfSprint {
			return fmt.Errorf("state transaction in block which should not contain it: tx = %v", tx.Hash)
		}

		decodedStateTx, err := decodeStateTransaction(tx.Input) // used to be Data
		if err != nil {
			return fmt.Errorf("state transaction error while decoding: tx = %v, err = %v", tx.Hash, err)
		}

		switch stateTxData := decodedStateTx.(type) {
		case *CommitmentMessageSigned:
			if commitmentMessageSignedExists {
				return fmt.Errorf("only one commitment is allowed per block: %v", tx.Hash)
			}

			commitmentMessageSignedExists = true
			signers, err := f.validators.Accounts().GetFilteredValidators(stateTxData.AggSignature.Bitmap)
			if err != nil {
				return fmt.Errorf("error for state transaction while retrieving signers: tx = %v, error = %v", tx.Hash, err)
			}

			if len(signers) < getQuorumSize(f.validators.Len()) {
				return fmt.Errorf("quorum size not reached for state tx: %v", tx.Hash)
			}

			aggs, err := bls.UnmarshalSignature(stateTxData.AggSignature.AggregatedSignature)
			if err != nil {
				return fmt.Errorf("error for state transaction while unmarshaling signature: tx = %v, error = %v", tx.Hash, err)
			}

			verified := aggs.VerifyAggregated(signers.GetBlsKeys(), stateTxData.Message.Hash().Bytes())
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
						return fmt.Errorf("state transaction error while validating proof: tx = %v, err = %v",
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
func (f *fsm) Insert(p *pbft.SealedProposal) error {
	// In this function we should try to return little to no errors since
	// at this point everything we have to do is just commit something that
	// we should have already computed beforehand.

	extra, _ := GetIbftExtra(f.block.Block.Header.ExtraData)

	// create map for faster access to indexes
	nodeIDIndexMap := make(map[pbft.NodeID]int, f.validators.Len())
	for i, addr := range f.validators.Accounts().GetAddresses() {
		nodeIDIndexMap[pbft.NodeID(addr.String())] = i // addr.String() == NodeId
	}

	// populated bitmap according to nodeId from validator set and committed seals
	// also populate slice of signatures
	bitmap := bitmap.Bitmap{}
	signatures := make(bls.Signatures, 0, len(p.CommittedSeals))
	for _, commSeal := range p.CommittedSeals {
		index, exists := nodeIDIndexMap[commSeal.NodeID]
		if !exists {
			return fmt.Errorf("invalid node id = %s", commSeal.NodeID)
		}

		s, err := bls.UnmarshalSignature(commSeal.Signature)
		if err != nil {
			return fmt.Errorf("invalid signature = %s", commSeal.Signature)
		}
		signatures = append(signatures, s)
		bitmap.Set(uint64(index))
	}

	aggregatedSignature, err := signatures.Aggregate().Marshal()
	if err != nil {
		return fmt.Errorf("could not aggregate seals: %v", err.Error())
	}

	// include aggregated signature of all committed seals
	// also includes bitmap which contains all indexes from validator set which provides there seals
	extra.Committed = &Signature{
		AggregatedSignature: aggregatedSignature,
		Bitmap:              bitmap,
	}

	// Write extar data to header
	f.block.Block.Header.ExtraData = append(make([]byte, 32), extra.MarshalRLPTo(nil)...)

	if err := f.backend.CommitBlock(f.block); err != nil {
		return err
	}

	if err := f.postInsertHook(); err != nil {
		return err
	}

	return nil
}

// Height returns the height for the current round
func (f *fsm) Height() uint64 {
	return f.parent.Number + 1
}

// ValidatorSet returns the validator set for the current round
func (f *fsm) ValidatorSet() pbft.ValidatorSet {
	return f.validators
}

// IsStuck returns whether the pbft is stuck
func (f *fsm) IsStuck(num uint64) (uint64, bool) {
	return f.polybftBackend.CheckIfStuck(num)
}

// getValidatorSetDelta calculates validator set delta based on parent and current header
func (f *fsm) getValidatorSetDelta(pendingBlockState vm.StateDB) (*ValidatorSetDelta, error) {
	provider := f.backend.GetStateProviderForDB(pendingBlockState)
	systemState := f.backend.GetSystemState(f.config, provider)
	newValidators, err := systemState.GetValidatorSet()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve validator set for current block %v", err)
	}
	return createValidatorSetDelta(f.validators.Accounts(), newValidators), nil
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
