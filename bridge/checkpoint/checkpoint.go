package checkpoint

import (
	"github.com/0xPolygon/polygon-edge/bridge/checkpoint/transport"
	"github.com/0xPolygon/polygon-edge/bridge/sam"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	EpochSize = 100
)

type Checkpoint interface {
	Start() error
	Close() error
	CreateNewCheckpoint() error
	SetValidators([]types.Address)
}

type Blockchain interface {
	GetBlocks(start, end uint64) ([]types.Block, error)
}

type checkpoint struct {
	logger            hclog.Logger
	signer            sam.Signer
	blockchain        Blockchain
	rootchainContract RootChainContractClient
	transport         transport.CheckpointTransport
	validators        []types.Address

	sampool sam.Pool

	closeCh chan struct{}
}

func NewCheckpoint(
	logger hclog.Logger,
	network *network.Server,
	signer sam.Signer,
) (Checkpoint, error) {
	checkpointLogger := logger.Named("checkpoint")

	return &checkpoint{
		logger: checkpointLogger,
		signer: signer,
		// blockchain: blockchain,
		// rootchainContract: rootchainContractClient,
		transport: transport.NewLibp2pGossipTransport(logger, network),
		sampool:   sam.NewPool(nil, 0),
	}, nil
}

func (c *checkpoint) Start() error {
	if err := c.transport.Start(); err != nil {
		return err
	}

	//	TODO: subscribe to CheckpointProposal, AckMessage, NoAckMessage
	//if err := c.transport.Subscribe(...); err != nil {
	//	return err
	//}

	return nil
}

func (c *checkpoint) Close() error {
	return nil
}

func (c *checkpoint) SetValidators(vals []types.Address) {
	c.validators = vals
}

func (c *checkpoint) CreateNewCheckpoint() error {
	// Step1: Get Latest information stored in RootChain contract
	currentEpoch, err := c.rootchainContract.GetCurrentHeaderBlock()
	if err != nil {
		return err
	}

	lastChildBlock, err := c.rootchainContract.GetLastChildBlock()
	if err != nil {
		return err
	}

	// Step2: Determine the range of next checkpoint and get blocks from local chain
	start, end := c.determineCheckpointRange(lastChildBlock)
	blocks, err := c.blockchain.GetBlocks(start, end)
	if err != nil {
		return err
	}

	// Step3: Generate Checkpoint
	checkpoint, err := c.generateCheckpoint(blocks)
	if err != nil {
		return err
	}

	// Step4: Get Hash, Sign it, and store to SAM Pool
	hash := checkpoint.Hash()
	sig, err := c.signer.Sign(hash.Bytes())
	if err != nil {
		return err
	}

	//	TODO: add to sampool
	//	1. add msg
	c.sampool.AddMessage(&sam.Message{
		Hash: hash,
		Data: checkpoint,
	})

	//	2. add signature
	c.sampool.AddSignature(&sam.MessageSignature{
		Hash:      hash,
		Signature: sig,
	})

	//	3. publish gossip (or only if proposer? == step5)

	// Step5: Propose checkpoint if proposer
	proposer := c.getProposer(currentEpoch)
	if proposer == c.signer.Address() {
		if err := c.ProposeCheckpoint(checkpoint, sig); err != nil {
			return err
		}
	}

	// Step5: Watch Checkpoint signatures

	return nil
}

func (c *checkpoint) determineCheckpointRange(lastChildBlock uint64) (uint64, uint64) {
	// TODO: implement
	return lastChildBlock + 1, lastChildBlock + EpochSize
}

func (c *checkpoint) generateCheckpoint(blocks []types.Block) (*CheckpointData, error) {
	// TODO: implement
	return nil, nil
}

func (c *checkpoint) getProposer(epoch uint64) types.Address {
	// FIXME: consider round change
	// FIXME: fetch from contract in Edge
	return c.validators[int(epoch)%int(len(c.validators))]
}

func (c *checkpoint) ProposeCheckpoint(checkpoint *CheckpointData, signature []byte) error {
	return c.transport.ProposeCheckpoint(&transport.CheckpointProposalMessage{
		Checkpoint: transport.Checkpoint{
			Proposer:        checkpoint.Proposer,
			Start:           checkpoint.Start,
			End:             checkpoint.End,
			RootHash:        checkpoint.RootHash,
			AccountRootHash: checkpoint.AccountRootHash,
			Timestamp:       checkpoint.Timestamp,
		},
		Signature: signature,
	})
}
