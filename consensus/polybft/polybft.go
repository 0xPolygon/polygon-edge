// Package polybft implements PBFT consensus algorithm integration and bridge feature
package polybft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/syncer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel"
)

const (
	DefaultEpochSize = 10
	minSyncPeers     = 2
	pbftProto        = "/pbft/0.2"
	bridgeProto      = "/bridge/0.2"
)

// polybftBackend is an interface defining polybft methods needed by fsm and sync tracker
type polybftBackend interface {
	// CheckIfStuck checks if state machine is stuck.
	CheckIfStuck(num uint64) (uint64, bool)

	// GetValidators retrieves validator set for the given block
	GetValidators(blockNumber uint64, parents []*types.Header) (AccountSet, error)
}

// Factory is the factory function to create a discovery consensus
func Factory(params *consensus.Params) (consensus.Consensus, error) {
	params.Logger.Info("polybft factory", "params", params.Config.Params, "specific consensus params", params.Config)

	polybft := &Polybft{
		config:  params,
		closeCh: make(chan struct{}),
		logger:  params.Logger,
	}

	return polybft, nil
}

type Polybft struct {
	// close closes all the pbft consensus
	closeCh chan struct{}

	// pbft is the pbft engine
	pbft *pbft.Pbft

	// state is reference to the struct which encapsulates consensus data persistence logic
	state *State

	// consensus parametres
	config *consensus.Params

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

	// topic for pbft consensus
	pbftTopic *network.Topic

	// topic for pbft consensus
	bridgeTopic *network.Topic

	// key encapsulates ECDSA address and BLS signing logic
	key *wallet.Key

	// validatorsCache represents cache of validators snapshots
	validatorsCache *validatorsSnapshotCache

	// logger
	logger hclog.Logger
}

// Initialize initializes the consensus (e.g. setup data)
func (p *Polybft) Initialize() error {
	p.logger.Info("initializing polybft")

	// read account
	account, err := wallet.GenerateAccountFromSecrets(p.config.SecretsManager)
	if err != nil {
		return fmt.Errorf("failed to read account data. Error: %v", err)
	}
	// set key
	p.key = wallet.NewKey(account)

	// create pbft topic
	pbftTopic, err := p.config.Network.NewTopic(pbftProto, &proto.GossipMessage{})
	if err != nil {
		return fmt.Errorf("failed to create pbft topic. Error: %v", err)
	}
	// set pbft topic
	p.pbftTopic = pbftTopic

	// create bridge topic
	bridgeTopic, err := p.config.Network.NewTopic(bridgeProto, &proto.GossipMessage{})
	if err != nil {
		return fmt.Errorf("failed to create bridge topic. Error: %v", err)
	}
	// set pbft topic
	p.bridgeTopic = bridgeTopic

	// create and set syncer
	p.syncer = syncer.NewSyncer(
		p.config.Logger,
		p.config.Network,
		p.config.Blockchain,
		time.Duration(p.config.BlockTime)*3*time.Second,
	)

	// set blockchain backend
	p.blockchain = &blockchainWrapper{
		blockchain: p.config.Blockchain,
		executor:   p.config.Executor,
	}

	// set block time  Nemanja - not sure if I am going to need it
	p.blockTime = time.Duration(p.config.BlockTime)

	//p.dataDir = node.ResolvePath("polybft")  Nemanja - what to do with this
	p.dataDir = "./polybft" // Nemanja - check this
	// create the data dir if not exists
	if err := os.MkdirAll(p.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory. Error: %v", err)
	}

	stt, err := newState(filepath.Join(p.dataDir, stateFileName))
	if err != nil {
		return fmt.Errorf("failed to create state instance. Error: %v", err)
	}

	p.state = stt
	p.validatorsCache = newValidatorsSnapshotCache(stt, DefaultEpochSize, p.blockchain)

	return nil
}

// Start starts the consensus and servers
func (p *Polybft) Start() error {
	p.logger.Info("starting consenzus")

	// Start the syncer
	if err := p.syncer.Start(); err != nil {
		return fmt.Errorf("failed to start syncer. Error: %v", err)
	}
	go func() {
		nullHandler := func(b *types.Block) bool {
			return false
		}
		if err := p.syncer.Sync(nullHandler); err != nil {
			panic(fmt.Errorf("failed to sync blocks. Error: %v", err))
		}
	}()

	// start consensus
	return p.StartSealing()
}

// StartSealing is executed if the PolyBFT protocol is running in sealing mode.
func (p *Polybft) StartSealing() error {
	p.logger.Info("Using signer", "address", p.key.String())

	// set miner
	p.blockchain.SetCoinbase(types.Address(p.key.Address()))

	// at this point the p2p server is running
	//p.transport = p.node.P2P()

	// run routine until close ch is notified
	go func() {
		// Nemanja - probably we do not need this
		// p.logger.Debug("Subscribing to chain head event...")
		// defer p.logger.Debug("Ending subscription to chain head event.")
		// chainHeadEventCh := make(chan core.ChainHeadEvent)
		// sub := p.blockchain.SubscribeChainHeadEvent(chainHeadEventCh)
		// defer sub.Unsubscribe()
		for {
			select {
			// case msg := <-chainHeadEventCh:
			// 	err := p.Publish(msg)
			// 	if err != nil {
			// 		p.logger.Warn("Error posting chain head event message", "error", err)
			// 	}
			case <-p.closeCh:
				return
			}
		}
	}()

	// Nemanja - no old sync tracker
	// start the sync tracker and let it run
	// p.syncTracker = &syncTracker{
	// 	closeCh:   p.closeCh,
	// 	lastBlock: p.blockchain.CurrentHeader(),
	// 	pubSub:    p,
	// 	isValidatorCallback: func(header *types.Header) bool {
	// 		snapshot, err := p.GetValidators(header.Number.Uint64(), nil)
	// 		if err != nil {
	// 			return false
	// 		}
	// 		return snapshot.ContainsNodeID(p.key.NodeID())
	// 	},
	// 	logger: p.logger.New("module", "sync-tracker"),
	// }
	// p.syncTracker.init()

	// create pbft at this point because we need to have the seal key
	// which is set in the command line
	opts := []pbft.ConfigOption{
		pbft.WithLogger(p.logger.Named("Pbft").StandardLogger(&hclog.StandardLoggerOptions{})),
		pbft.WithTracer(otel.Tracer("Pbft")),
	}
	pbftEngine := pbft.New(p.key, &pbftTransportWrapper{topic: p.pbftTopic}, opts...)

	// listen for transport messages and relay them to pbft
	err := p.pbftTopic.Subscribe(func(obj interface{}, from peer.ID) {
		gossipMsg := obj.(*proto.GossipMessage)

		var msg *pbft.MessageReq
		if err := json.Unmarshal(gossipMsg.Data, &msg); err != nil {
			panic(err)
		}

		pbftEngine.PushMessage(msg)
	})

	if err != nil {
		return fmt.Errorf("Topic subscription failed: %v", err)
	}

	p.pbft = pbftEngine

	if err := p.startRuntime(); err != nil {
		return fmt.Errorf("Runtime startup  failed: %v", err)
	}

	// Nemanja - do we need this?
	// Indicate that we are ready to accept transactions
	//p.eth.SetSynced()

	go func() {
		// start the pbft process
		p.startPbftProcess()
	}()

	// Nemanja - no dev logs for now
	// if p.DevPort != 0 {
	// 	go func() {
	// 		p.logger.Error("Debug toolkit started")
	// 		mux := http.NewServeMux()
	// 		mux.HandleFunc("/consensus", func(writer http.ResponseWriter, request *http.Request) {
	// 			writer.Header().Set("X-Content-Type-Options", "nosniff")
	// 			writer.Header().Set("Content-Type", "; charset=utf-8")

	// 			r := struct {
	// 				Locked   bool
	// 				State    string
	// 				Round    uint64
	// 				Proposal *pbft.Proposal
	// 			}{
	// 				Locked:   p.pbft.IsLocked(),
	// 				State:    p.pbft.GetState().String(),
	// 				Round:    p.pbft.Round(),
	// 				Proposal: p.pbft.GetProposal(),
	// 			}
	// 			err := json.NewEncoder(writer).Encode(r)
	// 			if err != nil {
	// 				p.logger.Warn("dump handler err", "err", err)
	// 			}
	// 		})
	// 		mux.HandleFunc("/consensus/state", pbft.ConsensusStateHandler(p.pbft))
	// 		mux.HandleFunc("/debug/pprof/", pprof.Index)
	// 		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	// 		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	// 		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	// 		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// 		p.logger.Error("Server", "err", http.ListenAndServe(":"+strconv.FormatUint(p.DevPort, 10), mux))
	// 	}()
	// }

	return nil
}

// startRuntime starts consensus runtime
func (p *Polybft) startRuntime() error {
	runtimeConfig := &runtimeConfig{
		PolyBFTConfig:  p.config.PolyBFT,
		Key:            p.key,
		DataDir:        p.dataDir,
		Transport:      &bridgeTransportWrapper{topic: p.bridgeTopic},
		State:          p.state,
		blockchain:     p.blockchain,
		polybftBackend: p,
	}

	runtime, err := newConsensusRuntime(runtimeConfig)
	if err != nil {
		return err
	}

	p.runtime = runtime

	if runtime.IsBridgeEnabled() {
		err := p.bridgeTopic.Subscribe(func(obj interface{}, from peer.ID) {
			msg := obj.(*TransportMessage)
			if _, err := p.runtime.deliverMessage(msg); err != nil {
				p.logger.Warn(fmt.Sprintf("Failed to deliver message. Error: %s", err))
			}
		})
		if err != nil {
			return fmt.Errorf("topic subscription failed:%w", err)
		}
	}

	return nil
}

func (p *Polybft) startPbftProcess() {
	// wait to have at least n peers connected. The 2 is just an initial heuristic value
	// Most likely we will parametrize this in the future.
	if !p.waitForNPeers() {
		return
	}

SYNC:
	p.runtime.setIsValidator(false)

	lastBlock := p.syncer.waitToStartValidating()
	if lastBlock == nil {
		// channel closed
		return
	}

	p.runtime.setIsValidator(true)

	// we have to start the bridge snapshot when we have finished syncing
	if err := p.runtime.restartEpoch(lastBlock); err != nil {
		p.logger.Error("failed to restart epoch", "error", err)
		goto SYNC
	}

	for {
		if err := p.runCycle(); err != nil {
			if errors.Is(err, errNotAValidator) {
				p.logger.Info("Node is no longer in validator set")
			} else {
				p.logger.Error("an error occurred while running a state machine cycle.", "error", err)
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

		num := p.config.Network.numPeers()
		if num >= minSyncPeers {
			break
		}
	}
	return true
}

// Close closes the connection
func (p *Polybft) Close() error {
	close(p.closeCh)
	return nil
}

// GetSyncProgression retrieves the current sync progression, if any
func (p *Polybft) GetSyncProgression() *progress.Progression {
	return p.syncer.GetSyncProgression()
}

// VerifyHeader implements consensus.Engine and checks whether a header conforms to the consensus rules
func (p *Polybft) VerifyHeader(header *types.Header) error {
	// Short circuit if the header is known, or its parent not
	header, ok := p.blockchain.GetHeader(header.Hash, header.Number)
	if !ok {
		return nil
	}

	parent, ok := p.blockchain.GetHeaderByNumber(header.Number - 1)
	if !ok {
		return fmt.Errorf(
			"unable to get parent header for block number %d",
			header.Number,
		)
	}

	return p.verifyHeaderImpl(parent, header, nil)
}

func (p *Polybft) verifyHeaderImpl(parent, header *types.Header, parents []*types.Header) error {
	blockNumber := header.Number
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
	extra, err := GetIbftExtra(header.ExtraData)
	if err != nil {
		return fmt.Errorf("failed to verify header for block %d. get extra error = %v", blockNumber, err)
	}
	if extra.Committed == nil {
		return fmt.Errorf(
			"failed to verify signatures for block %d because signatures are nil. Block hash: %v",
			blockNumber,
			header.Hash,
		)
	}
	if err := extra.Committed.VerifyCommittedFields(validators, header.Hash); err != nil {
		return fmt.Errorf("failed to verify signatures for block %d. Block hash: %v", blockNumber, header.Hash)
	}

	// validate the signatures for parent (skip block 1 because genesis does not have committed)
	if blockNumber > 1 {
		if extra.Parent == nil {
			return fmt.Errorf(
				"failed to verify signatures for parent of block %d because signatures are nil. Parent hash: %v",
				blockNumber,
				parent.Hash,
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
		if err := extra.Parent.VerifyCommittedFields(parentValidators, parent.Hash); err != nil {
			return fmt.Errorf("failed to verify signatures for parent of block %d. Parent hash: %v", blockNumber, parent.Hash)
		}
	}

	return nil
}

func (p *Polybft) CheckIfStuck(num uint64) (uint64, bool) {
	// TODO implement me
	panic("implement me")
}

func (p *Polybft) GetValidators(blockNumber uint64, parents []*types.Header) (AccountSet, error) {
	// TODO implement me
	panic("implement me")
}

// ProcessHeaders updates the snapshot based on the verified headers
func (p *Polybft) ProcessHeaders(_ []*types.Header) error {
	// Not required
	return nil
}

// GetBlockCreator retrieves the block creator (or signer) given the block header
func (p *Polybft) GetBlockCreator(_ *types.Header) (types.Address, error) {
	panic("TODO")
}

// PreCommitState a hook to be called before finalizing state transition on inserting block
func (p *Polybft) PreCommitState(_ *types.Header, _ *state.Transition) error {
	// Not required
	return nil
}

type pbftTransportWrapper struct {
	topic *network.Topic
}

func (p *pbftTransportWrapper) Gossip(msg *pbft.MessageReq) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	protoMsg := &proto.GossipMessage{
		Data: data,
	}
	return p.topic.Publish(protoMsg)
}

type bridgeTransportWrapper struct {
	topic  *network.Topic
	logger hclog.Logger
}

func (b *bridgeTransportWrapper) Gossip(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		b.logger.Warn(fmt.Sprintf("Failed to marshal bridge message:%s", err))
		return
	}
	protoMsg := &proto.GossipMessage{
		Data: data,
	}

	err = b.topic.Publish(protoMsg)
	if err != nil {
		b.logger.Warn(fmt.Sprintf("Failed to gossip bridge message:%s", err))
	}
}

var _ polybftBackend = &Polybft{}
