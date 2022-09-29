// Package polybft implements PBFT consensus algorithm integration and bridge feature
package polybft

import (
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/types"
)

// "github.com/ethereum/go-ethereum/common"
// "github.com/ethereum/go-ethereum/common/hexutil"
// "github.com/ethereum/go-ethereum/consensus"
// "github.com/ethereum/go-ethereum/core"
// "github.com/ethereum/go-ethereum/core/state"
// "github.com/ethereum/go-ethereum/core/types"
// "github.com/ethereum/go-ethereum/eth"
// "github.com/ethereum/go-ethereum/internal/account"
// "github.com/ethereum/go-ethereum/log"
// "github.com/ethereum/go-ethereum/node"
// "github.com/ethereum/go-ethereum/p2p"
// "github.com/ethereum/go-ethereum/params"
// "github.com/ethereum/go-ethereum/rpc"

const (
	minSyncPeers = 2
)

// polybftBackend is an interface defining polybft methods needed by fsm and sync tracker
type polybftBackend interface {
	// CheckIfStuck checks if state machine is stuck.
	CheckIfStuck(num uint64) (uint64, bool)

	// GetValidators retrieves validator set for the given block
	GetValidators(blockNumber uint64, parents []*types.Header) (AccountSet, error)
}

//var _ polybftBackend = &Polybft{}

// Factory is the factory function to create a discovery consensus
func Factory(params *consensus.Params) (consensus.Consensus, error) {
	/*
		topic, err := params.Network.NewTopic(pbftProto, &proto.GossipMessage{})
		if err != nil {
			return nil, err
		}

		// decode fixed validator set
		var validators []*validator
		fmt.Println(validators)

		fmt.Println(params.Config.Params)

		panic("x")

		syncer := syncer.NewSyncer(
			params.Logger,
			params.Network,
			params.Blockchain,
			time.Duration(params.BlockTime)*3*time.Second,
		)

		keyBytes, err := params.SecretsManager.GetSecret(secrets.ValidatorKey)
		if err != nil {
			return nil, err
		}
		ecdsaKey, err := crypto.BytesToECDSAPrivateKey(keyBytes)
		if err != nil {
			return nil, err
		}

		key := newSignKey(ecdsaKey)
		engine := pbft.New(key,
			&pbftTransport{topic: topic},
			pbft.WithLogger(params.Logger.Named("engine").StandardLogger(&hclog.StandardLoggerOptions{})),
		)

		// push messages to the pbft engine
		topic.Subscribe(func(obj interface{}, from peer.ID) {
			gossipMsg := obj.(*proto.GossipMessage)

			var msg *pbft.MessageReq
			if err := json.Unmarshal(gossipMsg.Data, &msg); err != nil {
				panic(err)
			}
			engine.PushMessage(msg)
		})

		polybft := &Polybft{
			pbftTopic:  topic,
			blockchain: params.Blockchain,
			blockTime:  time.Duration(params.BlockTime),
			syncer:     syncer,
			pbft:       engine,
			ecdsaKey:   ecdsaKey,
		}
		return polybft, nil
	*/
	return nil, nil
}

/*
// Polybft is a PBFT consensus protocol
type Polybft struct {
	// close closes all the pbft consensus
	closeCh chan struct{}

	// pbft is the pbft engine
	pbft *pbft.Pbft

	// state is reference to the struct which encapsulates consensus data persistence logic
	state *State

	// block time duration
	blockTime time.Duration

	// blockchain is a reference to the blockchain object
	blockchain blockchain

	// config is the configuration for polybft
	config *params.ChainConfig

	// key encapsulates ECDSA address and BLS signing logic
	key *key

	// runtime handles consensus runtime features like epoch, state and event management
	runtime *consensusRuntime

	// dataDir is the data directory to store the info
	dataDir string

	// validatorsCache represents cache of validators snapshots
	validatorsCache *validatorsSnapshotCache

	// Debug toolkit port
	DevPort         uint64
}


// VerifyHeader verifies the header is correct
func (p *Polybft) VerifyHeader(header *types.Header) error {
	// TOOD: Always verify
	return nil
}

// ProcessHeaders updates the snapshot based on the verified headers
func (p *Polybft) ProcessHeaders(headers []*types.Header) error {
	// Not required
	return nil
}

// GetBlockCreator retrieves the block creator (or signer) given the block header
func (p *Polybft) GetBlockCreator(header *types.Header) (types.Address, error) {
	panic("TODO")
}

// PreCommitState a hook to be called before finalizing state transition on inserting block
func (p *Polybft) PreCommitState(header *types.Header, txn *state.Transition) error {
	// Not required
	return nil
}

// GetSyncProgression retrieves the current sync progression, if any
func (p *Polybft) GetSyncProgression() *progress.Progression {
	return p.syncer.GetSyncProgression()
}

// Initialize initializes the consensus (e.g. setup data)
func (p *Polybft) Initialize() error {
	return nil
}

// Start starts the consensus and servers
func (p *Polybft) Start() error {
	// Start the syncer
	if err := p.syncer.Start(); err != nil {
		return err
	}
	go func() {
		nullHandler := func(b *types.Block) bool {
			return false
		}
		if err := p.syncer.Sync(nullHandler); err != nil {
			panic(err)
		}
	}()

	// start consensus
	for {
		sequence := p.blockchain.Header().Number + 1

		// setup consensus
		b := &backend{
			height:       sequence,
			consensus:    p,
			blockTime:    p.blockTime,
			validatorSet: p.validatorSet,
		}
		p.pbft.SetBackend(b)

		// run consensus
		p.pbft.Run(context.Background())
	}
}

// Close closes the connection
func (p *Polybft) Close() error {
	return nil
}

// New creates a new Polybft consensus protocol
func New(config *params.ChainConfig) *Polybft {
	config.TerminalTotalDifficulty = common.Big0 // this will allow us to always have CanonStatTy blocks
	p := &Polybft{
		closeCh: make(chan struct{}),
		config:  config,
	}

	return p
}

// Setup wires things up
// (such as setting external references to Ethereum protocol and blockchain, initializing state etc.)
func (p *Polybft) Setup(node *node.Node, eth *eth.Ethereum) {
	blockchain := &blockchainWrapper{
		blockchain: eth.BlockChain(),
		eth:        eth,
	}
	p.initPolybft(node, eth, blockchain)
	eth.GenesisGasLimit()
}

func (p *Polybft) initPolybft(node *node.Node, eth *eth.Ethereum, blockchain blockchain) {
	p.eth = eth
	p.node = node
	p.blockchain = blockchain

	p.dataDir = node.ResolvePath("polybft")
	// create the data dir if not exists
	if err := os.MkdirAll(p.dataDir, 0755); err != nil {
		panic(err)
	}

	stt, err := newState(filepath.Join(p.dataDir, stateFileName))
	if err != nil {
		panic(fmt.Errorf("failed to create state instance. Error: %v", err))
	}

	p.state = stt
	p.validatorsCache = newValidatorsSnapshotCache(stt, p.config.PolyBFT.EpochSize, p.blockchain)
}

type bridgeTransportWrapper struct {
	topic p2p.Topic
}

func (b *bridgeTransportWrapper) Gossip(msg interface{}) {
	err := b.topic.Publish(msg)
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to gossip message:%s", err))
	}
}

// StartSealing is executed if the PolyBFT protocol is running in sealing mode.
func (p *Polybft) StartSealing(account *account.Account) {

	// at this point the p2p server is running
	p.transport = p.node.P2P()

	pubKey, err := account.Bls.MarshalJSON()
	if err != nil {
		panic(err)
	}
	log.Info("Using signer", "address", account.Ecdsa.Address(), "bls key", hexutil.Encode(pubKey))
	p.key = newKey(account)

	go func() {
		log.Debug("Subscribing to chain head event...")
		defer log.Debug("Ending subscription to chain head event.")
		chainHeadEventCh := make(chan core.ChainHeadEvent)
		sub := p.blockchain.SubscribeChainHeadEvent(chainHeadEventCh)
		defer sub.Unsubscribe()
		for {
			select {
			case msg := <-chainHeadEventCh:
				err := p.Publish(msg)
				if err != nil {
					log.Warn("Error posting chain head event message", "error", err)
				}
			case <-p.closeCh:
				return
			}
		}
	}()

	p.blockchain.SetCoinbase(common.Address(p.key.Address()))

	// start the sync tracker and let it run
	p.syncTracker = &syncTracker{
		closeCh:   p.closeCh,
		lastBlock: p.blockchain.CurrentHeader(),
		pubSub:    p,
		isValidatorCallback: func(header *types.Header) bool {
			snapshot, err := p.GetValidators(header.Number.Uint64(), nil)
			if err != nil {
				return false
			}
			return snapshot.ContainsNodeID(p.key.NodeID())
		},
		logger: log.New("module", "sync-tracker"),
	}
	p.syncTracker.init()

	topic, err := p.transport.NewTopic("pbft/0.1", pbft.MessageReq{})
	if err != nil {
		panic(err)
	}

	// create pbft at this point because we need to have the seal key
	// which is set in the command line
	opts := []pbft.ConfigOption{
		pbft.WithLogger(log.Standard(log.New("module", "pbft-consensus"))),
		pbft.WithTracer(otel.Tracer("Pbft")),
	}
	p.pbft = pbft.New(p.key, &pbftTransportWrapper{topic: topic}, opts...)
	// listen for transport messages and relay them to pbft
	err = topic.Subscribe(func(obj interface{}) {
		msg := obj.(*pbft.MessageReq)
		p.pbft.PushMessage(msg)
	})

	if err != nil {
		panic(fmt.Sprintf("Topic subscription failed:%s", err))
	}

	if err := p.startRuntime(); err != nil {
		panic(err)
	}

	// Indicate that we are ready to accept transactions
	p.eth.SetSynced()

	go func() {
		// start the pbft process
		p.start()
	}()
	if p.DevPort != 0 {
		go func() {
			log.Error("Debug toolkit started")
			mux := http.NewServeMux()
			mux.HandleFunc("/consensus", func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("X-Content-Type-Options", "nosniff")
				writer.Header().Set("Content-Type", "; charset=utf-8")

				r := struct {
					Locked   bool
					State    string
					Round    uint64
					Proposal *pbft.Proposal
				}{
					Locked:   p.pbft.IsLocked(),
					State:    p.pbft.GetState().String(),
					Round:    p.pbft.Round(),
					Proposal: p.pbft.GetProposal(),
				}
				err := json.NewEncoder(writer).Encode(r)
				if err != nil {
					log.Warn("dump handler err", "err", err)
				}
			})
			mux.HandleFunc("/consensus/state", pbft.ConsensusStateHandler(p.pbft))
			mux.HandleFunc("/debug/pprof/", pprof.Index)
			mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

			log.Error("Server", "err", http.ListenAndServe(":"+strconv.FormatUint(p.DevPort, 10), mux))
		}()
	}
}

// startRuntime starts consensus runtime
func (p *Polybft) startRuntime() error {
	topic, err := p.transport.NewTopic("bridge/0.1", TransportMessage{})
	if err != nil {
		return err
	}

	config := &runtimeConfig{
		PolyBFTConfig:  p.config.PolyBFT,
		Key:            p.key,
		DataDir:        p.dataDir,
		Transport:      &bridgeTransportWrapper{topic: topic},
		State:          p.state,
		blockchain:     p.blockchain,
		polybftBackend: p,
	}

	runtime, err := newConsensusRuntime(config)
	if err != nil {
		return err
	}

	p.runtime = runtime

	if runtime.IsBridgeEnabled() {
		err := topic.Subscribe(func(obj interface{}) {
			msg := obj.(*TransportMessage)
			if _, err := p.runtime.deliverMessage(msg); err != nil {
				log.Warn(fmt.Sprintf("Failed to deliver message. Error: %s", err))
			}
		})
		if err != nil {
			return fmt.Errorf("topic subscription failed:%w", err)
		}
	}

	return nil
}

func (p *Polybft) start() {
	// wait to have at least n peers connected. The 2 is just an initial heuristic value
	// Most likely we will parametrize this in the future.
	if !p.waitForNPeers() {
		return
	}

SYNC:
	p.runtime.setIsValidator(false)

	lastBlock := p.syncTracker.waitToStartValidating()
	if lastBlock == nil {
		// channel closed
		return
	}

	p.runtime.setIsValidator(true)

	// we have to start the bridge snapshot when we have finished syncing
	if err := p.runtime.restartEpoch(lastBlock); err != nil {
		log.Error("failed to restart epoch", "error", err)
		goto SYNC
	}

	for {
		if err := p.runCycle(); err != nil {
			if errors.Is(err, errNotAValidator) {
				log.Info("Node is no longer in validator set")
			} else {
				log.Error("an error occurred while running a state machine cycle.", "error", err)
			}
			goto SYNC
		}

		switch p.pbft.GetState() {
		case pbft.SyncState:
			// we need to go back to sync
			goto SYNC
		case pbft.DoneState:
			// everything worked, move to the next iteration
		default:
			// stopped
			return
		}
	}
}

// runCycle runs a single cycle of the state machine and indicates if node should exit the consensus or keep on running
func (p *Polybft) runCycle() error {
	ff, err := p.runtime.FSM()
	if err != nil {
		return err
	}

	if err = p.pbft.SetBackend(ff); err != nil {
		return err
	}

	// this cancel is not sexy
	ctx, cancelFn := context.WithCancel(context.Background())
	go func() {
		<-p.closeCh
		cancelFn()
	}()

	p.pbft.Run(ctx)

	return nil
}

func (p *Polybft) waitForNPeers() bool {
	for {
		select {
		case <-p.closeCh:
			return false
		case <-time.After(2 * time.Second):
		}

		num := p.blockchain.PeersLen()
		if num >= minSyncPeers {
			break
		}
	}
	return true
}

// CheckIfStuck is an implementation of blockchain interface
func (p *Polybft) CheckIfStuck(num uint64) (uint64, bool) {
	isSyncing := p.syncTracker.isSyncing()
	if isSyncing {
		// we are currently syncing new data, for sure we are stuck.
		// We can return 0 here at least for now since that value is only used
		// for the open telemetry tracing.
		return 0, isSyncing
	}

	// Now, we have to check if the current value of the round 'num' is lower
	// than our currently synced block.
	currentHeader := p.blockchain.CurrentHeader().Number.Uint64()
	if currentHeader > num {
		// at this point, it will exit the sync process and start the fsm round again
		// (or sync a small number of blocks) to start from the correct position.
		return currentHeader, true
	}
	return 0, false
}

// GetValidators retrieves a snapshot for the given block number
func (p *Polybft) GetValidators(blockNumber uint64, parents []*types.Header) (AccountSet, error) {
	validatorsSnapshot, err := p.validatorsCache.GetSnapshot(blockNumber, parents)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve validators snapshot for block %d: %v", blockNumber, err)
	}
	log.Trace("GetValidators", "Snapshot", validatorsSnapshot)
	return validatorsSnapshot, nil
}

// Author implements consensus.Engine
func (p *Polybft) Author(header *types.Header) (common.Address, error) {
	// This is needed in EVM to set the coinbase receiver of fees.
	return header.Coinbase, nil
}

// Prepare implements consensus.Engine
func (p *Polybft) Prepare(_ consensus.ChainHeaderReader, header *types.Header) error {
	// this is ONLY being called by the miner to do some consensus related block building of the header.
	// For example, add the difficulty in PoW protocol. Once we move away from miner for pbft we
	// can panic here again.
	return nil
}

// Finalize implements consensus.Engine
func (p *Polybft) Finalize(
	_ consensus.ChainHeaderReader,
	header *types.Header,
	state *state.StateDB,
	txs []*types.Transaction,
	uncles []*types.Header,
) {
	// this is called after the block transactions have been processed to do some extra steps
	// (i.e. difficulty payment in ethash).
	// we do not need this step anymore implemented because that core logic will be done in this consensus module

	// TODO: This should not go in here. This is due to the current workflow in Geth.
	// In the future, anything that changes the state will be modeled as a transaction or some state transition
	// state.AddBalance(header.Coinbase, big.NewInt(header.Number.Int64()))
}

// VerifyHeader implements consensus.Engine and checks whether a header conforms to the consensus rules
func (p *Polybft) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, seal bool) error {
	// Short circuit if the header is known, or its parent not
	number := header.Number.Uint64()
	if chain.GetHeader(header.Hash(), number) != nil {
		return nil
	}

	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	return p.verifyHeaderImpl(parent, header, nil)
}

func (p *Polybft) verifyHeaderImpl(parent, header *types.Header, parents []*types.Header) error {
	blockNumber := header.Number.Uint64()
	if blockNumber == 0 {
		// TODO: Remove, this was just for simplicity since I had started the chain already,
		//  add the mix hash into the genesis command
		return nil
	}

	// validate header fields
	if err := validateHeaderFields(parent, header); err != nil {
		return fmt.Errorf("failed to validate header for block %d. error = %v", blockNumber, err)
	}

	validators, err := p.GetValidators(blockNumber-1, parents)
	if err != nil {
		return fmt.Errorf("failed to validate header for block %d. could not retrieve block validators:%w", blockNumber, err)
	}

	// decode the extra field and validate the signatures
	extra, err := GetIbftExtra(header.Extra)
	if err != nil {
		return fmt.Errorf("failed to verify header for block %d. get extra error = %v", blockNumber, err)
	}
	if extra.Committed == nil {
		return fmt.Errorf(
			"failed to verify signatures for block %d because signatures are nil. Block hash: %v",
			blockNumber,
			header.Hash(),
		)
	}
	if err := extra.Committed.VerifyCommittedFields(validators, header.Hash()); err != nil {
		return fmt.Errorf("failed to verify signatures for block %d. Block hash: %v", blockNumber, header.Hash())
	}

	// validate the signatures for parent (skip block 1 because genesis does not have committed)
	if blockNumber > 1 {
		if extra.Parent == nil {
			return fmt.Errorf(
				"failed to verify signatures for parent of block %d because signatures are nil. Parent hash: %v",
				blockNumber,
				parent.Hash(),
			)
		}
		parentValidators, err := p.GetValidators(blockNumber-2, parents)
		if err != nil {
			return fmt.Errorf(
				"failed to validate header for block %d. could not retrieve parent validators:%w",
				blockNumber,
				err,
			)
		}
		if err := extra.Parent.VerifyCommittedFields(parentValidators, parent.Hash()); err != nil {
			return fmt.Errorf("failed to verify signatures for parent of block %d. Parent hash: %v", blockNumber, parent.Hash())
		}
	}

	return nil
}

// VerifyUncles implements consensus.Engine. Not of interest, since polybft is instant finality consensus engine.
func (p *Polybft) VerifyUncles(_ consensus.ChainReader, block *types.Block) error {
	// we do not have uncles, return nil
	return nil
}

// VerifyHeaders implements consensus.Engine and is similar to VerifyHeader,
// but verifies a batch of headers concurrently.
func (p *Polybft) VerifyHeaders(
	reader consensus.ChainHeaderReader,
	headers []*types.Header,
	seals []bool,
) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	go func() {
		for index, header := range headers {
			var parent *types.Header
			if index == 0 {
				parent = reader.GetHeader(headers[0].ParentHash, headers[0].Number.Uint64()-1)
			} else if headers[index-1].Hash() == headers[index].ParentHash {
				parent = headers[index-1]
			}

			var err error
			if parent == nil {
				err = consensus.ErrUnknownAncestor
			} else {
				err = p.verifyHeaderImpl(parent, header, headers)
			}

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()

	return abort, results
}

// APIs implements consensus.Engine, returning the user facing RPC APIs.
func (p *Polybft) APIs(_ consensus.ChainHeaderReader) []rpc.API {
	return []rpc.API{}
}

// Close shutdowns the consensus engine
func (p *Polybft) Close() error {
	close(p.closeCh)
	return nil
}

// Publish posts a message to the internal event hub
func (p *Polybft) Publish(msg interface{}) error {
	return p.eth.EventMux().Post(msg)
}

// Subscribe subscribes to messages on the internal event hub
func (p *Polybft) Subscribe(typ ...interface{}) Subscription {
	return p.eth.EventMux().Subscribe(typ...)
}

type key struct {
	raw *account.Account
}

func newKey(raw *account.Account) *key {
	k := &key{
		raw: raw,
	}
	return k
}

func (k *key) String() string {
	return k.raw.Ecdsa.Address().String()
}

func (k *key) Address() ethgo.Address {
	return k.raw.Ecdsa.Address()
}

func (k *key) NodeID() pbft.NodeID {
	return pbft.NodeID(k.String())
}

func (k *key) Sign(b []byte) ([]byte, error) {
	s, err := k.raw.Bls.Sign(b)
	if err != nil {
		return nil, err
	}
	return s.Marshal()
}

// wrapper for the pbft transport on top of rlpx transport
type pbftTransportWrapper struct {
	topic p2p.Topic
}

func (p *pbftTransportWrapper) Gossip(msg *pbft.MessageReq) error {
	log.Trace("Gossip", "from", msg.From, "view", msg.View, "type", msg.Type.String())
	return p.topic.Publish(msg)
}
*/
