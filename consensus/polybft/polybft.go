// Package polybft implements PBFT consensus algorithm integration and bridge feature
package polybft

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/syncer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	minSyncPeers = 2
	pbftProto    = "/pbft/0.2"
	bridgeProto  = "/bridge/0.2"
)

// polybftBackend is an interface defining polybft methods needed by fsm and sync tracker
type polybftBackend interface {
	// GetValidators retrieves validator set for the given block
	GetValidators(blockNumber uint64, parents []*types.Header) (AccountSet, error)
}

// Factory is the factory function to create a discovery consensus
func Factory(params *consensus.Params) (consensus.Consensus, error) {
	logger := params.Logger.Named("polybft")

	setupHeaderHashFunc()

	polybft := &Polybft{
		config:  params,
		closeCh: make(chan struct{}),
		logger:  logger,
		txPool:  params.TxPool,
	}

	// initialize polybft consensus config
	customConfigJSON, err := json.Marshal(params.Config.Config)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(customConfigJSON, &polybft.consensusConfig)
	if err != nil {
		return nil, err
	}

	return polybft, nil
}

type Polybft struct {
	// close closes all the pbft consensus
	closeCh chan struct{}

	// ibft is the ibft engine
	ibft *IBFTConsensusWrapper

	// state is reference to the struct which encapsulates consensus data persistence logic
	state *State

	// consensus parametres
	config *consensus.Params

	// consensusConfig is genesis configuration for polybft consensus protocol
	consensusConfig *PolyBFTConfig

	// blockchain is a reference to the blockchain object
	blockchain blockchainBackend

	// runtime handles consensus runtime features like epoch, state and event management
	runtime *consensusRuntime

	// block time duration
	blockTime time.Duration

	// dataDir is the data directory to store the info
	dataDir string

	// reference to the syncer
	syncer syncer.Syncer

	// topic for consensus engine messages
	consensusTopic *network.Topic

	// topic for bridge messages
	bridgeTopic *network.Topic

	// key encapsulates ECDSA address and BLS signing logic
	key *wallet.Key

	// validatorsCache represents cache of validators snapshots
	validatorsCache *validatorsSnapshotCache

	// logger
	logger hclog.Logger

	// tx pool as interface
	txPool txPoolInterface
}

func GenesisPostHookFactory(config *chain.Chain, engineName string) func(txn *state.Transition) error {
	return func(transition *state.Transition) error {
		polyBFTConfig, err := GetPolyBFTConfig(config)
		if err != nil {
			return err
		}

		// Initialize child validator set
		input, err := getInitChildValidatorSetInput(polyBFTConfig)
		if err != nil {
			return err
		}

		if err = initContract(contracts.ValidatorSetContract, input, "ChildValidatorSet", transition); err != nil {
			return err
		}

		input, err = nativeTokenInitializer.Encode(
			[]interface{}{helper.GetRootchainAdminAddr(), nativeTokenName, nativeTokenSymbol})
		if err != nil {
			return err
		}

		return initContract(contracts.NativeTokenContract, input, "MRC20", transition)
	}
}

// Initialize initializes the consensus (e.g. setup data)
func (p *Polybft) Initialize() error {
	p.logger.Info("initializing polybft...")

	// read account
	account, err := wallet.NewAccountFromSecret(p.config.SecretsManager)
	if err != nil {
		return fmt.Errorf("failed to read account data. Error: %w", err)
	}

	// set key
	p.key = wallet.NewKey(account)

	// create and set syncer
	p.syncer = syncer.NewSyncer(
		p.config.Logger.Named("syncer"),
		p.config.Network,
		p.config.Blockchain,
		time.Duration(p.config.BlockTime)*3*time.Second,
	)

	// set blockchain backend
	p.blockchain = &blockchainWrapper{
		blockchain: p.config.Blockchain,
		executor:   p.config.Executor,
	}

	// create bridge and consensus topics
	if err := p.createTopics(); err != nil {
		return fmt.Errorf("cannot create topics: %w", err)
	}

	// set block time
	p.blockTime = time.Duration(p.config.BlockTime)

	// initialize polybft consensus data directory
	p.dataDir = filepath.Join(p.config.Config.Path, "polybft")
	// create the data dir if not exists
	if err := os.MkdirAll(p.dataDir, 0750); err != nil {
		return fmt.Errorf("failed to create data directory. Error: %w", err)
	}

	stt, err := newState(filepath.Join(p.dataDir, stateFileName), p.logger)
	if err != nil {
		return fmt.Errorf("failed to create state instance. Error: %w", err)
	}

	p.state = stt
	p.validatorsCache = newValidatorsSnapshotCache(p.config.Logger, stt, p.consensusConfig.EpochSize, p.blockchain)

	// create runtime
	p.initRuntime()

	p.ibft = newIBFTConsensusWrapper(p.logger, p.runtime, p)

	if err := p.subscribeToIbftTopic(); err != nil {
		return fmt.Errorf("topic subscription failed: %w", err)
	}

	return nil
}

// Start starts the consensus and servers
func (p *Polybft) Start() error {
	p.logger.Info("starting polybft consensus", "signer", p.key.String())

	// start syncer (also initializes peer map)
	if err := p.syncer.Start(); err != nil {
		return fmt.Errorf("failed to start syncer. Error: %w", err)
	}

	// we need to call restart epoch on runtime to initialize epoch state
	if err := p.runtime.restartEpoch(p.blockchain.CurrentHeader()); err != nil {
		return fmt.Errorf("consensus runtime start - restart epoch failed: %w", err)
	}

	// start syncing
	go func() {
		blockHandler := func(b *types.Block) bool {
			p.runtime.OnBlockInserted(b)

			return false
		}

		if err := p.syncer.Sync(blockHandler); err != nil {
			panic(fmt.Errorf("failed to sync blocks. Error: %w", err))
		}
	}()

	// start pbft process
	if err := p.startRuntime(); err != nil {
		return fmt.Errorf("consensus runtime start failed: %w", err)
	}

	return nil
}

// initRuntime creates consensus runtime
func (p *Polybft) initRuntime() error {
	runtimeConfig := &runtimeConfig{
		PolyBFTConfig:   p.consensusConfig,
		Key:             p.key,
		DataDir:         p.dataDir,
		BridgeTransport: &runtimeTransportWrapper{p.bridgeTopic, p.logger},
		State:           p.state,
		blockchain:      p.blockchain,
		polybftBackend:  p,
		txPool:          p.txPool,
	}

	runtime, err := newConsensusRuntime(p.logger, runtimeConfig)
	if err != nil {
		return err
	}

	p.runtime = runtime

	return nil
}

// startRuntime starts consensus runtime
func (p *Polybft) startRuntime() error {
	if p.runtime.IsBridgeEnabled() {
		// start bridge event tracker
		if err := p.runtime.startEventTracker(); err != nil {
			return fmt.Errorf("starting event tracker  failed: %w", err)
		}

		// subscribe to bridge topic
		if err := p.runtime.subscribeToBridgeTopic(p.bridgeTopic); err != nil {
			return fmt.Errorf("bridge topic subscription failed: %w", err)
		}
	}

	go p.startPbftProcess()

	return nil
}

func (p *Polybft) startPbftProcess() {
	// wait to have at least n peers connected. The 2 is just an initial heuristic value
	// Most likely we will parametrize this in the future.
	if !p.waitForNPeers() {
		return
	}

	newBlockSub := p.blockchain.SubscribeEvents()
	defer newBlockSub.Close()

	syncerBlockCh := make(chan struct{})

	go func() {
		eventCh := newBlockSub.GetEventCh()

		for {
			select {
			case <-p.closeCh:
				return
			case ev := <-eventCh:
				// The blockchain notification system can eventually deliver
				// stale block notifications. These should be ignored
				if ev.Source == "syncer" && ev.NewChain[0].Number > p.blockchain.CurrentHeader().Number {
					syncerBlockCh <- struct{}{}
				}
			}
		}
	}()

	var (
		sequenceCh   <-chan struct{}
		stopSequence func()
	)

	for {
		latestHeader := p.blockchain.CurrentHeader()

		currentValidators, err := p.GetValidators(latestHeader.Number, nil)
		if err != nil {
			p.logger.Error("failed to query current validator set", "block number", latestHeader.Number, "error", err)
		}

		isValidator := currentValidators.ContainsNodeID(p.key.String())
		p.runtime.setIsActiveValidator(isValidator)

		p.txPool.SetSealing(isValidator) // update tx pool

		if isValidator {
			// initialze FSM as a stateless ibft backend via runtime as an adapter
			err = p.runtime.FSM()
			if err != nil {
				p.logger.Error("failed to create fsm", "block number", latestHeader.Number, "error", err)

				continue
			}

			sequenceCh, stopSequence = p.ibft.runSequence(latestHeader.Number + 1)
		}

		select {
		case <-syncerBlockCh:
			if isValidator {
				stopSequence()
				p.logger.Info("canceled sequence", "sequence", latestHeader.Number+1)
			}
		case <-sequenceCh:
		case <-p.closeCh:
			if isValidator {
				stopSequence()
			}

			return
		}
	}
}

func (p *Polybft) waitForNPeers() bool {
	for {
		select {
		case <-p.closeCh:
			return false
		case <-time.After(2 * time.Second):
		}

		if len(p.config.Network.Peers()) >= minSyncPeers {
			break
		}
	}

	return true
}

// Close closes the connection
func (p *Polybft) Close() error {
	if p.syncer != nil {
		if err := p.syncer.Close(); err != nil {
			return err
		}
	}

	close(p.closeCh)

	return nil
}

// GetSyncProgression retrieves the current sync progression, if any
func (p *Polybft) GetSyncProgression() *progress.Progression {
	return p.syncer.GetSyncProgression()
}

// VerifyHeader implements consensus.Engine and checks whether a header conforms to the consensus rules
func (p *Polybft) VerifyHeader(header *types.Header) error {
	// Short circuit if the header is known
	if _, ok := p.blockchain.GetHeaderByHash(header.Hash); ok {
		return nil
	}

	parent, ok := p.blockchain.GetHeaderByHash(header.ParentHash)
	if !ok {
		return fmt.Errorf(
			"unable to get parent header by hash for block number %d",
			header.Number,
		)
	}

	return p.verifyHeaderImpl(parent, header, nil)
}

func (p *Polybft) verifyHeaderImpl(parent, header *types.Header, parents []*types.Header) error {
	blockNumber := header.Number

	// validate header fields
	if err := validateHeaderFields(parent, header); err != nil {
		return fmt.Errorf("failed to validate header for block %d. error = %w", blockNumber, err)
	}

	validators, err := p.GetValidators(blockNumber-1, parents)
	if err != nil {
		return fmt.Errorf("failed to validate header for block %d. could not retrieve block validators:%w", blockNumber, err)
	}

	// decode the extra field and validate the signatures
	extra, err := GetIbftExtra(header.ExtraData)
	if err != nil {
		return fmt.Errorf("failed to verify header for block %d. get extra error = %w", blockNumber, err)
	}

	if extra.Committed == nil {
		return fmt.Errorf("failed to verify signatures for block %d because signatures are not present", blockNumber)
	}

	checkpointHash, err := extra.Checkpoint.Hash(p.blockchain.GetChainID(), header.Number, header.Hash)
	if err != nil {
		return fmt.Errorf("failed to calculate sign hash: %w", err)
	}

	if err := extra.Committed.VerifyCommittedFields(validators, checkpointHash); err != nil {
		return fmt.Errorf("failed to verify signatures for block %d. Signed hash %v: %w",
			blockNumber, checkpointHash, err)
	}

	// validate the signatures for parent (skip block 1 because genesis does not have committed)
	if blockNumber > 1 {
		if extra.Parent == nil {
			return fmt.Errorf("failed to verify signatures for parent of block %d because signatures are not present",
				blockNumber)
		}

		parentValidators, err := p.GetValidators(blockNumber-2, parents)
		if err != nil {
			return fmt.Errorf(
				"failed to validate header for block %d. could not retrieve parent validators: %w",
				blockNumber,
				err,
			)
		}

		parentExtra, err := GetIbftExtra(parent.ExtraData)
		if err != nil {
			return err
		}

		parentCheckpointHash, err := parentExtra.Checkpoint.Hash(p.blockchain.GetChainID(), parent.Number, parent.Hash)
		if err != nil {
			return fmt.Errorf("failed to calculate parent block sign hash: %w", err)
		}

		if err := extra.Parent.VerifyCommittedFields(parentValidators, parentCheckpointHash); err != nil {
			return fmt.Errorf("failed to verify signatures for parent of block %d. Signed hash: %v: %w",
				blockNumber, parentCheckpointHash, err)
		}
	}

	return nil
}

func (p *Polybft) GetValidators(blockNumber uint64, parents []*types.Header) (AccountSet, error) {
	return p.validatorsCache.GetSnapshot(blockNumber, parents)
}

// ProcessHeaders updates the snapshot based on the verified headers
func (p *Polybft) ProcessHeaders(_ []*types.Header) error {
	// Not required
	return nil
}

// GetBlockCreator retrieves the block creator (or signer) given the block header
func (p *Polybft) GetBlockCreator(h *types.Header) (types.Address, error) {
	return types.BytesToAddress(h.Miner), nil
}

// PreCommitState a hook to be called before finalizing state transition on inserting block
func (p *Polybft) PreCommitState(_ *types.Header, _ *state.Transition) error {
	// Not required
	return nil
}

// GetBridgeProvider returns an instance of BridgeDataProvider
func (p *Polybft) GetBridgeProvider() consensus.BridgeDataProvider {
	return p.runtime
}
