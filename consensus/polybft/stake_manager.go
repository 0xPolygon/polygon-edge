package polybft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var (
	bigZero          = big.NewInt(0)
	validatorTypeABI = abi.MustNewType("tuple(uint256[4] blsKey, uint256 stake, bool isWhitelisted, bool isActive)")
)

// StakeManager interface provides functions for handling stake change of validators
// and updating validator set based on changed stake
type StakeManager interface {
	PostBlock(req *PostBlockRequest) error
	PostEpoch(req *PostEpochRequest) error
	UpdateValidatorSet(epoch uint64, currentValidatorSet AccountSet) (*ValidatorSetDelta, error)
}

// dummyStakeManager is a dummy implementation of StakeManager interface
// used only for unit testing
type dummyStakeManager struct{}

func (d *dummyStakeManager) PostBlock(req *PostBlockRequest) error { return nil }
func (d *dummyStakeManager) PostEpoch(req *PostEpochRequest) error { return nil }
func (d *dummyStakeManager) UpdateValidatorSet(epoch uint64,
	currentValidatorSet AccountSet) (*ValidatorSetDelta, error) {
	return &ValidatorSetDelta{}, nil
}

var _ StakeManager = (*stakeManager)(nil)

// stakeManager saves transfer events that happened in each block
// and calculates updated validator set based on changed stake
type stakeManager struct {
	logger                  hclog.Logger
	state                   *State
	rootChainRelayer        txrelayer.TxRelayer
	key                     ethgo.Key
	validatorSetContract    types.Address
	supernetManagerContract types.Address
	maxValidatorSetSize     int
}

// newStakeManager returns a new instance of stake manager
func newStakeManager(
	logger hclog.Logger,
	state *State,
	rootchainRelayer txrelayer.TxRelayer,
	key ethgo.Key,
	validatorSetAddr, supernetManagerAddr types.Address,
	maxValidatorSetSize int,
) *stakeManager {
	return &stakeManager{
		logger:                  logger,
		state:                   state,
		rootChainRelayer:        rootchainRelayer,
		key:                     key,
		validatorSetContract:    validatorSetAddr,
		supernetManagerContract: supernetManagerAddr,
		maxValidatorSetSize:     maxValidatorSetSize,
	}
}

// PostEpoch saves the initial validator set to db
func (s *stakeManager) PostEpoch(req *PostEpochRequest) error {
	if req.NewEpochID != 1 {
		return nil
	}

	// save initial validator set as full validator set in db
	return s.state.StakeStore.insertFullValidatorSet(validatorSetState{
		BlockNumber: 0,
		EpochID:     0,
		Validators:  newValidatorStakeMap(req.ValidatorSet.Accounts()),
	})
}

// PostBlock is called on every insert of finalized block (either from consensus or syncer)
// It will read any transfer event that happened in block and update full validator set in db
func (s *stakeManager) PostBlock(req *PostBlockRequest) error {
	events, err := s.getTransferEventsFromReceipts(req.FullBlock.Receipts)
	if err != nil {
		return err
	}

	if len(events) == 0 {
		return nil
	}

	s.logger.Debug("Gotten transfer (stake changed) events from logs on block",
		"eventsNum", len(events), "block", req.FullBlock.Block.Number())

	fullValidatorSet, err := s.state.StakeStore.getFullValidatorSet()
	if err != nil {
		return err
	}

	stakeMap := fullValidatorSet.Validators

	s.logger.Debug("full validator set before", "block", fullValidatorSet.BlockNumber, "data", stakeMap)

	for _, event := range events {
		if event.IsStake() {
			s.logger.Debug("Stake transfer event", "To", event.To, "Value", event.Value)

			// then this amount was minted To validator address
			stakeMap.addStake(event.To, event.Value)
		} else if event.IsUnstake() {
			s.logger.Debug("Unstake transfer event", "From", event.From, "Value", event.Value)

			// then this amount was burned From validator address
			stakeMap.removeStake(event.From, event.Value)
		} else {
			// this should not happen, but lets log it if it does
			s.logger.Warn("Found a transfer event that represents neither stake nor unstake")
		}
	}

	for addr, data := range stakeMap {
		if data.BlsKey == nil {
			data.BlsKey, err = s.getBlsKey(data.Address)
			if err != nil {
				s.logger.Warn("Could not get info for new validator", "epoch", req.Epoch, "address", addr)
			}
		}

		data.IsActive = data.VotingPower.Cmp(bigZero) > 0
	}

	s.logger.Debug("full validator set after", "block", req.FullBlock.Block.Number(), "data", stakeMap)

	return s.state.StakeStore.insertFullValidatorSet(validatorSetState{
		EpochID:     req.Epoch,
		BlockNumber: req.FullBlock.Block.Number(),
		Validators:  stakeMap,
	})
}

// UpdateValidatorSet returns an updated validator set
// based on stake change (transfer) events from ValidatorSet contract
func (s *stakeManager) UpdateValidatorSet(epoch uint64, oldValidatorSet AccountSet) (*ValidatorSetDelta, error) {
	s.logger.Info("Calculating validators set update...", "epoch", epoch)

	fullValidatorSet, err := s.state.StakeStore.getFullValidatorSet()
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
	updatedValidators := AccountSet{}
	addedValidators := AccountSet{}
	oldActiveMap := make(map[types.Address]*ValidatorMetadata)

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

	delta := &ValidatorSetDelta{
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

// getTransferEventsFromReceipts parses logs from receipts to find transfer events
func (s *stakeManager) getTransferEventsFromReceipts(receipts []*types.Receipt) ([]*contractsapi.TransferEvent, error) {
	events := make([]*contractsapi.TransferEvent, 0)

	for i := 0; i < len(receipts); i++ {
		if receipts[i].Status == nil || *receipts[i].Status != types.ReceiptSuccess {
			continue
		}

		for _, log := range receipts[i].Logs {
			if log.Address != s.validatorSetContract {
				continue
			}

			var transferEvent contractsapi.TransferEvent

			doesMatch, err := transferEvent.ParseLog(convertLog(log))
			if err != nil {
				return nil, err
			}

			if !doesMatch {
				continue
			}

			events = append(events, &transferEvent)
		}
	}

	return events, nil
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

type validatorSetState struct {
	BlockNumber uint64            `json:"block"`
	EpochID     uint64            `json:"epoch"`
	Validators  validatorStakeMap `json:"validators"`
}

func (vs validatorSetState) Marshal() ([]byte, error) {
	return json.Marshal(vs)
}

func (vs *validatorSetState) Unmarshal(b []byte) error {
	return json.Unmarshal(b, vs)
}

// validatorStakeMap holds ValidatorMetadata for each validator address
type validatorStakeMap map[types.Address]*ValidatorMetadata

// newValidatorStakeMap returns a new instance of validatorStakeMap
func newValidatorStakeMap(validatorSet AccountSet) validatorStakeMap {
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
		(*sc)[address] = &ValidatorMetadata{
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
func (sc validatorStakeMap) getSorted(maxValidatorSetSize int) AccountSet {
	activeValidators := make(AccountSet, 0, len(sc))

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
