package polybft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	bolt "go.etcd.io/bbolt"
)

var (
	bigZero          = big.NewInt(0)
	validatorTypeABI = abi.MustNewType("tuple(uint256[4] blsKey, uint256 stake, bool isWhitelisted, bool isActive)")
)

// StakeManager interface provides functions for handling stake change of validators
// and updating validator set based on changed stake
type StakeManager interface {
	EventSubscriber
	PostBlock(req *PostBlockRequest) error
	UpdateValidatorSet(epoch uint64, currentValidatorSet validator.AccountSet) (*validator.ValidatorSetDelta, error)
}

var _ StakeManager = (*dummyStakeManager)(nil)

// dummyStakeManager is a dummy implementation of StakeManager interface
// used only for unit testing
type dummyStakeManager struct{}

func (d *dummyStakeManager) PostBlock(req *PostBlockRequest) error { return nil }
func (d *dummyStakeManager) UpdateValidatorSet(epoch uint64,
	currentValidatorSet validator.AccountSet) (*validator.ValidatorSetDelta, error) {
	return &validator.ValidatorSetDelta{}, nil
}

// EventSubscriber implementation
func (d *dummyStakeManager) GetLogFilters() map[types.Address][]types.Hash {
	return make(map[types.Address][]types.Hash)
}
func (d *dummyStakeManager) ProcessLog(header *types.Header, log *ethgo.Log, dbTx *bolt.Tx) error {
	return nil
}

var _ StakeManager = (*stakeManager)(nil)

// stakeManager saves transfer events that happened in each block
// and calculates updated validator set based on changed stake
type stakeManager struct {
	logger                  hclog.Logger
	state                   *State
	rootChainRelayer        txrelayer.TxRelayer
	key                     ethgo.Key
	supernetManagerContract types.Address
	validatorSetContract    types.Address
	maxValidatorSetSize     int
	polybftBackend          polybftBackend
}

// newStakeManager returns a new instance of stake manager
func newStakeManager(
	logger hclog.Logger,
	state *State,
	rootchainRelayer txrelayer.TxRelayer,
	key ethgo.Key,
	validatorSetAddr, supernetManagerAddr types.Address,
	blockchain blockchainBackend,
	polybftBackend polybftBackend,
	maxValidatorSetSize int,
	dbTx *bolt.Tx,
) (*stakeManager, error) {
	sm := &stakeManager{
		logger:                  logger,
		state:                   state,
		rootChainRelayer:        rootchainRelayer,
		key:                     key,
		supernetManagerContract: supernetManagerAddr,
		validatorSetContract:    validatorSetAddr,
		maxValidatorSetSize:     maxValidatorSetSize,
		polybftBackend:          polybftBackend,
	}

	if err := sm.init(blockchain, dbTx); err != nil {
		return nil, err
	}

	return sm, nil
}

// PostBlock is called on every insert of finalized block (either from consensus or syncer)
// It will update the fullValidatorSet in db to the current block number
// Note that EventSubscriber - AddLog will get all the transfer events that happened in block
func (s *stakeManager) PostBlock(req *PostBlockRequest) error {
	fullValidatorSet, err := s.getOrInitValidatorSet(req.DBTx)
	if err != nil {
		return err
	}

	blockNumber := req.FullBlock.Block.Number()

	s.logger.Debug("Stake manager on post block",
		"block", blockNumber,
		"last saved", fullValidatorSet.BlockNumber,
		"last updated", fullValidatorSet.UpdatedAtBlockNumber)

	// we should save new state even if number of events is zero
	// because otherwise next time we will process more blocks
	fullValidatorSet.EpochID = req.Epoch
	fullValidatorSet.BlockNumber = blockNumber

	return s.state.StakeStore.insertFullValidatorSet(fullValidatorSet, req.DBTx)
}

func (s *stakeManager) init(blockchain blockchainBackend, dbTx *bolt.Tx) error {
	currentHeader := blockchain.CurrentHeader()
	currentBlockNumber := currentHeader.Number

	validatorSet, err := s.getOrInitValidatorSet(dbTx)
	if err != nil {
		return err
	}

	// early return if current block is already processed
	if validatorSet.BlockNumber == currentBlockNumber {
		return nil
	}

	// retrieve epoch needed for state
	epochID, err := getEpochID(blockchain, currentHeader)
	if err != nil {
		return err
	}

	s.logger.Debug("Stake manager on post block",
		"block", currentBlockNumber,
		"last saved", validatorSet.BlockNumber,
		"last updated", validatorSet.UpdatedAtBlockNumber)

	// we will use eventsGetter to update the fullValidatorSet if
	// for any reason, we don't have the correct state
	eventsGetter := &eventsGetter[*contractsapi.TransferEvent]{
		receiptsGetter: receiptsGetter{
			blockchain: blockchain,
		},
		isValidLogFn: func(l *types.Log) bool {
			return l.Address == s.validatorSetContract
		},
		parseEventFn: func(h *types.Header, l *ethgo.Log) (*contractsapi.TransferEvent, bool, error) {
			var transferEvent contractsapi.TransferEvent
			doesMatch, err := transferEvent.ParseLog(l)

			return &transferEvent, doesMatch, err
		},
	}

	transferEvents, err := eventsGetter.getEventsFromBlocksRange(validatorSet.BlockNumber+1, currentBlockNumber)
	if err != nil {
		return err
	}

	if err := s.updateWithReceipts(&validatorSet, transferEvents, currentBlockNumber); err != nil {
		return err
	}

	// we should save new state even if number of events is zero
	// because otherwise next time we will process more blocks
	validatorSet.EpochID = epochID
	validatorSet.BlockNumber = currentBlockNumber

	return s.state.StakeStore.insertFullValidatorSet(validatorSet, dbTx)
}

func (s *stakeManager) getOrInitValidatorSet(dbTx *bolt.Tx) (validatorSetState, error) {
	validatorSet, err := s.state.StakeStore.getFullValidatorSet(dbTx)
	if err != nil {
		if !errors.Is(err, errNoFullValidatorSet) {
			return validatorSetState{}, err
		}

		validators, err := s.polybftBackend.GetValidatorsWithTx(0, nil, dbTx)
		if err != nil {
			return validatorSetState{}, err
		}

		validatorSet = validatorSetState{
			BlockNumber:          0,
			EpochID:              0,
			UpdatedAtBlockNumber: 0,
			Validators:           newValidatorStakeMap(validators),
		}

		if err = s.state.StakeStore.insertFullValidatorSet(validatorSet, dbTx); err != nil {
			return validatorSetState{}, err
		}
	}

	return validatorSet, nil
}

func (s *stakeManager) updateWithReceipts(
	fullValidatorSet *validatorSetState,
	transferEvents []*contractsapi.TransferEvent,
	blockNumber uint64) error {
	if len(transferEvents) == 0 {
		return nil
	}

	for _, event := range transferEvents {
		if event.IsStake() {
			s.logger.Debug("Stake transfer event", "to", event.To, "value", event.Value)

			// then this amount was minted To validator address
			fullValidatorSet.Validators.addStake(event.To, event.Value)
		} else if event.IsUnstake() {
			s.logger.Debug("Unstake transfer event", "from", event.From, "value", event.Value)

			// then this amount was burned From validator address
			fullValidatorSet.Validators.removeStake(event.From, event.Value)
		} else {
			// this should not happen, but lets log it if it does
			s.logger.Warn("Found a transfer event that represents neither stake nor unstake")
		}
	}

	for addr, data := range fullValidatorSet.Validators {
		if data.BlsKey == nil {
			blsKey, err := s.getBlsKey(data.Address)
			if err != nil {
				s.logger.Warn("Could not get info for new validator",
					"block", blockNumber, "address", addr)
			}

			data.BlsKey = blsKey
		}

		data.IsActive = data.VotingPower.Cmp(bigZero) > 0
	}

	// mark on which block validator set has been updated
	fullValidatorSet.UpdatedAtBlockNumber = blockNumber

	s.logger.Debug("Full validator set after", "block", blockNumber, "data", fullValidatorSet.Validators)

	return nil
}

// UpdateValidatorSet returns an updated validator set
// based on stake change (transfer) events from ValidatorSet contract
func (s *stakeManager) UpdateValidatorSet(
	epoch uint64, oldValidatorSet validator.AccountSet) (*validator.ValidatorSetDelta, error) {
	s.logger.Info("Calculating validators set update...", "epoch", epoch)

	fullValidatorSet, err := s.state.StakeStore.getFullValidatorSet(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get full validators set. Epoch: %d. Error: %w", epoch, err)
	}

	// stake map that holds stakes for all validators
	stakeMap := fullValidatorSet.Validators

	// slice of all validator set
	newValidatorSet := stakeMap.getSorted(s.maxValidatorSetSize)
	// set of all addresses that will be in next validator set
	addressesSet := make(map[types.Address]struct{}, len(newValidatorSet))

	for _, si := range newValidatorSet {
		addressesSet[si.Address] = struct{}{}
	}

	removedBitmap := bitmap.Bitmap{}
	updatedValidators := validator.AccountSet{}
	addedValidators := validator.AccountSet{}
	oldActiveMap := make(map[types.Address]*validator.ValidatorMetadata)

	for i, validator := range oldValidatorSet {
		oldActiveMap[validator.Address] = validator
		// remove existing validators from validator set if they did not make it to the set
		if _, exists := addressesSet[validator.Address]; !exists {
			removedBitmap.Set(uint64(i))
		}
	}

	for _, newValidator := range newValidatorSet {
		// check if its already in existing validator set
		if oldValidator, exists := oldActiveMap[newValidator.Address]; exists {
			if oldValidator.VotingPower.Cmp(newValidator.VotingPower) != 0 {
				updatedValidators = append(updatedValidators, newValidator)
			}
		} else {
			if newValidator.BlsKey == nil {
				newValidator.BlsKey, err = s.getBlsKey(newValidator.Address)
				if err != nil {
					return nil, fmt.Errorf("could not retrieve validator data. Address: %v. Error: %w",
						newValidator.Address, err)
				}
			}

			addedValidators = append(addedValidators, newValidator)
		}
	}

	s.logger.Info("Calculating validators set update finished.", "epoch", epoch)

	delta := &validator.ValidatorSetDelta{
		Added:   addedValidators,
		Updated: updatedValidators,
		Removed: removedBitmap,
	}

	if s.logger.IsDebug() {
		newValidatorSet, err := oldValidatorSet.Copy().ApplyDelta(delta)
		if err != nil {
			return nil, err
		}

		s.logger.Debug("New validator set", "validatorSet", newValidatorSet)
	}

	return delta, nil
}

// getBlsKey returns bls key for validator from the supernet contract
func (s *stakeManager) getBlsKey(address types.Address) (*bls.PublicKey, error) {
	getValidatorFn := &contractsapi.GetValidatorCustomSupernetManagerFn{
		Validator_: address,
	}

	encoded, err := getValidatorFn.EncodeAbi()
	if err != nil {
		return nil, err
	}

	response, err := s.rootChainRelayer.Call(
		s.key.Address(),
		ethgo.Address(s.supernetManagerContract),
		encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke validators function on the supernet manager: %w", err)
	}

	byteResponse, err := hex.DecodeHex(response)
	if err != nil {
		return nil, fmt.Errorf("unable to decode hex response, %w", err)
	}

	decoded, err := validatorTypeABI.Decode(byteResponse)
	if err != nil {
		return nil, err
	}

	output, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert decoded outputs to map")
	}

	blsKey, ok := output["blsKey"].([4]*big.Int)
	if !ok {
		return nil, fmt.Errorf("failed to decode blskey")
	}

	pubKey, err := bls.UnmarshalPublicKeyFromBigInt(blsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal BLS public key: %w", err)
	}

	return pubKey, nil
}

// EventSubscriber implementation

// GetLogFilters returns a map of log filters for getting desired events,
// where the key is the address of contract that emits desired events,
// and the value is a slice of signatures of events we want to get.
// This function is the implementation of EventSubscriber interface
func (s *stakeManager) GetLogFilters() map[types.Address][]types.Hash {
	var transferEvent contractsapi.TransferEvent

	return map[types.Address][]types.Hash{
		s.validatorSetContract: {types.Hash(transferEvent.Sig())},
	}
}

// ProcessLog is the implementation of EventSubscriber interface,
// used to handle a log defined in GetLogFilters, provided by event provider
func (s *stakeManager) ProcessLog(header *types.Header, log *ethgo.Log, dbTx *bolt.Tx) error {
	var transferEvent contractsapi.TransferEvent

	doesMatch, err := transferEvent.ParseLog(log)
	if err != nil {
		return err
	}

	if !doesMatch {
		return nil
	}

	fullValidatorSet, err := s.getOrInitValidatorSet(dbTx)
	if err != nil {
		return err
	}

	if err := s.updateWithReceipts(&fullValidatorSet,
		[]*contractsapi.TransferEvent{&transferEvent}, header.Number); err != nil {
		return err
	}

	return s.state.StakeStore.insertFullValidatorSet(fullValidatorSet, dbTx)
}

type validatorSetState struct {
	BlockNumber          uint64            `json:"block"`
	EpochID              uint64            `json:"epoch"`
	UpdatedAtBlockNumber uint64            `json:"updated_at_block"`
	Validators           validatorStakeMap `json:"validators"`
}

func (vs validatorSetState) Marshal() ([]byte, error) {
	return json.Marshal(vs)
}

func (vs *validatorSetState) Unmarshal(b []byte) error {
	return json.Unmarshal(b, vs)
}

// validatorStakeMap holds ValidatorMetadata for each validator address
type validatorStakeMap map[types.Address]*validator.ValidatorMetadata

// newValidatorStakeMap returns a new instance of validatorStakeMap
func newValidatorStakeMap(validatorSet validator.AccountSet) validatorStakeMap {
	stakeMap := make(validatorStakeMap, len(validatorSet))

	for _, v := range validatorSet {
		stakeMap[v.Address] = v.Copy()
	}

	return stakeMap
}

// addStake adds given amount to a validator defined by address
func (sc *validatorStakeMap) addStake(address types.Address, amount *big.Int) {
	if metadata, exists := (*sc)[address]; exists {
		metadata.VotingPower.Add(metadata.VotingPower, amount)
		metadata.IsActive = metadata.VotingPower.Cmp(bigZero) > 0
	} else {
		(*sc)[address] = &validator.ValidatorMetadata{
			VotingPower: new(big.Int).Set(amount),
			Address:     address,
			IsActive:    amount.Cmp(bigZero) > 0,
		}
	}
}

// removeStake removes given amount from validator defined by address
func (sc *validatorStakeMap) removeStake(address types.Address, amount *big.Int) {
	stakeData := (*sc)[address]
	stakeData.VotingPower.Sub(stakeData.VotingPower, amount)
	stakeData.IsActive = stakeData.VotingPower.Cmp(bigZero) > 0
}

// getSorted returns validators (*ValidatorMetadata) in sorted order
func (sc validatorStakeMap) getSorted(maxValidatorSetSize int) validator.AccountSet {
	activeValidators := make(validator.AccountSet, 0, len(sc))

	for _, v := range sc {
		if v.VotingPower.Cmp(bigZero) > 0 {
			activeValidators = append(activeValidators, v)
		}
	}

	sort.Slice(activeValidators, func(i, j int) bool {
		v1, v2 := activeValidators[i], activeValidators[j]

		switch v1.VotingPower.Cmp(v2.VotingPower) {
		case 1:
			return true
		case 0:
			return bytes.Compare(v1.Address[:], v2.Address[:]) < 0
		default:
			return false
		}
	})

	if len(activeValidators) <= maxValidatorSetSize {
		return activeValidators
	}

	return activeValidators[:maxValidatorSetSize]
}

func (sc validatorStakeMap) String() string {
	var sb strings.Builder

	for _, x := range sc.getSorted(len(sc)) {
		bls := ""
		if x.BlsKey != nil {
			bls = hex.EncodeToString(x.BlsKey.Marshal())
		}

		sb.WriteString(fmt.Sprintf("%s:%s:%s:%t\n",
			x.Address, x.VotingPower, bls, x.IsActive))
	}

	return sb.String()
}

func getEpochID(blockchain blockchainBackend, header *types.Header) (uint64, error) {
	provider, err := blockchain.GetStateProviderForBlock(header)
	if err != nil {
		return 0, err
	}

	return blockchain.GetSystemState(provider).GetEpoch()
}
