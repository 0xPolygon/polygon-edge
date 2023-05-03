// Package polybft implements PBFT consensus algorithm integration and bridge feature
package polybft

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
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
	// closeCh is used to signal that consensus protocol is stopped
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

		// initialize ValidatorSet SC
		input, err := getInitValidatorSetInput(polyBFTConfig)
		if err != nil {
			return err
		}

		if err = initContract(contracts.SystemCaller,
			contracts.ValidatorSetContract, input, "ValidatorSet", transition); err != nil {
			return err
		}

		if err = mintRewardTokensToWalletAddress(&polyBFTConfig, transition); err != nil {
			return err
		}

		// initialize RewardPool SC
		input, err = getInitRewardPoolInput(polyBFTConfig)
		if err != nil {
			return err
		}

		if err = initContract(contracts.SystemCaller,
			contracts.RewardPoolContract, input, "RewardPool", transition); err != nil {
			return err
		}

		// initialize Predicate SCs
		if polyBFTConfig.BridgeAllowListAdmin != types.ZeroAddress ||
			polyBFTConfig.BridgeBlockListAdmin != types.ZeroAddress {
			input, err = getInitChildERC20PredicateAccessListInput(polyBFTConfig)
			if err != nil {
				return err
			}

			if err = initContract(contracts.SystemCaller, contracts.ChildERC20PredicateContract, input,
				"ChildERC20PredicateAccessList", transition); err != nil {
				return err
			}

			input, err = getInitChildERC721PredicateAccessListInput(polyBFTConfig)
			if err != nil {
				return err
			}

			if err = initContract(contracts.SystemCaller, contracts.ChildERC721PredicateContract, input,
				"ChildERC721PredicateAccessList", transition); err != nil {
				return err
			}

			input, err = getInitChildERC1155PredicateAccessListInput(polyBFTConfig)
			if err != nil {
				return err
			}

			if err = initContract(contracts.SystemCaller, contracts.ChildERC1155PredicateContract, input,
				"ChildERC1155PredicateAccessList", transition); err != nil {
				return err
			}
		} else {
			input, err = getInitChildERC20PredicateInput(polyBFTConfig.Bridge)
			if err != nil {
				return err
			}

			if err = initContract(contracts.SystemCaller, contracts.ChildERC20PredicateContract, input,
				"ChildERC20Predicate", transition); err != nil {
				return err
			}

			// initialize ChildERC721Predicate SC
			input, err = getInitChildERC721PredicateInput(polyBFTConfig.Bridge)
			if err != nil {
				return err
			}

			if err = initContract(contracts.SystemCaller, contracts.ChildERC721PredicateContract, input,
				"ChildERC721Predicate", transition); err != nil {
				return err
			}

			// initialize ChildERC1155Predicate SC
			input, err = getInitChildERC1155PredicateInput(polyBFTConfig.Bridge)
			if err != nil {
				return err
			}

			if err = initContract(contracts.SystemCaller, contracts.ChildERC1155PredicateContract, input,
				"ChildERC1155Predicate", transition); err != nil {
				return err
			}
		}

		rootNativeERC20Token := types.ZeroAddress
		if polyBFTConfig.Bridge != nil {
			rootNativeERC20Token = polyBFTConfig.Bridge.RootNativeERC20Addr
		}

		if polyBFTConfig.MintableNativeToken {
			// initialize NativeERC20Mintable SC
			params := &contractsapi.InitializeNativeERC20MintableFn{
				Predicate_: contracts.ChildERC20PredicateContract,
				Owner_:     polyBFTConfig.Governance,
				RootToken_: rootNativeERC20Token,
				Name_:      polyBFTConfig.NativeTokenConfig.Name,
				Symbol_:    polyBFTConfig.NativeTokenConfig.Symbol,
				Decimals_:  polyBFTConfig.NativeTokenConfig.Decimals,
			}

			input, err := params.EncodeAbi()
			if err != nil {
				return err
			}

			if err = initContract(contracts.SystemCaller,
				contracts.NativeERC20TokenContract, input, "NativeERC20Mintable", transition); err != nil {
				return err
			}
		} else {
			// initialize NativeERC20 SC
			params := &contractsapi.InitializeNativeERC20Fn{
				Name_:      polyBFTConfig.NativeTokenConfig.Name,
				Symbol_:    polyBFTConfig.NativeTokenConfig.Symbol,
				Decimals_:  polyBFTConfig.NativeTokenConfig.Decimals,
				RootToken_: rootNativeERC20Token,
				Predicate_: contracts.ChildERC20PredicateContract,
			}

			input, err := params.EncodeAbi()
			if err != nil {
				return err
			}

			if err = initContract(contracts.SystemCaller,
				contracts.NativeERC20TokenContract, input, "NativeERC20", transition); err != nil {
				return err
			}
		}

		return nil
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
	if err = p.createTopics(); err != nil {
		return fmt.Errorf("cannot create topics: %w", err)
	}

	// set block time
	p.blockTime = time.Duration(p.config.BlockTime)

	// initialize polybft consensus data directory
	p.dataDir = filepath.Join(p.config.Config.Path, "polybft")
	// create the data dir if not exists
	if err = common.CreateDirSafe(p.dataDir, 0750); err != nil {
		return fmt.Errorf("failed to create data directory. Error: %w", err)
	}

	stt, err := newState(filepath.Join(p.dataDir, stateFileName), p.logger, p.closeCh)
	if err != nil {
		return fmt.Errorf("failed to create state instance. Error: %w", err)
	}

	p.state = stt
	p.validatorsCache = newValidatorsSnapshotCache(p.config.Logger, stt, p.blockchain)

	// create runtime
	if err := p.initRuntime(); err != nil {
		return err
	}

	p.ibft = newIBFTConsensusWrapper(p.logger, p.runtime, p)

	if err = p.subscribeToIbftTopic(); err != nil {
		return fmt.Errorf("IBFT topic subscription failed: %w", err)
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

	// start syncing
	go func() {
		blockHandler := func(b *types.FullBlock) bool {
			p.runtime.OnBlockInserted(b)

			return false
		}

		if err := p.syncer.Sync(blockHandler); err != nil {
			p.logger.Error("blocks synchronization failed", "error", err)
		}
	}()

	// start consensus runtime
	if err := p.startRuntime(); err != nil {
		return fmt.Errorf("consensus runtime start failed: %w", err)
	}

	// start state DB process
	go p.state.startStatsReleasing()

	return nil
}

// initRuntime creates consensus runtime
func (p *Polybft) initRuntime() error {
	runtimeConfig := &runtimeConfig{
		PolyBFTConfig:         p.consensusConfig,
		Key:                   p.key,
		DataDir:               p.dataDir,
		State:                 p.state,
		blockchain:            p.blockchain,
		polybftBackend:        p,
		txPool:                p.txPool,
		bridgeTopic:           p.bridgeTopic,
		numBlockConfirmations: p.config.NumBlockConfirmations,
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
	go p.startConsensusProtocol()

	return nil
}

func (p *Polybft) startConsensusProtocol() {
	// wait to have at least n peers connected. The 2 is just an initial heuristic value
	// Most likely we will parametrize this in the future.
	if !p.waitForNPeers() {
		return
	}

	p.logger.Debug("peers connected")

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
				if ev.Source == "syncer" && ev.NewChain[0].Number >= p.blockchain.CurrentHeader().Number {
					p.logger.Info("sync block notification received", "block height", ev.NewChain[0].Number,
						"current height", p.blockchain.CurrentHeader().Number)
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

		now := time.Now().UTC()

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

		p.logger.Debug("time to run the sequence", "seconds", time.Since(now))
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
	p.runtime.close()

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
	// validate header fields
	if err := validateHeaderFields(parent, header); err != nil {
		return fmt.Errorf("failed to validate header for block %d. error = %w", header.Number, err)
	}

	// decode the extra data
	extra, err := GetIbftExtra(header.ExtraData)
	if err != nil {
		return fmt.Errorf("failed to verify header for block %d. get extra error = %w", header.Number, err)
	}

	// validate extra data
	return extra.ValidateFinalizedData(
		header, parent, parents, p.blockchain.GetChainID(), p, bls.DomainCheckpointManager, p.logger)
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

// GetBridgeProvider is an implementation of Consensus interface
// Returns an instance of BridgeDataProvider
func (p *Polybft) GetBridgeProvider() consensus.BridgeDataProvider {
	return p.runtime
}

// GetBridgeProvider is an implementation of Consensus interface
// Filters extra data to not contain Committed field
func (p *Polybft) FilterExtra(extra []byte) ([]byte, error) {
	return GetIbftExtraClean(extra)
}
