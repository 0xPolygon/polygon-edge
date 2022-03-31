package checkpoint

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/bridge/checkpoint/transport"
	ctypes "github.com/0xPolygon/polygon-edge/bridge/checkpoint/types"
	"github.com/0xPolygon/polygon-edge/bridge/sam"
	"github.com/0xPolygon/polygon-edge/bridge/utils"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Trapesys/fastmerkle"
	"github.com/hashicorp/go-hclog"
	"math/big"
)

type Checkpoint interface {
	Start() error
	Close() error
	StartNewCheckpoint(epochSize uint64) error
}

type Blockchain interface {
	Header() *types.Header
	GetBlocks(start, end uint64, full bool) ([]*types.Block, error)
	GetChainID() uint64
}

type checkpoint struct {
	logger            hclog.Logger
	signer            sam.Signer
	blockchain        Blockchain
	rootchainContract RootChainContractClient
	transport         transport.CheckpointTransport
	validatorSet      utils.ValidatorSet
	sampool           sam.Pool
}

func NewCheckpoint(
	logger hclog.Logger,
	network *network.Server,
	blockchain Blockchain,
	signer sam.Signer,
	validatorSet utils.ValidatorSet,
) (Checkpoint, error) {
	checkpointLogger := logger.Named("checkpoint")

	return &checkpoint{
		logger:            checkpointLogger,
		signer:            signer,
		blockchain:        blockchain,
		rootchainContract: nil,
		transport:         transport.NewLibp2pGossipTransport(logger, network),
		validatorSet:      validatorSet,
		sampool:           sam.NewPool(validatorSet),
	}, nil
}

func (c *checkpoint) Start() error {
	if err := c.startTransport(); err != nil {
		return err
	}

	return nil
}

func (c *checkpoint) Close() error {
	return nil
}

func (c *checkpoint) StartNewCheckpoint(epochSize uint64) error {
	// Step1: Get the height block number in RootChain contract
	// For phase1, get from latest Edge block number and epoch size
	c.rootchainContract = &MockRootChainContractClient{
		blockchain: c.blockchain,
		epochSize:  epochSize,
	}

	lastChildBlock, err := c.rootchainContract.GetLastChildBlock()
	if err != nil {
		return err
	}

	// Step2: Determine the range of next checkpoint and get blocks from local chain
	start, end := c.determineCheckpointRange(lastChildBlock)

	blocks, err := c.blockchain.GetBlocks(start, end, true)
	if err != nil {
		return err
	}

	// Step3: Generate Checkpoint
	checkpoint, err := c.generateCheckpoint(blocks)
	if err != nil {
		return err
	}

	// Calculate own signature for checkpoint
	hash := checkpoint.Hash()
	sig, err := c.signer.Sign(hash.Bytes())

	if err != nil {
		return err
	}

	// Step5: Register checkpoint into SAM Pool
	c.sampool.AddMessage(&sam.Message{
		Hash: hash,
		Data: checkpoint,
	})

	c.addCheckpointSignature(checkpoint, c.signer.Address(), sig)

	// Step6: Gossip checkpoint if proposer
	if err := c.gossipCheckpoint(checkpoint, sig); err != nil {
		return err
	}

	// Step7: Start time for timeout (Phase 2)

	return nil
}

func (c *checkpoint) determineCheckpointRange(lastChildBlock uint64) (start uint64, end uint64) {
	lastBlockNumber := c.blockchain.Header().Number

	start = lastChildBlock + 1
	end = lastBlockNumber

	return
}

func (c *checkpoint) generateCheckpoint(blocks []*types.Block) (*ctypes.Checkpoint, error) {
	// Generate the Merkle root for the block hashes
	blockHashes := make([][]byte, len(blocks))
	for i, block := range blocks {
		blockHashes[i] = common.ConcatData([][]byte{
			big.NewInt(int64(block.Header.Number)).Bytes(),
			big.NewInt(int64(block.Header.Timestamp)).Bytes(),
			block.Header.TxRoot.Bytes(),
			block.Header.ReceiptsRoot.Bytes(),
		})
	}

	blocksMerkleTree, treeErr := fastmerkle.GenerateMerkleTree(blockHashes)
	if treeErr != nil {
		return nil, fmt.Errorf(
			"unable to construct a Merkle tree of block hashes, %w",
			treeErr,
		)
	}

	// Generate the Merkle root for the validator set
	validators := c.validatorSet.Validators()
	validatorsBytes := make([][]byte, len(validators))

	for i, validator := range validators {
		validatorsBytes[i] = validator.Bytes()
	}

	validatorsMerkleTree, treeErr := fastmerkle.GenerateMerkleTree(validatorsBytes)
	if treeErr != nil {
		return nil, fmt.Errorf(
			"unable to construct a Merkle tree of validator addresses, %w",
			treeErr,
		)
	}

	// Create a placeholder checkpoint
	return &ctypes.Checkpoint{
		Proposer:        c.getProposer(c.getCheckpointEpoch()),
		Start:           blocks[0].Number(),
		End:             blocks[len(blocks)-1].Number(),
		RootHash:        types.BytesToHash(blocksMerkleTree.RootHash()),
		AccountRootHash: types.BytesToHash(validatorsMerkleTree.RootHash()),
		ChainID:         c.blockchain.GetChainID(),
	}, nil
}

// getCheckpointEpoch returns the epoch of the pending checkpoint
func (c *checkpoint) getCheckpointEpoch() uint64 {
	// TODO: implement
	return 0
}

func (c *checkpoint) getProposer(epoch uint64) types.Address {
	// FIXME: consider round change
	// FIXME: fetch from contract in Edge
	validators := c.validatorSet.Validators()
	if len(validators) == 0 {
		return types.ZeroAddress
	}

	return validators[int(epoch)%len(validators)]
}

func (c *checkpoint) addCheckpointSignature(checkpoint *ctypes.Checkpoint, address types.Address, sig []byte) {
	hash := checkpoint.Hash()

	c.sampool.AddSignature(&sam.MessageSignature{
		Hash:      hash,
		Address:   address,
		Signature: sig,
	})

	total := c.sampool.GetSignatureCount(hash)

	if !c.sampool.IsMessageConsumed(hash) &&
		total >= c.validatorSet.Threshold() &&
		checkpoint.Proposer == c.signer.Address() {
		// TODO: Submit Checkpoint into RootChain contract
		c.logger.Info(
			"received 2/3 signatures for checkpoint, submitting checkpoint to RootChain contract",
			"checkpoint",
			checkpoint,
			"proposer",
			c.signer.Address(),
			"signatures",
			total,
		)

		c.sampool.ConsumeMessage(hash)
	}
}

func (c *checkpoint) addAckSignature(ack *ctypes.Ack, address types.Address, sig []byte) {
	hash := ack.Hash()

	c.sampool.AddSignature(&sam.MessageSignature{
		Hash:      hash,
		Address:   address,
		Signature: sig,
	})

	total := c.sampool.GetSignatureCount(hash)
	if total >= c.validatorSet.Threshold() {
		// TODO: Create Edge Transaction to change proposer in the contract in Edge
		c.logger.Info(
			"received 2/3 signatures for ack, change proposer",
			"ack",
			ack,
			"signatures",
			total,
		)
	}
}

func (c *checkpoint) addNoAckSignature(noAck *ctypes.NoAck, address types.Address, sig []byte) {
	hash := noAck.Hash()

	c.sampool.AddSignature(&sam.MessageSignature{
		Hash:      hash,
		Address:   address,
		Signature: sig,
	})

	total := c.sampool.GetSignatureCount(hash)
	if total >= c.validatorSet.Threshold() {
		// TODO: Create Edge Transaction to change proposer in the contract in Edge
		c.logger.Info(
			"received 2/3 signatures for NoAck, change proposer",
			"noack",
			noAck,
			"signatures",
			total,
		)
	}
}

func (c *checkpoint) startTransport() error {
	if err := c.transport.Start(); err != nil {
		return err
	}

	return c.transport.Subscribe(func(msg interface{}) {
		switch typedMsg := msg.(type) {
		case *transport.CheckpointMessage:
			c.handleCheckpointMessage(typedMsg)
		case *transport.AckMessage:
			c.handleAckMessage(typedMsg)
		case *transport.NoAckMessage:
			c.handleNoAckMessage(typedMsg)
		}
	})
}

func (c *checkpoint) handleCheckpointMessage(msg *transport.CheckpointMessage) {
	sender, err := c.signer.RecoverAddress(msg.Hash().Bytes(), msg.Sig())
	if err != nil {
		c.logger.Error("failed to get address from signature", "err", err)

		return
	}

	isValidator := c.validatorSet.IsValidator(sender)
	if !isValidator {
		c.logger.Info("ignore Checkpoint message from non-validator", "sender", sender, "hash", msg.Hash())

		return
	}

	//	TODO: validate checkpoint?

	c.addCheckpointSignature(&msg.Checkpoint, sender, msg.Sig())
}

func (c *checkpoint) handleAckMessage(msg *transport.AckMessage) {
	sender, err := c.signer.RecoverAddress(msg.Hash().Bytes(), msg.Sig())
	if err != nil {
		c.logger.Error("failed to get address from signature", "err", err)

		return
	}

	isValidator := c.validatorSet.IsValidator(sender)
	if !isValidator {
		c.logger.Info("ignore Ack message from non-validator", "sender", sender, "hash", msg.Hash())

		return
	}

	c.addAckSignature(&msg.Ack, sender, msg.Sig())
}

func (c *checkpoint) handleNoAckMessage(msg *transport.NoAckMessage) {
	sender, err := c.signer.RecoverAddress(msg.Hash().Bytes(), msg.Sig())
	if err != nil {
		c.logger.Error("failed to get address from signature", "err", err)

		return
	}

	isValidator := c.validatorSet.IsValidator(sender)
	if !isValidator {
		c.logger.Info("ignore NoAck message from non-validator", "sender", sender, "hash", msg.Hash())

		return
	}

	c.addNoAckSignature(&msg.NoAck, sender, msg.Sig())
}

func (c *checkpoint) gossipCheckpoint(checkpoint *ctypes.Checkpoint, signature []byte) error {
	return c.transport.SendCheckpoint(&transport.CheckpointMessage{
		Checkpoint: *checkpoint,
		Signature:  signature,
	})
}

// MockRootChainContractClient is a mock for phase1
// don't connect to RootChain contract for now
type MockRootChainContractClient struct {
	blockchain Blockchain
	epochSize  uint64
}

func (m *MockRootChainContractClient) GetLastChildBlock() (uint64, error) {
	header := m.blockchain.Header()

	currentEpoch := header.Number/m.epochSize - 1

	return currentEpoch * m.epochSize, nil
}
