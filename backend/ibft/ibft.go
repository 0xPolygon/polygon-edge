package ibft

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/blockchain"
	"time"

	"github.com/0xPolygon/polygon-edge/backend"
	"github.com/0xPolygon/polygon-edge/backend/ibft/proto"
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
	ErrInvalidHookParam     = errors.New("invalid IBFT hook param passed in")
	ErrInvalidMechanismType = errors.New("invalid backend mechanism type in params")
	ErrMissingMechanismType = errors.New("missing backend mechanism type in params")
)

type blockchainInterface interface {
	Header() *types.Header
	GetHeaderByNumber(i uint64) (*types.Header, bool)
	WriteBlock(block *types.Block) error
	VerifyPotentialBlock(block *types.Block) error
	CalculateGasLimit(number uint64) (uint64, error)
}

type txPoolInterface interface {
	Prepare()
	Length() uint64
	Peek() *types.Transaction
	Pop(tx *types.Transaction)
	Drop(tx *types.Transaction)
	Demote(tx *types.Transaction)
	ResetWithHeaders(headers ...*types.Header)
}

// backendIBFT represents the IBFT backend mechanism object
type backendIBFT struct {
	logger hclog.Logger

	config *backend.Config // Backend configuration

	consensus *consensus

	blockchain *blockchain.Blockchain // Interface exposed by the blockchain layer

	network  *network.Server // Reference to the networking layer
	executor *state.Executor // Reference to the state executor
	txpool   txPoolInterface // Reference to the transaction pool
	syncer   syncer.Syncer   // Reference to the sync protocol
	Grpc     *grpc.Server    // gRPC configuration

	metrics *backend.Metrics

	secretsManager secrets.SecretsManager

	validatorKey        *ecdsa.PrivateKey // Private key for the validator
	validatorKeyAddr    types.Address
	currentValidatorSet ValidatorSet

	store     *snapshotStore // Snapshot store that keeps track of all snapshots
	transport transport      // Reference to the transport protocol
	operator  *operator

	mechanisms []ConsensusMechanism // IBFT ConsensusMechanism used (PoA / PoS)

	epochSize          uint64
	quorumSizeBlockNum uint64

	blockTime       time.Duration // Minimum block generation time in seconds
	ibftBaseTimeout time.Duration // Base timeout for IBFT message in seconds

	sealing bool // Flag indicating if the node is a sealer

	closeCh chan struct{} // Channel for closing
}

// Factory implements the base backend Factory method
func Factory(params *backend.BackendParams) (backend.Backend, error) {
	//	defaults for user set fields in genesis
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
		//	Block number specified for quorum size switch
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
		ibftBaseTimeout:    time.Duration(params.IBFTBaseTimeout) * time.Second, //	TODO: what is this for
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

	// Set up the snapshots
	if err := i.setupSnapshot(); err != nil {
		return err
	}

	snap, err := i.getLatestSnapshot()
	if err != nil {
		return err
	}

	i.currentValidatorSet = snap.Set

	return nil
}

// Start starts the IBFT backend
func (i *backendIBFT) Start() error {
	// Start the syncer
	if err := i.syncer.Start(); err != nil {
		return err
	}

	// Start the actual IBFT protocol
	go i.startt()

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

//  setupTransport read current mechanism in params and sets up backend mechanism
func (i *backendIBFT) setupMechanism() error {
	ibftForks, err := GetIBFTForks(i.config.Config)
	if err != nil {
		return err
	}

	i.mechanisms = make([]ConsensusMechanism, len(ibftForks))

	for idx, fork := range ibftForks {
		factory, ok := mechanismBackends[fork.Type]
		if !ok {
			return fmt.Errorf("backend mechanism doesn't define: %s", fork.Type)
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

func (i *backendIBFT) startt() {
	callInsertBlockHook := func(blockNumber uint64) {
		if hookErr := i.runHook(InsertBlockHook, blockNumber, blockNumber); hookErr != nil {
			i.logger.Error(fmt.Sprintf("Unable to run hook %s, %v", InsertBlockHook, hookErr))
		}
	}

	//	TODO: refactor into method
	go func() {
		if err := i.syncer.WatchSync(func(block *types.Block) bool {
			callInsertBlockHook(block.Number())
			i.txpool.ResetWithHeaders(block.Header)

			return false
		}); err != nil {
			i.logger.Error("watch sync fail", "err", err)
		}
	}()

	//	TODO: refactor into method
	newBlockSub := i.blockchain.SubscribeEvents()
	syncerBlock := make(chan struct{})
	go func() {
		for {
			ev := <-newBlockSub.GetEventCh()
			if ev.Source == "syncer" {
				syncerBlock <- struct{}{}
			}
		}

	}()

	defer newBlockSub.Close()

	for {
		latest := i.blockchain.Header().Number
		snap, _ := i.getSnapshot(latest)

		i.currentValidatorSet = snap.Set
		//Update the No.of validator metric
		i.metrics.Validators.Set(float64(len(snap.Set)))

		if !i.currentValidatorSet.Includes(i.validatorKeyAddr) {
			continue
		}

		select {
		case <-i.consensus.runSequence(latest + 1):
			//	consensus inserted block
			continue
		case <-syncerBlock:
			//	syncer inserted block -> stop running consensus
			i.consensus.cancelSequence()
			i.logger.Info("canceled sequence", "sequence", latest+1)
		case <-i.closeCh:
			//	IBFT backend stopped
			i.consensus.cancelSequence()
			return
		}
	}
}

// isValidSnapshot checks if the current node is in the validator set for the latest snapshot
func (i *backendIBFT) isValidSnapshot() bool {
	if !i.isSealing() {
		return false
	}

	// check if we are a validator and enabled
	header := i.blockchain.Header()
	snap, err := i.getSnapshot(header.Number)

	if err != nil {
		return false
	}

	if snap.Set.Includes(i.validatorKeyAddr) {
		return true
	}

	return false
}

// shouldWriteTransactions checks if each backend mechanism accepts a block with transactions at given height
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

type status uint8

const (
	success status = iota
	fail
	skip
)

type txExeResult struct {
	tx     *types.Transaction
	status status
}

func (i *backendIBFT) writeTransaction(
	tx *types.Transaction,
	transition transitionInterface,
	gasLimit uint64,
) (*txExeResult, bool) {
	if tx == nil {
		return nil, false
	}

	if tx.ExceedsBlockGasLimit(gasLimit) {
		i.txpool.Drop(tx)

		if err := transition.WriteFailedReceipt(tx); err != nil {
			//	log this error
		}

		//	continue processing
		return &txExeResult{tx, fail}, true
	}

	if err := transition.Write(tx); err != nil {
		if _, ok := err.(*state.GasLimitReachedTransitionApplicationError); ok { // nolint:errorlint
			//	stop processing
			return nil, false
		} else if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable { // nolint:errorlint
			i.txpool.Demote(tx)

			return &txExeResult{tx, skip}, true
		} else {
			i.txpool.Drop(tx)

			return &txExeResult{tx, fail}, true
		}
	}

	i.txpool.Pop(tx)

	return &txExeResult{tx, success}, true
}

func (i *backendIBFT) executeTransactions(
	gasLimit,
	blockNumber uint64,
	transition transitionInterface,
) (executed []*types.Transaction) {
	executed = make([]*types.Transaction, 0)

	if !i.shouldWriteTransactions(blockNumber) {
		return
	}

	var (
		blockTimer    = time.NewTimer(i.blockTime)
		stopExecution = false

		successful = 0
		failed     = 0
		skipped    = 0
	)

	defer blockTimer.Stop()

	i.txpool.Prepare()

	for {
		select {
		case <-blockTimer.C:
			return
		default:
			if stopExecution {
				//	wait for the timer to expire
				continue
			}

			//	execute transactions one by one
			result, ok := i.writeTransaction(
				i.txpool.Peek(),
				transition,
				gasLimit,
			)
			if !ok {
				stopExecution = true
				continue
			}

			tx := result.tx

			switch result.status {
			case success:
				executed = append(executed, tx)
				successful++
			case fail:
				failed++
			case skip:
				skipped++
			}

		}
	}
}

//	TODO: ~~BuildProposal
// buildBlock builds the block, based on the passed in snapshot and parent header
func (i *backendIBFT) buildBlock(snap *Snapshot, parent *types.Header) (*types.Block, error) {
	header := &types.Header{
		ParentHash: parent.Hash,
		Number:     parent.Number + 1,
		Miner:      types.Address{},
		Nonce:      types.Nonce{},
		MixHash:    IstanbulDigest,
		// this is required because blockchain needs difficulty to organize blocks and forks
		Difficulty: parent.Number + 1,
		StateRoot:  types.EmptyRootHash, // this avoids needing state for now
		Sha3Uncles: types.EmptyUncleHash,
		GasLimit:   parent.GasLimit, // Inherit from parent for now, will need to adjust dynamically later.
	}

	// calculate gas limit based on parent header
	gasLimit, err := i.blockchain.CalculateGasLimit(header.Number)
	if err != nil {
		return nil, err
	}

	header.GasLimit = gasLimit

	if hookErr := i.runHook(CandidateVoteHook, header.Number, &candidateVoteHookParams{
		header: header,
		snap:   snap,
	}); hookErr != nil {
		i.logger.Error(fmt.Sprintf("Unable to run hook %s, %v", CandidateVoteHook, hookErr))
	}

	// set the timestamp
	parentTime := time.Unix(int64(parent.Timestamp), 0)
	headerTime := parentTime.Add(i.blockTime)

	if headerTime.Before(time.Now()) {
		headerTime = time.Now()
	}

	header.Timestamp = uint64(headerTime.Unix())

	// we need to include in the extra field the current set of validators
	putIbftExtraValidators(header, snap.Set)

	transition, err := i.executor.BeginTxn(parent.StateRoot, header, i.validatorKeyAddr)
	if err != nil {
		return nil, err
	}
	// If the mechanism is PoS -> build a regular block if it's not an end-of-epoch block
	// If the mechanism is PoA -> always build a regular block, regardless of epoch

	txs := i.executeTransactions(gasLimit, header.Number, transition)

	if err := i.PreStateCommit(header, transition); err != nil {
		return nil, err
	}

	_, root := transition.Commit()
	header.StateRoot = root
	header.GasUsed = transition.TotalGas()

	// build the block
	block := backend.BuildBlock(backend.BuildBlockParams{
		Header:   header,
		Txns:     txs,
		Receipts: transition.Receipts(),
	})

	//	TODO: this is how the signature is created (?)
	// write the seal of the block after all the fields are completed
	header, err = writeSeal(i.validatorKey, block.Header)
	if err != nil {
		return nil, err
	}

	block.Header = header

	// compute the hash, this is only a provisional hash since the final one
	// is sealed after all the committed seals
	//	TODO: header hash was never generated with ProposerSeal and CommittedSeals
	block.Header.ComputeHash()

	i.logger.Info("build block", "number", header.Number, "txns", len(txs))

	return block, nil
}

type transitionInterface interface {
	Write(txn *types.Transaction) error
	WriteFailedReceipt(txn *types.Transaction) error
}

// writeTransactions writes transactions from the txpool to the transition object
// and returns transactions that were included in the transition (new block)
func (i *backendIBFT) writeTransactions(gasLimit uint64, transition transitionInterface) []*types.Transaction {
	var transactions []*types.Transaction

	successTxCount := 0
	failedTxCount := 0

	i.txpool.Prepare()

	for {
		tx := i.txpool.Peek()
		if tx == nil {
			break
		}

		if tx.ExceedsBlockGasLimit(gasLimit) {
			if err := transition.WriteFailedReceipt(tx); err != nil {
				failedTxCount++

				i.txpool.Drop(tx)

				continue
			}

			failedTxCount++

			transactions = append(transactions, tx)
			i.txpool.Drop(tx)

			continue
		}

		if err := transition.Write(tx); err != nil {
			if _, ok := err.(*state.GasLimitReachedTransitionApplicationError); ok { // nolint:errorlint
				break
			} else if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable { // nolint:errorlint
				i.txpool.Demote(tx)
			} else {
				failedTxCount++
				i.txpool.Drop(tx)
			}

			continue
		}

		// no errors, pop the tx from the pool
		i.txpool.Pop(tx)

		successTxCount++

		transactions = append(transactions, tx)
	}

	//nolint:lll
	i.logger.Info("executed txns", "failed ", failedTxCount, "successful", successTxCount, "remaining in pool", i.txpool.Length())

	return transactions
}

// updateMetrics will update various metrics based on the given block
// currently we capture No.of Txs and block interval metrics using this function
func (i *backendIBFT) updateMetrics(block *types.Block) {
	// get previous header
	prvHeader, _ := i.blockchain.GetHeaderByNumber(block.Number() - 1)
	parentTime := time.Unix(int64(prvHeader.Timestamp), 0)
	headerTime := time.Unix(int64(block.Header.Timestamp), 0)

	//Update the block interval metric
	if block.Number() > 1 {
		i.metrics.BlockInterval.Set(
			headerTime.Sub(parentTime).Seconds(),
		)
	}

	//Update the Number of transactions in the block metric
	i.metrics.NumTxs.Set(float64(len(block.Body().Transactions)))
}

var (
	errIncorrectBlockLocked    = errors.New("block locked is incorrect")
	errIncorrectBlockHeight    = errors.New("proposed block number is incorrect")
	errBlockVerificationFailed = errors.New("block verification fail")
	errFailedToInsertBlock     = errors.New("fail to insert block")
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

	snap, err := i.getSnapshot(parent.Number)
	if err != nil {
		return err
	}

	// verify all the header fields + seal
	if err := i.verifyHeaderImpl(snap, parent, header); err != nil {
		return err
	}

	// verify the committed seals
	if err := verifyCommittedFields(snap, header, i.quorumSize(header.Number)); err != nil {
		return err
	}

	return nil
}

//	quorumSize returns a callback that when executed on a ValidatorSet computes
//	number of votes required to reach quorum based on the size of the set.
//	The blockNumber argument indicates which formula was used to calculate the result (see PRs #513, #549)
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
	return ecrecoverFromHeader(header)
}

// PreStateCommit a hook to be called before finalizing state transition on inserting block
func (i *backendIBFT) PreStateCommit(header *types.Header, txn *state.Transition) error {
	params := &preStateCommitHookParams{
		header: header,
		txn:    txn,
	}
	if hookErr := i.runHook(PreStateCommitHook, header.Number, params); hookErr != nil {
		return hookErr
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

// Close closes the IBFT backend mechanism, and does write back to disk
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
