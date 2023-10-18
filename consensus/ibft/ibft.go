package ibft

import (
	"errors"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/fork"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/syncer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/armon/go-metrics"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

const (
	DefaultEpochSize = 100000
	IbftKeyName      = "validator.key"
	KeyEpochSize     = "epochSize"

	ibftProto = "/ibft/0.2"

	// consensusMetrics is a prefix used for consensus-related metrics
	consensusMetrics = "consensus"
)

var (
	ErrInvalidHookParam           = errors.New("invalid IBFT hook param passed in")
	ErrProposerSealByNonValidator = errors.New("proposer seal by non-validator")
	ErrInvalidMixHash             = errors.New("invalid mixhash")
	ErrInvalidSha3Uncles          = errors.New("invalid sha3 uncles")
	ErrWrongDifficulty            = errors.New("wrong difficulty")
)

type txPoolInterface interface {
	Prepare()
	Length() uint64
	Peek() *types.Transaction
	Pop(tx *types.Transaction)
	Drop(tx *types.Transaction)
	Demote(tx *types.Transaction)
	ResetWithHeaders(headers ...*types.Header)
	SetSealing(bool)
}

type forkManagerInterface interface {
	Initialize() error
	Close() error
	GetSigner(uint64) (signer.Signer, error)
	GetValidatorStore(uint64) (fork.ValidatorStore, error)
	GetValidators(uint64) (validators.Validators, error)
	GetHooks(uint64) fork.HooksInterface
}

// backendIBFT represents the IBFT consensus mechanism object
type backendIBFT struct {
	consensus *IBFTConsensus

	// Static References
	logger         hclog.Logger           // Reference to the logging
	blockchain     *blockchain.Blockchain // Reference to the blockchain layer
	network        *network.Server        // Reference to the networking layer
	executor       *state.Executor        // Reference to the state executor
	txpool         txPoolInterface        // Reference to the transaction pool
	syncer         syncer.Syncer          // Reference to the sync protocol
	secretsManager secrets.SecretsManager // Reference to the secret manager
	Grpc           *grpc.Server           // Reference to the gRPC manager
	operator       *operator              // Reference to the gRPC service of IBFT
	transport      transport              // Reference to the transport protocol

	// Dynamic References
	forkManager       forkManagerInterface  // Manager to hold IBFT Forks
	currentSigner     signer.Signer         // Signer at current sequence
	currentValidators validators.Validators // signer at current sequence
	currentHooks      fork.HooksInterface   // Hooks at current sequence

	// Configurations
	config             *consensus.Config // Consensus configuration
	epochSize          uint64
	quorumSizeBlockNum uint64
	blockTime          time.Duration // Minimum block generation time in seconds

	// Channels
	closeCh chan struct{} // Channel for closing
}

// Factory implements the base consensus Factory method
func Factory(params *consensus.Params) (consensus.Consensus, error) {
	// defaults for user set fields in genesis
	var (
		epochSize          = uint64(DefaultEpochSize)
		quorumSizeBlockNum = uint64(0)
	)

	if definedEpochSize, ok := params.Config.Config[KeyEpochSize]; ok {
		// Epoch size is defined, use the passed in one
		readSize, ok := definedEpochSize.(float64)
		if !ok {
			return nil, errors.New("invalid type assertion")
		}

		epochSize = uint64(readSize)
	}

	if rawBlockNum, ok := params.Config.Config["quorumSizeBlockNum"]; ok {
		// Block number specified for quorum size switch
		readBlockNum, ok := rawBlockNum.(float64)
		if !ok {
			return nil, errors.New("invalid type assertion")
		}

		quorumSizeBlockNum = uint64(readBlockNum)
	}

	logger := params.Logger.Named("ibft")

	forkManager, err := fork.NewForkManager(
		logger,
		params.Blockchain,
		params.Executor,
		params.SecretsManager,
		params.Config.Path,
		epochSize,
		params.Config.Config,
	)

	if err != nil {
		return nil, err
	}

	p := &backendIBFT{
		// References
		logger:     logger,
		blockchain: params.Blockchain,
		network:    params.Network,
		executor:   params.Executor,
		txpool:     params.TxPool,
		syncer: syncer.NewSyncer(
			params.Logger,
			params.Network,
			params.Blockchain,
			time.Duration(params.BlockTime)*3*time.Second,
		),
		secretsManager: params.SecretsManager,
		Grpc:           params.Grpc,
		forkManager:    forkManager,

		// Configurations
		config:             params.Config,
		epochSize:          epochSize,
		quorumSizeBlockNum: quorumSizeBlockNum,
		blockTime:          time.Duration(params.BlockTime) * time.Second,

		// Channels
		closeCh: make(chan struct{}),
	}

	// Istanbul requires a different header hash function
	p.SetHeaderHash()

	return p, nil
}

func (i *backendIBFT) Initialize() error {
	// register the grpc operator
	if i.Grpc != nil {
		i.operator = &operator{ibft: i}
		proto.RegisterIbftOperatorServer(i.Grpc, i.operator)
	}

	// start the transport protocol
	if err := i.setupTransport(); err != nil {
		return err
	}

	// initialize fork manager
	if err := i.forkManager.Initialize(); err != nil {
		return err
	}

	if err := i.updateCurrentModules(i.blockchain.Header().Number + 1); err != nil {
		return err
	}

	i.logger.Info("validator key", "addr", i.currentSigner.Address().String())

	i.consensus = newIBFT(
		i.logger.Named("consensus"),
		i,
		i,
	)

	// Ensure consensus takes into account user configured block production time
	i.consensus.ExtendRoundTimeout(i.blockTime)

	return nil
}

// sync runs the syncer in the background to receive blocks from advanced peers
func (i *backendIBFT) startSyncing() {
	callInsertBlockHook := func(fullBlock *types.FullBlock) bool {
		if err := i.currentHooks.PostInsertBlock(fullBlock.Block); err != nil {
			i.logger.Error("failed to call PostInsertBlock", "height", fullBlock.Block.Header.Number, "error", err)
		}

		if err := i.updateCurrentModules(fullBlock.Block.Number() + 1); err != nil {
			i.logger.Error("failed to update sub modules", "height", fullBlock.Block.Number()+1, "err", err)
		}

		i.txpool.ResetWithHeaders(fullBlock.Block.Header)

		return false
	}

	if err := i.syncer.Sync(
		callInsertBlockHook,
	); err != nil {
		i.logger.Error("watch sync failed", "err", err)
	}
}

// Start starts the IBFT consensus
func (i *backendIBFT) Start() error {
	// Start the syncer
	if err := i.syncer.Start(); err != nil {
		return err
	}

	// Start syncing blocks from other peers
	go i.startSyncing()

	// Start the actual consensus protocol
	go i.startConsensus()

	return nil
}

// GetSyncProgression gets the latest sync progression, if any
func (i *backendIBFT) GetSyncProgression() *progress.Progression {
	return i.syncer.GetSyncProgression()
}

func (i *backendIBFT) startConsensus() {
	var (
		newBlockSub   = i.blockchain.SubscribeEvents()
		syncerBlockCh = make(chan struct{})
	)

	// Receive a notification every time syncer manages
	// to insert a valid block. Used for cancelling active consensus
	// rounds for a specific height
	go func() {
		eventCh := newBlockSub.GetEventCh()

		for {
			if ev := <-eventCh; ev.Source == "syncer" {
				if ev.NewChain[0].Number < i.blockchain.Header().Number {
					// The blockchain notification system can eventually deliver
					// stale block notifications. These should be ignored
					continue
				}

				syncerBlockCh <- struct{}{}
			}
		}
	}()

	defer i.blockchain.UnsubscribeEvents(newBlockSub)

	var (
		sequenceCh  = make(<-chan struct{})
		isValidator bool
	)

	for {
		var (
			latest  = i.blockchain.Header().Number
			pending = latest + 1
		)

		if err := i.updateCurrentModules(pending); err != nil {
			i.logger.Error(
				"failed to update submodules",
				"height", pending,
				"err", err,
			)
		}

		// Update the No.of validator metric
		metrics.SetGauge([]string{consensusMetrics, "validators"}, float32(i.currentValidators.Len()))

		isValidator = i.isActiveValidator()

		i.txpool.SetSealing(isValidator)

		if isValidator {
			sequenceCh = i.consensus.runSequence(pending)
		}

		select {
		case <-syncerBlockCh:
			if isValidator {
				i.consensus.stopSequence()
				i.logger.Info("canceled sequence", "sequence", pending)
			}
		case <-sequenceCh:
		case <-i.closeCh:
			if isValidator {
				i.consensus.stopSequence()
			}

			return
		}
	}
}

// isActiveValidator returns whether my signer belongs to current validators
func (i *backendIBFT) isActiveValidator() bool {
	return i.currentValidators.Includes(i.currentSigner.Address())
}

// updateMetrics will update various metrics based on the given block
// currently we capture No.of Txs and block interval metrics using this function
func (i *backendIBFT) updateMetrics(block *types.Block) {
	// get previous header
	prvHeader, _ := i.blockchain.GetHeaderByNumber(block.Number() - 1)
	parentTime := time.Unix(int64(prvHeader.Timestamp), 0)
	headerTime := time.Unix(int64(block.Header.Timestamp), 0)

	// Update the block interval metric
	if block.Number() > 1 {
		metrics.SetGauge([]string{consensusMetrics, "block_interval"}, float32(headerTime.Sub(parentTime).Seconds()))
	}

	// Update the Number of transactions in the block metric
	metrics.SetGauge([]string{consensusMetrics, "num_txs"}, float32(len(block.Body().Transactions)))

	// Update the base fee metric
	metrics.SetGauge([]string{consensusMetrics, "base_fee"}, float32(block.Header.BaseFee))
}

// verifyHeaderImpl verifies fields including Extra
// for the past or being proposed header
func (i *backendIBFT) verifyHeaderImpl(
	parent, header *types.Header,
	headerSigner signer.Signer,
	validators validators.Validators,
	hooks fork.HooksInterface,
	shouldVerifyParentCommittedSeals bool,
) error {
	if header.MixHash != signer.IstanbulDigest {
		return ErrInvalidMixHash
	}

	if header.Sha3Uncles != types.EmptyUncleHash {
		return ErrInvalidSha3Uncles
	}

	// difficulty has to match number
	if header.Difficulty != header.Number {
		return ErrWrongDifficulty
	}

	// ensure the extra data is correctly formatted
	if _, err := headerSigner.GetIBFTExtra(header); err != nil {
		return err
	}

	// verify the ProposerSeal
	if err := verifyProposerSeal(
		header,
		headerSigner,
		validators,
	); err != nil {
		return err
	}

	// verify the ParentCommittedSeals
	if err := i.verifyParentCommittedSeals(
		parent, header,
		shouldVerifyParentCommittedSeals,
	); err != nil {
		return err
	}

	// Additional header verification
	if err := hooks.VerifyHeader(header); err != nil {
		return err
	}

	return nil
}

// VerifyHeader wrapper for verifying headers
func (i *backendIBFT) VerifyHeader(header *types.Header) error {
	parent, ok := i.blockchain.GetHeaderByNumber(header.Number - 1)
	if !ok {
		return fmt.Errorf(
			"unable to get parent header for block number %d",
			header.Number,
		)
	}

	headerSigner, validators, hooks, err := getModulesFromForkManager(
		i.forkManager,
		header.Number,
	)
	if err != nil {
		return err
	}

	// verify all the header fields
	if err := i.verifyHeaderImpl(
		parent,
		header,
		headerSigner,
		validators,
		hooks,
		false,
	); err != nil {
		return err
	}

	extra, err := headerSigner.GetIBFTExtra(header)
	if err != nil {
		return err
	}

	hashForCommittedSeal, err := i.calculateProposalHash(
		headerSigner,
		header,
		extra.RoundNumber,
	)
	if err != nil {
		return err
	}

	// verify the Committed Seals
	// CommittedSeals exists only in the finalized header
	if err := headerSigner.VerifyCommittedSeals(
		hashForCommittedSeal,
		extra.CommittedSeals,
		validators,
		i.quorumSize(header.Number)(validators),
	); err != nil {
		return err
	}

	return nil
}

// quorumSize returns a callback that when executed on a Validators computes
// number of votes required to reach quorum based on the size of the set.
// The blockNumber argument indicates which formula was used to calculate the result (see PRs #513, #549)
func (i *backendIBFT) quorumSize(blockNumber uint64) QuorumImplementation {
	if blockNumber < i.quorumSizeBlockNum {
		return LegacyQuorumSize
	}

	return OptimalQuorumSize
}

// ProcessHeaders updates the snapshot based on previously verified headers
func (i *backendIBFT) ProcessHeaders(headers []*types.Header) error {
	for _, header := range headers {
		hooks := i.forkManager.GetHooks(header.Number)

		if err := hooks.ProcessHeader(header); err != nil {
			return err
		}
	}

	return nil
}

// GetBlockCreator retrieves the block signer from the extra data field
func (i *backendIBFT) GetBlockCreator(header *types.Header) (types.Address, error) {
	signer, err := i.forkManager.GetSigner(header.Number)
	if err != nil {
		return types.ZeroAddress, err
	}

	return signer.EcrecoverFromHeader(header)
}

// PreCommitState a hook to be called before finalizing state transition on inserting block
func (i *backendIBFT) PreCommitState(block *types.Block, txn *state.Transition) error {
	hooks := i.forkManager.GetHooks(block.Number())

	return hooks.PreCommitState(block.Header, txn)
}

// GetEpoch returns the current epoch
func (i *backendIBFT) GetEpoch(number uint64) uint64 {
	if number%i.epochSize == 0 {
		return number / i.epochSize
	}

	return number/i.epochSize + 1
}

// IsLastOfEpoch checks if the block number is the last of the epoch
func (i *backendIBFT) IsLastOfEpoch(number uint64) bool {
	return number > 0 && number%i.epochSize == 0
}

// Close closes the IBFT consensus mechanism, and does write back to disk
func (i *backendIBFT) Close() error {
	close(i.closeCh)

	if i.syncer != nil {
		if err := i.syncer.Close(); err != nil {
			return err
		}
	}

	if i.forkManager != nil {
		if err := i.forkManager.Close(); err != nil {
			return err
		}
	}

	return nil
}

// SetHeaderHash updates hash calculation function for IBFT
func (i *backendIBFT) SetHeaderHash() {
	types.HeaderHash = func(h *types.Header) types.Hash {
		signer, err := i.forkManager.GetSigner(h.Number)
		if err != nil {
			return types.ZeroHash
		}

		hash, err := signer.CalculateHeaderHash(h)
		if err != nil {
			return types.ZeroHash
		}

		return hash
	}
}

// GetBridgeProvider returns an instance of BridgeDataProvider
func (i *backendIBFT) GetBridgeProvider() consensus.BridgeDataProvider {
	return nil
}

// FilterExtra is the implementation of Consensus interface
func (i *backendIBFT) FilterExtra(extra []byte) ([]byte, error) {
	return extra, nil
}

// updateCurrentModules updates Signer, Hooks, and Validators
// that are used at specified height
// by fetching from ForkManager
func (i *backendIBFT) updateCurrentModules(height uint64) error {
	lastSigner := i.currentSigner

	signer, validators, hooks, err := getModulesFromForkManager(i.forkManager, height)
	if err != nil {
		return err
	}

	i.currentSigner = signer
	i.currentValidators = validators
	i.currentHooks = hooks

	i.logFork(lastSigner, signer)

	return nil
}

// logFork logs validation type switch
func (i *backendIBFT) logFork(
	lastSigner, signer signer.Signer,
) {
	if lastSigner != nil && signer != nil && lastSigner.Type() != signer.Type() {
		i.logger.Info("IBFT validation type switched", "old", lastSigner.Type(), "new", signer.Type())
	}
}

func (i *backendIBFT) verifyParentCommittedSeals(
	parent, header *types.Header,
	shouldVerifyParentCommittedSeals bool,
) error {
	if parent.IsGenesis() {
		return nil
	}

	parentSigner, parentValidators, _, err := getModulesFromForkManager(
		i.forkManager,
		parent.Number,
	)
	if err != nil {
		return err
	}

	parentHeader, ok := i.blockchain.GetHeaderByHash(parent.Hash)
	if !ok {
		return fmt.Errorf("header %s not found", parent.Hash)
	}

	parentExtra, err := parentSigner.GetIBFTExtra(parentHeader)
	if err != nil {
		return err
	}

	parentHash, err := i.calculateProposalHash(
		parentSigner,
		parentHeader,
		parentExtra.RoundNumber,
	)
	if err != nil {
		return err
	}

	// if shouldVerifyParentCommittedSeals is false, skip the verification
	// when header doesn't have Parent Committed Seals (Backward Compatibility)
	return parentSigner.VerifyParentCommittedSeals(
		parentHash,
		header,
		parentValidators,
		i.quorumSize(parent.Number)(parentValidators),
		shouldVerifyParentCommittedSeals,
	)
}

// getModulesFromForkManager is a helper function to get all modules from ForkManager
func getModulesFromForkManager(forkManager forkManagerInterface, height uint64) (
	signer.Signer,
	validators.Validators,
	fork.HooksInterface,
	error,
) {
	signer, err := forkManager.GetSigner(height)
	if err != nil {
		return nil, nil, nil, err
	}

	validators, err := forkManager.GetValidators(height)
	if err != nil {
		return nil, nil, nil, err
	}

	hooks := forkManager.GetHooks(height)

	return signer, validators, hooks, nil
}

// verifyProposerSeal verifies ProposerSeal in IBFT Extra of header
// and make sure signer belongs to validators
func verifyProposerSeal(
	header *types.Header,
	signer signer.Signer,
	validators validators.Validators,
) error {
	proposer, err := signer.EcrecoverFromHeader(header)
	if err != nil {
		return err
	}

	if !validators.Includes(proposer) {
		return ErrProposerSealByNonValidator
	}

	return nil
}

// ValidateExtraDataFormat Verifies that extra data can be unmarshaled
func (i *backendIBFT) ValidateExtraDataFormat(header *types.Header) error {
	blockSigner, _, _, err := getModulesFromForkManager(
		i.forkManager,
		header.Number,
	)

	if err != nil {
		return err
	}

	_, err = blockSigner.GetIBFTExtra(header)

	return err
}
