// Package polybft implements PBFT consensus algorithm integration and bridge feature
package polybft

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"time"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/syncer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	minSyncPeers = 2
	pbftProto    = "/pbft/0.2"
)

type signKey struct {
	key  *ecdsa.PrivateKey
	addr pbft.NodeID
}

func newSignKey(key *ecdsa.PrivateKey) *signKey {
	addr, err := crypto.GetAddressFromKey(key)
	if err != nil {
		panic(fmt.Errorf("BUG: %v", err))
	}
	return &signKey{key: key, addr: pbft.NodeID(addr.String())}
}

func (s *signKey) NodeID() pbft.NodeID {
	return s.addr
}

func (s *signKey) Sign(b []byte) ([]byte, error) {
	return crypto.Sign(s.key, b)
}

type validator struct {
	PubKey *ecdsa.PublicKey
	Addr   types.Address
}

func (v *validator) UnmarshalJSON(buf []byte) error {
	var obj struct {
		PubKey string `json:"pubKey"`
	}
	if err := json.Unmarshal(buf, &obj); err != nil {
		return err
	}

	decoded, err := hex.DecodeString(obj.PubKey)
	if err != nil {
		return err
	}
	pub, err := crypto.ParsePublicKey(decoded)
	if err != nil {
		return err
	}
	v.PubKey = pub

	addr, err := crypto.GetAddressFromKey(pub)
	if err != nil {
		return err
	}
	v.Addr = addr
	return nil
}

// Factory is the factory function to create a discovery consensus
func Factory(params *consensus.Params) (consensus.Consensus, error) {
	topic, err := params.Network.NewTopic(pbftProto, &proto.GossipMessage{})
	if err != nil {
		return nil, err
	}

	// decode fixed validator set
	var validators []*validator
	fmt.Println(validators)

	fmt.Println(params.Config.Params)

	// panic("x")

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
	err = topic.Subscribe(func(obj interface{}, from peer.ID) {
		gossipMsg := obj.(*proto.GossipMessage)

		var msg *pbft.MessageReq
		if err := json.Unmarshal(gossipMsg.Data, &msg); err != nil {
			panic(err)
		}
		engine.PushMessage(msg)
	})

	if err != nil {
		return nil, fmt.Errorf("cannot subscribe to the pbft topic")
	}

	sync := syncer.NewSyncer(
		params.Logger,
		params.Network,
		params.Blockchain,
		time.Duration(params.BlockTime)*3*time.Second,
	)

	polybft := &Polybft{
		pbftTopic:  topic,
		blockchain: params.Blockchain,
		blockTime:  time.Duration(params.BlockTime),
		syncer:     sync,
		pbft:       engine,
		ecdsaKey:   ecdsaKey,
	}
	return polybft, nil
}

// polybftBackend is an interface defining polybft methods needed by fsm and sync tracker
type polybftBackend interface {
	// CheckIfStuck checks if state machine is stuck.
	CheckIfStuck(num uint64) (uint64, bool)

	// GetValidators retrieves validator set for the given block
	GetValidators(blockNumber uint64, parents []*types.Header) (AccountSet, error)

	// InsertBlock(b *types.Block)
}

type Polybft struct {
	// pbft is the pbft engine
	pbft *pbft.Pbft

	// block time duration
	blockTime time.Duration

	// refernece to the blockchain
	blockchain *blockchain.Blockchain

	// reference to the syncer
	syncer syncer.Syncer

	// topic for pbft consensus
	pbftTopic *network.Topic

	// signing key
	ecdsaKey *ecdsa.PrivateKey

	// validatorSet is fixed
	validatorSet *validatorSet
}

func (p *Polybft) CheckIfStuck(num uint64) (uint64, bool) {
	// TODO implement me
	panic("implement me")
}

func (p *Polybft) GetValidators(blockNumber uint64, parents []*types.Header) (AccountSet, error) {
	// TODO implement me
	panic("implement me")
}

// VerifyHeader verifies the header is correct
func (p *Polybft) VerifyHeader(header *types.Header) error {
	// TOOD: Always verify
	return nil
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

// GetSyncProgression retrieves the current sync progression, if any
func (p *Polybft) GetSyncProgression() *progress.Progression {
	return p.syncer.GetSyncProgression()
}

// Initialize initializes the consensus (e.g. setup data)
func (p *Polybft) Initialize() error {
	return nil
}

func (p *Polybft) InsertBlock(block *types.Block) {
	if err := p.blockchain.WriteBlock(block, "consensus"); err != nil {
		panic(err)
	}
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
		err := p.pbft.SetBackend(b)
		if err != nil {
			return fmt.Errorf("cannot set backend:%v", err)
		}

		// run consensus
		p.pbft.Run(context.Background())
	}
}

// Close closes the connection
func (p *Polybft) Close() error {
	return nil
}

type backend struct {
	height       uint64
	consensus    polybftBackend
	blockTime    time.Duration
	hash         []byte
	validatorSet *validatorSet
}

// BuildProposal builds a proposal for the current round (used if proposer)
func (b *backend) BuildProposal() (*pbft.Proposal, error) {
	block := &types.Block{
		Header: &types.Header{
			Number: b.height,
		},
	}

	hash := block.Hash()
	b.hash = hash[:]

	proposal := &pbft.Proposal{
		Data: block.MarshalRLP(),
		Time: time.Now().Add(b.blockTime),
		Hash: hash[:],
	}
	return proposal, nil
}

// Height returns the height for the current round
func (b *backend) Height() uint64 {
	return b.height
}

// Init is used to signal the backend that a new round is going to start.
func (b *backend) Init(*pbft.RoundInfo) {
	b.hash = nil
}

// Insert inserts the sealed proposal
func (b *backend) Insert(proposal *pbft.SealedProposal) error {
	block := types.Block{}
	if err := block.UnmarshalRLP(proposal.Proposal.Data); err != nil {
		return err
	}

	hash := block.Hash()
	if !bytes.Equal(hash[:], proposal.Proposal.Hash) {
		return fmt.Errorf("wrong hash: %v, %v", hash[:], proposal.Proposal.Hash)
	}
	b.hash = hash[:]

	// TODO: merge sealed keys in extra block data
	// TODO insert block
	// b.consensus.InsertBlock(&block)
	return nil
}

// IsStuck returns whether the pbft is stucked
func (b *backend) IsStuck(num uint64) (uint64, bool) {
	// TODO
	return 0, false
}

// Validate validates a raw proposal (used if non-proposer)
func (b *backend) Validate(proposal *pbft.Proposal) error {
	block := types.Block{}
	if err := block.UnmarshalRLP(proposal.Data); err != nil {
		return err
	}
	if block.Header.Number != b.height {
		return fmt.Errorf("wrong height, expected %d but found %d", b.height, block.Header.Number)
	}
	return nil
}

// ValidatorSet returns the validator set for the current round
func (b *backend) ValidatorSet() pbft.ValidatorSet {
	return b.validatorSet
}

// ValidateCommit is used to validate that a given commit is valid
func (b *backend) ValidateCommit(from pbft.NodeID, seal []byte) error {
	pub, err := crypto.RecoverPubkey(seal, b.hash)
	if err != nil {
		return err
	}
	addr, err := crypto.GetAddressFromKey(pub)
	if err != nil {
		return err
	}
	if addr.String() != string(from) {
		return fmt.Errorf("wrong seal address: %s, %s", addr.String(), string(from))
	}
	return nil
}

type pbftTransport struct {
	topic *network.Topic
}

func (p *pbftTransport) Gossip(msg *pbft.MessageReq) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	protoMsg := &proto.GossipMessage{
		Data: data,
	}
	return p.topic.Publish(protoMsg)
}

var _ polybftBackend = &Polybft{}
