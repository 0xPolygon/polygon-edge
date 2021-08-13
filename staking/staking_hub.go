package staking

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

type StakingEventType string

var (
	StakingEvent   StakingEventType = "staking"
	UnstakingEvent StakingEventType = "unstaking"
	UnknownEvent   StakingEventType = "unknown"
)

var (
	defaultFileName = "staking-map.json"
)

// StakingHub acts as a hub (manager) for staked account balances
type StakingHub struct {
	// Address -> Stake
	StakingMap map[types.Address]*big.Int

	// Dirty staking map that's modified during block execution
	// Address -> Stake
	DirtyStakingMap map[types.Address]*big.Int

	// Specifies the working directory for the Polygon SDK (location for writeback)
	WorkingDirectory string

	// Write-back period (in s) for backup staking data
	WritebackPeriod time.Duration

	// Logger for logging errors and information
	Logger hclog.Logger

	// Event list mutex
	EventQueueMutex sync.Mutex

	// Staking map mutex
	StakingMutex sync.Mutex

	// Dirty staking map mutex
	DirtyStakingMutex sync.Mutex

	// Close channel
	CloseCh chan struct {
	}
}

var stakingHubInstance StakingHub
var once sync.Once

// GetStakingHub initializes the stakingHubInstance singleton
func GetStakingHub() *StakingHub {
	once.Do(func() {
		stakingHubInstance = StakingHub{
			StakingMap:      make(map[types.Address]*big.Int),
			DirtyStakingMap: make(map[types.Address]*big.Int),
			CloseCh:         make(chan struct{}),
			WritebackPeriod: 60 * time.Second,
		}
	})

	return &stakingHubInstance
}

// SetWorkingDirectory sets the writeback directory for the staking map
func (sh *StakingHub) SetWorkingDirectory(directory string) {
	sh.WorkingDirectory = directory

	err := sh.readFromDisk()
	if err != nil {
		// Log as warning because reading this map is not crucial
		sh.log(err.Error(), logWarning)
	}

	go sh.saveToDisk()
}

// LogType defines possible log types for the logger
type LogType string

var (
	logInfo    LogType = "info"
	logError   LogType = "error"
	logWarning LogType = "warning"
)

// log logs the output to the console if the logger is set
func (sh *StakingHub) log(output string, logType LogType) {
	if sh.Logger != nil {
		switch logType {
		case logInfo:
			sh.Logger.Info(output)
		case logError:
			sh.Logger.Error(output)
		case logWarning:
			sh.Logger.Warn(output)
		}
	}
}

// SetLogger sets the StakingHub logger
func (sh *StakingHub) SetLogger(logger hclog.Logger) {
	sh.Logger = logger.Named("staking-hub")
}

// CloseStakingHub stops the writeback process
func (sh *StakingHub) CloseStakingHub() {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	// Alert the closing channel
	select {
	case sh.CloseCh <- struct{}{}:
		sh.log("Closing staking hub...", logInfo)
	default:
		sh.log("SH Writer not set up. Closing staking hub...", logInfo)
	}
}

// saveToDisk is a helper method for periodically saving the stake data to disk
func (sh *StakingHub) saveToDisk() {
	for {
		select {
		case <-sh.CloseCh:
			close(sh.CloseCh)
			return
		default:
		}

		// Save the current staking map to disk, in JSON
		mappings := sh.getStakerMappings()

		reader, err := sh.marshalJSON(mappings)
		if err != nil {
			continue
		}

		// Save the json to workingDirectory/staking-map.json
		file, err := os.Create(filepath.Join(sh.WorkingDirectory, defaultFileName))
		if err != nil {
			_ = file.Close()
			sh.log("unable to create writeback file", logError)

			continue
		}

		_, err = io.Copy(file, reader)
		if err != nil {
			sh.log("unable to write date into writeback file", logError)
		}

		err = file.Close()
		if err != nil {
			sh.log("unable to close writeback file", logError)
		}

		// Sleep for the writeback period
		time.Sleep(sh.WritebackPeriod * time.Second)
	}
}

// readFromDisk reads the staking map from a previously saved disk copy, if it exists
func (sh *StakingHub) readFromDisk() error {
	// Check if the file exists
	mapFile, err := os.Open(filepath.Join(sh.WorkingDirectory, defaultFileName))
	if err != nil {
		return fmt.Errorf("no exiting map file is present")
	}

	byteValue, err := ioutil.ReadAll(mapFile)
	if err != nil {
		return fmt.Errorf("unable to read map file")
	}

	var stakingMap []stakerMapping

	// Unmarshal the json
	if err = json.Unmarshal(byteValue, &stakingMap); err != nil {
		return fmt.Errorf("unable to unmarshal staking map")
	}

	// Close the file
	if err = mapFile.Close(); err != nil {
		return fmt.Errorf("unable to close staking file")
	}

	// Update the staking map using the read data
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	for _, val := range stakingMap {
		sh.StakingMap[val.Address] = val.Stake
	}

	return nil
}

// marshalJSON generates the json object for staker mappings
func (sh *StakingHub) marshalJSON(mappings []stakerMapping) (io.Reader, error) {
	b, err := json.MarshalIndent(mappings, "", "\t")
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// isStaker is a helper method to check whether or not an address has a staked balance.
// The staking status is block based, meaning that if the address is a staker at block A, this
// method will return true for all calls within the context of block A.
func (sh *StakingHub) isStaker(address types.Address) bool {
	if _, ok := sh.StakingMap[address]; ok {
		return sh.StakingMap[address].Cmp(big.NewInt(0)) > 0
	}

	return false
}

// IncreaseStake increases the account's staked balance, or sets it if the account wasn't
// in the StakingMap
func (sh *StakingHub) IncreaseStake(address types.Address, stakeBalance *big.Int) {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	if !sh.isStaker(address) {
		sh.StakingMap[address] = stakeBalance
	} else {
		sh.StakingMap[address] = big.NewInt(0).Add(sh.StakingMap[address], stakeBalance)
	}

	sh.log(
		fmt.Sprintf("Stake increase:\t%s %s", address.String(), stakeBalance.String()),
		logInfo,
	)
}

// getDirtyReferenceBalance returns the staked balance:
//
// - if the account's staked balance is dirty, return the dirty staked balance
//
// - if the account's staked balance is not dirty, return the clean staked balance
func (sh *StakingHub) getDirtyReferenceBalance(address types.Address) *big.Int {
	var referenceBalance *big.Int
	if sh.isDirtyStaker(address) {
		// The staking entry is dirty, grab it
		referenceBalance = sh.DirtyStakingMap[address]
	} else {
		// The staking entry is not dirty, get the clean record
		referenceBalance = sh.getCleanStake(address)
	}

	return referenceBalance
}

// isDirtyStaker checks if the address has had its stake dirtied during the current block execution
func (sh *StakingHub) isDirtyStaker(address types.Address) bool {
	_, ok := sh.DirtyStakingMap[address]

	return ok
}

// getCleanStake retrieves the clean staked balance for an address
func (sh *StakingHub) getCleanStake(address types.Address) *big.Int {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	if !sh.isStaker(address) {
		return big.NewInt(0)
	}

	return sh.StakingMap[address]
}

// IncreaseDirtyStake increases the account's dirty staked balance during the block execution
func (sh *StakingHub) IncreaseDirtyStake(address types.Address, stakeBalance *big.Int) {
	sh.DirtyStakingMutex.Lock()
	defer sh.DirtyStakingMutex.Unlock()

	referenceBalance := sh.getDirtyReferenceBalance(address)

	sh.DirtyStakingMap[address] = big.NewInt(0).Add(referenceBalance, stakeBalance)

	sh.log(
		fmt.Sprintf("Dirty stake increase:\t%s %s", address.String(), stakeBalance.String()),
		logInfo,
	)
}

// CommitDirtyStakes commits any staking balance changes to the main staking map
func (sh *StakingHub) CommitDirtyStakes() {
	sh.DirtyStakingMutex.Lock()
	defer sh.DirtyStakingMutex.Unlock()

	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	// Commit the dirty map to the clean one
	for address, dirtyStake := range sh.DirtyStakingMap {
		sh.StakingMap[address] = dirtyStake
	}

	// Clear out the dirty map
	sh.DirtyStakingMap = make(map[types.Address]*big.Int)
}

// DecreaseStake decreases the account's staked balance if the account is present
func (sh *StakingHub) DecreaseStake(address types.Address, unstakeBalance *big.Int) {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	if sh.isStaker(address) && sh.StakingMap[address].Cmp(unstakeBalance) >= 0 {
		sh.StakingMap[address] = big.NewInt(0).Sub(sh.StakingMap[address], unstakeBalance)

		sh.log(
			fmt.Sprintf("Stake decrease:\t%s %s", address.String(), unstakeBalance.String()),
			logInfo,
		)
	}
}

// ResetDirtyStake resets the account's dirty staked balance during block execution
func (sh *StakingHub) ResetDirtyStake(address types.Address) {
	sh.DirtyStakingMutex.Lock()
	defer sh.DirtyStakingMutex.Unlock()

	sh.DirtyStakingMap[address] = big.NewInt(0)

	sh.log(
		fmt.Sprintf("Dirty take reset:\t%s", address.String()),
		logInfo,
	)
}

// ResetStake resets the account's staked balance
func (sh *StakingHub) ResetStake(address types.Address) {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	if sh.isStaker(address) {
		delete(sh.StakingMap, address)
	}

	sh.log(
		fmt.Sprintf("Stake reset:\t%s", address.String()),
		logInfo,
	)
}

// GetStakedBalance returns an accounts staked balance if it is a staker.
// Returns 0 if the address is not a staker
func (sh *StakingHub) GetStakedBalance(address types.Address) *big.Int {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	if sh.isStaker(address) {
		return sh.StakingMap[address]
	}

	return big.NewInt(0)
}

// GetDirtyStakedBalance returns an account's dirty staked balance if it is a staker.
// Returns 0 if the address is not a staker
func (sh *StakingHub) GetDirtyStakedBalance(address types.Address) *big.Int {
	sh.DirtyStakingMutex.Lock()
	defer sh.DirtyStakingMutex.Unlock()

	return sh.getDirtyReferenceBalance(address)
}

// stakerMapping is a representation of a staked account balance
type stakerMapping struct {
	Address types.Address `json:"address"`
	Stake   *big.Int      `json:"stake"`
}

// GetStakerMappings returns the staking addresses and their staking balances
func (sh *StakingHub) getStakerMappings() []stakerMapping {
	sh.StakingMutex.Lock()
	defer sh.StakingMutex.Unlock()

	mappings := make([]stakerMapping, len(sh.StakingMap))

	var indx = 0
	for address, stake := range sh.StakingMap {
		mappings[indx] = stakerMapping{
			Address: address,
			Stake:   stake,
		}
		indx++
	}

	return mappings
}

// ClearDirtyStakes resets the dirty staking map
func (sh *StakingHub) ClearDirtyStakes() {
	sh.DirtyStakingMutex.Lock()
	defer sh.DirtyStakingMutex.Unlock()

	sh.DirtyStakingMap = make(map[types.Address]*big.Int)
}
