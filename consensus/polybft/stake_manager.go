package polybft

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
)

var (
	// getValidatorABI is an ABI method on SupernetManager contract
	// that returns the validator data
	getValidatorABI, _ = contractsapi.CustomSupernetManager.Abi.Methods["getValidator"]

	bigZero = big.NewInt(0)
)

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
	relayer txrelayer.TxRelayer,
	key ethgo.Key,
	validatorSetAddr, supernetManagerAddr types.Address,
	maxValidatorSetSize int,
) *stakeManager {
	return &stakeManager{
		logger:                  logger,
		state:                   state,
		rootChainRelayer:        relayer,
		key:                     key,
		validatorSetContract:    validatorSetAddr,
		supernetManagerContract: supernetManagerAddr,
		maxValidatorSetSize:     maxValidatorSetSize,
	}
}

// PostBlock is called on every insert of finalized block (either from consensus or syncer)
// It will read any exit event that happened in block and insert it to state boltDb
func (s *stakeManager) PostBlock(req *PostBlockRequest) error {
	epoch := req.Epoch

	if req.IsEpochEndingBlock {
		// transfer events that happened in epoch ending blocks,
		// should be added to the bucket of the next epoch
		epoch++
	}

	// commit exit events only when we finalize a block
	events, err := s.getTransferEventsFromReceipts(epoch, req.FullBlock.Receipts)
	if err != nil {
		return err
	}

	if len(events) > 0 {
		s.logger.Debug("Gotten transfer (stake changed) events from logs on block",
			"eventsNum", len(events), "block", req.FullBlock.Block.Number())
	}

	return s.state.StakeStore.insertTransferEvents(epoch, events)
}

// UpdateValidatorSet returns an updated validator set
// based on stake change (transfer) events from ValidatorSet contract
func (s *stakeManager) UpdateValidatorSet(epoch uint64, currentValidatorSet AccountSet) (*ValidatorSetDelta, error) {
	s.logger.Info("Calculating validators set update...", "epoch", epoch)

	transferEvents, err := s.state.StakeStore.getTransferEvents(epoch)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer events for epoch: %d. Error: %w", epoch, err)
	}

	if len(transferEvents) == 0 {
		return &ValidatorSetDelta{}, nil
	}

	stakeCounter := newStakeCounter(currentValidatorSet)

	for _, event := range transferEvents {
		if event.IsStake() {
			// then this amount was minted To validator address
			stakeCounter.addStake(event.To, event.Value)
		} else if event.IsUnstake() {
			// then this amount was burned From validator address
			stakeCounter.removeStake(event.From, event.Value)
		} else {
			// this should not happen, but lets log it if it does
			s.logger.Debug("Found a transfer event that represents neither stake nor unstake")
		}
	}

	// sort validators by stake since we update the validator set
	// based on highest stakes
	stakeCounter.sortByStake()

	removedBitmap := &bitmap.Bitmap{}
	updatedValidators := AccountSet{}
	addedValidators := AccountSet{}

	// first deal with existing validators
	nonChangedValidators := 0
	for i, a := range currentValidatorSet.Copy() {
		stakeInfo := stakeCounter.getStake(a.Address)

		if stakeInfo.pos > s.maxValidatorSetSize-1 {
			// validator is not in the maximum validator set we support based on its stake
			// so we will remove it
			removedBitmap.Set(uint64(i))
		} else {
			if stakeInfo.stake.Cmp(bigZero) == 0 {
				// validator un-staked all, remove it from validator set
				removedBitmap.Set(uint64(i))
			} else if stakeInfo.stake.Cmp(a.VotingPower) != 0 {
				// validator updated its stake so put it in the updated validators list
				a.VotingPower = new(big.Int).Set(stakeInfo.stake)
				updatedValidators = append(updatedValidators, a)
			} else {
				// he did not change stake, but he's left in validator set
				nonChangedValidators++
			}
		}
	}

	// add new validators
	for _, si := range stakeCounter.iterateThroughNewValidators() {
		if si.pos > s.maxValidatorSetSize-1 {
			// new validator doesn't have enough stake to be in validator set
			continue
		}

		if (len(addedValidators) + len(updatedValidators) + nonChangedValidators) == s.maxValidatorSetSize {
			// we reached the maximum validator set size
			break
		}

		validatorData, err := s.getNewValidatorInfo(si.address, si.stake)
		if err != nil {
			return nil, fmt.Errorf("could not retrieve validator data. Address: %v. Error: %w", si.address, err)
		}

		addedValidators = append(addedValidators, validatorData)
	}

	s.logger.Info("Calculating validators set update finished.", "epoch", epoch)

	delta := &ValidatorSetDelta{
		Added:   addedValidators,
		Updated: updatedValidators,
		Removed: *removedBitmap,
	}

	if s.logger.IsDebug() {
		newValidatorSet, err := currentValidatorSet.Copy().ApplyDelta(delta)
		if err != nil {
			return nil, err
		}

		s.logger.Debug("New validator set", "validatorSet", newValidatorSet)
	}

	return delta, nil
}

// getTransferEventsFromReceipts parses logs from receipts to find transfer events
func (s *stakeManager) getTransferEventsFromReceipts(epoch uint64,
	receipts []*types.Receipt) ([]*contractsapi.TransferEvent, error) {
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

// getValidatorInfo returns data for new validator (bls key, is active) from the supernet contract
func (s *stakeManager) getNewValidatorInfo(address types.Address, stake *big.Int) (*ValidatorMetadata, error) {
	encoded, err := getValidatorABI.Encode([]interface{}{address})
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

	decoded, err := getValidatorABI.Outputs.Decode(byteResponse)
	if err != nil {
		return nil, err
	}

	//nolint:godox
	// TODO - @goran-ethernal change this to use the generated stub
	// once we remove old ChildValidatorSet stubs and generate new ones
	// from the new contract
	output, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert decoded outputs to map")
	}

	outputMap, ok := output["0"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert decoded outputs to map")
	}

	blsKey, ok := outputMap["blsKey"].([4]*big.Int)
	if !ok {
		return nil, fmt.Errorf("failed to decode blskey")
	}

	pubKey, err := bls.UnmarshalPublicKeyFromBigInt(blsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal BLS public key: %w", err)
	}

	return &ValidatorMetadata{
		Address:     address,
		VotingPower: stake,
		BlsKey:      pubKey,
		IsActive:    true,
	}, nil
}

// stakeInfo holds info about validator stake
// it holds sorted map of all stakes of all validators
type stakeInfo struct {
	pos     int
	stake   *big.Int
	address types.Address
}

// stakeCOunter sorts and returns stake info for all validators
type stakeCounter struct {
	stakeMap            map[types.Address]*stakeInfo
	currentValidatorSet AccountSet
}

// newStakeCounter returns a new instance of stake counter
func newStakeCounter(currentValidatorSet AccountSet) *stakeCounter {
	stakeCounter := &stakeCounter{
		currentValidatorSet: currentValidatorSet,
		stakeMap:            make(map[types.Address]*stakeInfo, 0),
	}

	for _, v := range currentValidatorSet {
		stakeCounter.stakeMap[v.Address] = &stakeInfo{
			stake:   new(big.Int).Set(v.VotingPower),
			address: v.Address,
		}
	}

	return stakeCounter
}

// addStake adds given amount for a validator to stake map
func (sc *stakeCounter) addStake(address types.Address, amount *big.Int) {
	sInfo, exists := sc.stakeMap[address]
	if !exists {
		sInfo = &stakeInfo{address: address, stake: amount}
		sc.stakeMap[address] = sInfo
	} else {
		sc.stakeMap[address].stake = sInfo.stake.Add(sInfo.stake, amount)
	}
}

// removeStake removes given amount for a validator from stake map
func (sc *stakeCounter) removeStake(address types.Address, amount *big.Int) {
	bigStake := sc.stakeMap[address].stake
	sc.stakeMap[address].stake = bigStake.Sub(bigStake, amount)
}

// sortByStake sorts all validators by their stake amount
func (sc *stakeCounter) sortByStake() {
	keys := make([]types.Address, 0, len(sc.stakeMap))
	for k := range sc.stakeMap {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		v1, v2 := sc.stakeMap[keys[i]], sc.stakeMap[keys[j]]
		return v1.stake.Cmp(v2.stake) > 1
	})

	for i, k := range keys {
		sc.stakeMap[k].pos = i
	}
}

// getStake returns stake info for given validator
func (sc *stakeCounter) getStake(address types.Address) *stakeInfo {
	return sc.stakeMap[address]
}

// iterateThroughNewValidators returns a slice of stake info of validators that are not
// in the current validator set
func (sc *stakeCounter) iterateThroughNewValidators() []*stakeInfo {
	newValidators := make([]*stakeInfo, 0)

	currentValidatorSetMap := sc.currentValidatorSet.GetAddressesAsSet()
	for a, s := range sc.stakeMap {
		if _, exists := currentValidatorSetMap[a]; exists {
			continue
		}

		newValidators = append(newValidators, s)
	}

	return newValidators
}
