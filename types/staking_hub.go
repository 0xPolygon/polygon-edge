package types

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StakingHub acts as a hub (manager) for staked account balances
type StakingHub struct {
	// Address -> Stake
	StakingMap map[Address]*big.Int

	// The lowest staked amount in the validator set
	StakingThreshold *big.Int

	// Specifies the working directory for the Polygon SDK (location for writeback)
	WorkingDirectory string

	// Write-back period (in s) for backup staking data
	WritebackPeriod time.Duration

	// Mutex
	StakingMutex sync.Mutex

	// Close channel
	CloseCh chan struct{}
}

var stakingHubInstance StakingHub

// GetStakingHub initializes the stakingHubInstance singleton
func GetStakingHub() *StakingHub {
	var once sync.Once
	once.Do(func() {
		stakingHubInstance = StakingHub{
			StakingMap:       make(map[Address]*big.Int),
			StakingThreshold: big.NewInt(0),
			CloseCh:          make(chan struct{}),
		}
	})

	return &stakingHubInstance
}

func (sh *StakingHub) SetWorkingDirectory(directory string) {
	sh.WorkingDirectory = directory

	//go sh.SaveToDisk()
}

func (sh *StakingHub) CloseStakingHub() {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	// Alert the closing channel
	sh.CloseCh <- struct{}{}

	close(sh.CloseCh)
}

// SaveToDisk is a helper method for periodically saving the stake data to disk
func (sh *StakingHub) SaveToDisk() {
	for {
		select {
		case <-sh.CloseCh:
			return
		default:
		}

		// Save the current staking map to disk, in JSON
		mappings := sh.GetStakerMappings()

		reader, err := sh.marshalJSON(mappings)
		if err != nil {
			continue
		}

		// Save the json to workingDirectory/StakingMap.json
		file, err := os.Create(filepath.Join(sh.WorkingDirectory, "StakingMap.json"))
		if err != nil {
			_ = file.Close()
			continue
		}
		_, _ = io.Copy(file, reader)

		// Sleep for the writeback period
		time.Sleep(sh.WritebackPeriod * time.Second)
	}
}

// marshalJSON generates the json object for staker mappings
func (sh *StakingHub) marshalJSON(mappings []StakerMapping) (io.Reader, error) {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	b, err := json.MarshalIndent(mappings, "", "\t")
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// isStaker is a helper method to check whether or not an address has a staked balance
func (sh *StakingHub) isStaker(address Address) bool {
	if _, ok := sh.StakingMap[address]; ok {
		return true
	}

	return false
}

// IncreaseStake increases the account's staked balance, or sets it if the account wasn't
// in the StakingMap
func (sh *StakingHub) IncreaseStake(address Address, stakeBalance *big.Int) {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	if !sh.isStaker(address) {
		sh.StakingMap[address] = stakeBalance
	} else {
		sh.StakingMap[address] = big.NewInt(0).Add(sh.StakingMap[address], stakeBalance)
	}
}

// DecreaseStake decreases the account's staked balance if the account is present
func (sh *StakingHub) DecreaseStake(address Address, unstakeBalance *big.Int) {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	if sh.isStaker(address) {
		sh.StakingMap[address] = big.NewInt(0).Sub(sh.StakingMap[address], unstakeBalance)
	}
}

// ResetStake resets the account's staked balance
func (sh *StakingHub) ResetStake(address Address) {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	if sh.isStaker(address) {
		delete(sh.StakingMap, address)
	}
}

// GetStakedBalance returns an accounts staked balance if it is a staker.
// Returns 0 if the address is not a staker
func (sh *StakingHub) GetStakedBalance(address Address) *big.Int {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	if sh.isStaker(address) {
		return sh.StakingMap[address]
	}

	return big.NewInt(0)
}

// GetStakerAddresses returns a list of all addresses that have a stake > 0
func (sh *StakingHub) GetStakerAddresses() []Address {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	stakers := make([]Address, len(sh.StakingMap))

	var indx = 0
	for address := range sh.StakingMap {
		stakers[indx] = address
		indx++
	}

	return stakers
}

// StakerMapping is a representation of a staked account balance
type StakerMapping struct {
	Address Address  `json:"address"`
	Stake   *big.Int `json:"stake"`
}

// GetStakerMappings returns the staking addresses and their staking balances
func (sh *StakingHub) GetStakerMappings() []StakerMapping {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	mappings := make([]StakerMapping, len(sh.StakingMap))

	var indx = 0
	for address, stake := range sh.StakingMap {
		mappings[indx] = StakerMapping{
			Address: address,
			Stake:   stake,
		}
		indx++
	}

	return mappings
}
