package ibft

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/syncer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

const (
	DefaultEpochSize = 100000
	IbftKeyName      = "validator.key"
	ibftProto        = "/ibft/0.2"
)

var (
	ErrInvalidHookParam = errors.New("invalid IBFT hook param passed in")
)

type txPoolInterface interface {
	Prepare()
	Length() uint64
	Peek() *types.Transaction
	Pop(tx *types.Transaction)
	Drop(tx *types.Transaction)
	Demote(tx *types.Transaction)
	ResetWithHeaders(headers ...*types.Header)
}

// backendIBFT represents the IBFT consensus mechanism object
type backendIBFT struct {
	logger hclog.Logger

	config *consensus.Config // Consensus configuration

	consensus *IBFTConsensus

	blockchain *blockchain.Blockchain // Interface exposed by the blockchain layer
	network    *network.Server        // Reference to the networking layer
	executor   *state.Executor        // Reference to the state executor
	txpool     txPoolInterface        // Reference to the transaction pool
	syncer     syncer.Syncer          // Reference to the sync protocol
	Grpc       *grpc.Server           // gRPC configuration

	metrics *consensus.Metrics

	secretsManager secrets.SecretsManager

	validatorKey       *ecdsa.PrivateKey // Private key for the validator
	validatorKeyAddr   types.Address
	activeValidatorSet ValidatorSet

	store     *snapshotStore // Snapshot store that keeps track of all snapshots
	transport transport      // Reference to the transport protocol
	operator  *operator

	mechanisms []ConsensusMechanism // IBFT ConsensusMechanism used (PoA / PoS)

	epochSize          uint64
	quorumSizeBlockNum uint64

	blockTime time.Duration // Minimum block generation time in seconds

	sealing bool // Flag indicating if the node is a sealer

	closeCh chan struct{} // Channel for closing
}

// Factory implements the base consensus Factory method
func Factory(params *consensus.Params) (consensus.Consensus, error) {
	// defaults for user set fields in genesis
	var (
		epochSize          = uint64(DefaultEpochSize)
		quorumSizeBlockNum = uint64(0)
	)

	if definedEpochSize, ok := params.Config.Config["epochSize"]; ok {
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

	p := &backendIBFT{
		logger:             params.Logger.Named("ibft"),
		config:             params.Config,
		Grpc:               params.Grpc,
		blockchain:         params.Blockchain,
		executor:           params.Executor,
		closeCh:            make(chan struct{}),
		txpool:             params.TxPool,
		network:            params.Network,
		epochSize:          epochSize,
		quorumSizeBlockNum: quorumSizeBlockNum,
		sealing:            params.Seal,
		metrics:            params.Metrics,
		secretsManager:     params.SecretsManager,
		blockTime:          time.Duration(params.BlockTime) * time.Second,
		syncer: syncer.NewSyncer(
			params.Logger,
			params.Network,
			params.Blockchain,
			time.Duration(params.BlockTime)*3*time.Second),
	}

	// Initialize the mechanism
	if err := p.setupMechanism(); err != nil {
		return nil, err
	}

	// Istanbul requires a different header hash function
	types.HeaderHash = istanbulHeaderHash

	return p, nil
}

// runHook runs a specified hook if it is present in the hook map
func (i *backendIBFT) runHook(hookName HookType, height uint64, hookParam interface{}) error {
	for _, mechanism := range i.mechanisms {
		if !mechanism.IsAvailable(hookName, height) {
			continue
		}

		// Grab the hook map
		hookMap := mechanism.GetHookMap()

		// Grab the actual hook if it's present
		hook, ok := hookMap[hookName]
		if !ok {
			// hook not found, continue
			continue
		}

		// Run the hook
		if err := hook(hookParam); err != nil {
			return fmt.Errorf("error occurred during a call of %s hook in %s: %w", hookName, mechanism.GetType(), err)
		}
	}

	return nil
}

func (i *backendIBFT) Initialize() error {
	// register the grpc operator
	if i.Grpc != nil {
		i.operator = &operator{ibft: i}
		proto.RegisterIbftOperatorServer(i.Grpc, i.operator)
	}

	// Set up the node's validator key
	if err := i.createKey(); err != nil {
		return err
	}

	i.logger.Info("validator key", "addr", i.validatorKeyAddr.String())

	// start the transport protocol
	if err := i.setupTransport(); err != nil {
		return err
	}

	i.consensus = newIBFT(
		i.logger.Named("consensus"),
		i,
		i,
	)

	// Ensure consensus takes into account user configured block production time
	i.consensus.ExtendRoundTimeout(i.blockTime)

	// Set up the snapshots
	if err := i.setupSnapshot(); err != nil {
		return err
	}

	snap, err := i.getLatestSnapshot()
	if err != nil {
		return err
	}

	i.activeValidatorSet = snap.Set

	return nil
}

// sync runs the syncer in the background to receive blocks from advanced peers
func (i *backendIBFT) startSyncing() {
	callInsertBlockHook := func(blockNumber uint64) {
		if err := i.runHook(InsertBlockHook, blockNumber, blockNumber); err != nil {
			i.logger.Error(fmt.Sprintf("Unable to run hook %s, %v", InsertBlockHook, err))
		}
	}

	if err := i.syncer.Sync(
		func(block *types.Block) bool {
			callInsertBlockHook(block.Number())
			i.txpool.ResetWithHeaders(block.Header)

			return false
		},
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

// GetIBFTForks returns IBFT fork configurations from chain config
func GetIBFTForks(ibftConfig map[string]interface{}) ([]IBFTFork, error) {
	// no fork, only specifying IBFT type in chain config
	if originalType, ok := ibftConfig["type"].(string); ok {
		typ, err := ParseType(originalType)
		if err != nil {
			return nil, err
		}

		return []IBFTFork{
			{
				Type:       typ,
				Deployment: nil,
				From:       common.JSONNumber{Value: 0},
				To:         nil,
			},
		}, nil
	}

	// with forks
	if types, ok := ibftConfig["types"].([]interface{}); ok {
		bytes, err := json.Marshal(types)
		if err != nil {
			return nil, err
		}

		var forks []IBFTFork
		if err := json.Unmarshal(bytes, &forks); err != nil {
			return nil, err
		}

		return forks, nil
	}

	return nil, errors.New("current IBFT type not found")
}

// setupMechanism reads the current mechanism in params and sets up consensus mechanism
func (i *backendIBFT) setupMechanism() error {
	ibftForks, err := GetIBFTForks(i.config.Config)
	if err != nil {
		return err
	}

	i.mechanisms = make([]ConsensusMechanism, len(ibftForks))

	for idx, fork := range ibftForks {
		factory, ok := mechanismBackends[fork.Type]
		if !ok {
			return fmt.Errorf("consensus mechanism doesn't define: %s", fork.Type)
		}

		fork := fork
		if i.mechanisms[idx], err = factory(i, &fork); err != nil {
			return err
		}
	}

	return nil
}

// createKey sets the validator's private key from the secrets manager
func (i *backendIBFT) createKey() error {
	if i.validatorKey == nil {
		// Check if the validator key is initialized
		var key *ecdsa.PrivateKey

		if i.secretsManager.HasSecret(secrets.ValidatorKey) {
			// The validator key is present in the secrets manager, load it
			validatorKey, readErr := crypto.ReadConsensusKey(i.secretsManager)
			if readErr != nil {
				return fmt.Errorf("unable to read validator key from Secrets Manager, %w", readErr)
			}

			key = validatorKey
		} else {
			// The validator key is not present in the secrets manager, generate it
			validatorKey, validatorKeyEncoded, genErr := crypto.GenerateAndEncodePrivateKey()
			if genErr != nil {
				return fmt.Errorf("unable to generate validator key for Secrets Manager, %w", genErr)
			}

			// Save the key to the secrets manager
			saveErr := i.secretsManager.SetSecret(secrets.ValidatorKey, validatorKeyEncoded)
			if saveErr != nil {
				return fmt.Errorf("unable to save validator key to Secrets Manager, %w", saveErr)
			}

			key = validatorKey
		}

		i.validatorKey = key
		i.validatorKeyAddr = crypto.PubKeyToAddress(&key.PublicKey)
	}

	return nil
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
		for {
			if ev := <-newBlockSub.GetEventCh(); ev.Source == "syncer" {
				if ev.NewChain[0].Number < i.blockchain.Header().Number {
					// The blockchain notification system can eventually deliver
					// stale block notifications. These should be ignored
					continue
				}

				syncerBlockCh <- struct{}{}
			}
		}
	}()

	defer newBlockSub.Close()

	for {
		var (
			latest  = i.blockchain.Header().Number
			pending = latest + 1
		)

		i.updateActiveValidatorSet(latest)

		if !i.isActiveValidator() {
			// we are not participating in consensus for this height
			continue
		}

		select {
		case <-i.consensus.runSequence(pending):
			// consensus inserted block
			continue
		case <-syncerBlockCh:
			// syncer inserted block -> stop running consensus
			i.consensus.stopSequence()
			i.logger.Info("canceled sequence", "sequence", pending)
		case <-i.closeCh:
			// IBFT consensus stopped
			i.consensus.stopSequence()

			return
		}
	}
}

func (i *backendIBFT) isActiveValidator() bool {
	return i.activeValidatorSet.Includes(i.validatorKeyAddr)
}

func (i *backendIBFT) updateActiveValidatorSet(latestHeight uint64) {
	snap := i.getSnapshot(latestHeight)

	i.activeValidatorSet = snap.Set

	// Update the No.of validator metric
	i.metrics.Validators.Set(float64(len(snap.Set)))
}

// shouldWriteTransactions checks if each consensus mechanism accepts a block with transactions at given height
// returns true if all mechanisms accept
// otherwise return false
func (i *backendIBFT) shouldWriteTransactions(height uint64) bool {
	for _, m := range i.mechanisms {
		if m.ShouldWriteTransactions(height) {
			return true
		}
	}

	return false
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
		i.metrics.BlockInterval.Set(
			headerTime.Sub(parentTime).Seconds(),
		)
	}

	// Update the Number of transactions in the block metric
	i.metrics.NumTxs.Set(float64(len(block.Body().Transactions)))
}

var (
	errBlockVerificationFailed = errors.New("block verification fail")
)

// isSealing checks if the current node is sealing blocks
func (i *backendIBFT) isSealing() bool {
	return i.sealing
}

// verifyHeaderImpl implements the actual header verification logic
func (i *backendIBFT) verifyHeaderImpl(snap *Snapshot, parent, header *types.Header) error {
	// ensure the extra data is correctly formatted
	if _, err := getIbftExtra(header); err != nil {
		return err
	}

	if hookErr := i.runHook(VerifyHeadersHook, header.Number, header.Nonce); hookErr != nil {
		return hookErr
	}

	if header.MixHash != IstanbulDigest {
		return fmt.Errorf("invalid mixhash")
	}

	if header.Sha3Uncles != types.EmptyUncleHash {
		return fmt.Errorf("invalid sha3 uncles")
	}

	// difficulty has to match number
	if header.Difficulty != header.Number {
		return fmt.Errorf("wrong difficulty")
	}

	// verify the sealer
	if err := verifySigner(snap, header); err != nil {
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

	parentSnap := i.getSnapshot(parent.Number)
	if parentSnap == nil {
		return errParentSnapshotNotFound
	}

	// verify all the header fields + seal
	if err := i.verifyHeaderImpl(parentSnap, parent, header); err != nil {
		return err
	}

	// verify the committed seals
	if err := verifyCommittedFields(parentSnap, header, i.quorumSize(header.Number)); err != nil {
		return err
	}

	return nil
}

// quorumSize returns a callback that when executed on a ValidatorSet computes
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
	return i.processHeaders(headers)
}

// GetBlockCreator retrieves the block signer from the extra data field
func (i *backendIBFT) GetBlockCreator(header *types.Header) (types.Address, error) {
	return ecrecoverProposer(header)
}

// PreStateCommit a hook to be called before finalizing state transition on inserting block
func (i *backendIBFT) PreStateCommit(header *types.Header, txn *state.Transition) error {
	params := &preStateCommitHookParams{
		header: header,
		txn:    txn,
	}

	if err := i.runHook(PreStateCommitHook, header.Number, params); err != nil {
		return err
	}

	return nil
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

	if i.config.Path != "" {
		err := i.store.saveToPath(i.config.Path)

		if err != nil {
			return err
		}
	}

	if i.syncer != nil {
		if err := i.syncer.Close(); err != nil {
			return err
		}
	}

	return nil
}
