package genesis

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/secrets/local"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	StatError   = "StatError"
	ExistsError = "ExistsError"
)

// GenesisGenError is a specific error type for generating genesis
type GenesisGenError struct {
	message   string
	errorType string
}

// GetMessage returns the message of the genesis generation error
func (g *GenesisGenError) GetMessage() string {
	return g.message
}

// GetType returns the type of the genesis generation error
func (g *GenesisGenError) GetType() string {
	return g.errorType
}

type premineInfo struct {
	address types.Address
	balance *big.Int
}

// verifyGenesisExistence checks if the genesis file at the specified path is present
func verifyGenesisExistence(genesisPath string) *GenesisGenError {
	_, err := os.Stat(genesisPath)
	if err != nil && !os.IsNotExist(err) {
		return &GenesisGenError{
			message:   fmt.Sprintf("failed to stat (%s): %v", genesisPath, err),
			errorType: StatError,
		}
	}

	if !os.IsNotExist(err) {
		return &GenesisGenError{
			message:   fmt.Sprintf("genesis file at path (%s) already exists", genesisPath),
			errorType: ExistsError,
		}
	}

	return nil
}

// fillPremineMap fills the premine map for the genesis.json file with passed in balances and accounts
func fillPremineMap(premineMap map[types.Address]*chain.GenesisAccount, premineInfos []*premineInfo) {
	for _, premine := range premineInfos {
		premineMap[premine.address] = &chain.GenesisAccount{
			Balance: premine.balance,
		}
	}
}

// parsePremineInfo parses provided premine information and returns premine address and premine balance
func parsePremineInfo(premineInfoRaw string) (*premineInfo, error) {
	address := types.ZeroAddress
	val := command.DefaultPremineBalance

	if delimiterIdx := strings.Index(premineInfoRaw, ":"); delimiterIdx != -1 {
		// <addr>:<balance>
		address, val = types.StringToAddress(premineInfoRaw[:delimiterIdx]), premineInfoRaw[delimiterIdx+1:]
	} else {
		// <addr>
		address = types.StringToAddress(premineInfoRaw)
	}

	amount, err := types.ParseUint256orHex(&val)
	if err != nil {
		return nil, fmt.Errorf("failed to parse amount %s: %w", val, err)
	}

	return &premineInfo{address: address, balance: amount}, nil
}

func ReadValidatorsByRegexp(dir, prefix string) ([]*polybft.Validator, error) {
	if dir == "" {
		dir = "."
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	validCnt := 0

	for _, file := range files {
		if file.IsDir() && strings.HasPrefix(file.Name(), prefix) {
			files[validCnt] = file
			validCnt++
		}
	}

	files = files[0:validCnt]

	// we must sort files by number after the prefix not by name string
	sort.Slice(files, func(i, j int) bool {
		f := strings.TrimPrefix(files[i].Name(), prefix)
		s := strings.TrimPrefix(files[j].Name(), prefix)
		num1, _ := strconv.Atoi(strings.TrimLeft(f, "-"))
		num2, _ := strconv.Atoi(strings.TrimLeft(s, "-"))

		return num1 < num2
	})

	validators := make([]*polybft.Validator, len(files))

	for i, file := range files {
		path := filepath.Join(dir, file.Name())

		account, nodeID, err := getSecrets(path)
		if err != nil {
			return nil, err
		}

		validator := &polybft.Validator{
			Address: types.Address(account.Ecdsa.Address()),
			BlsKey:  hex.EncodeToString(account.Bls.PublicKey().Marshal()),
			NodeID:  nodeID,
		}
		validators[i] = validator
	}

	return validators, nil
}

func getSecrets(directory string) (*wallet.Account, string, error) {
	baseConfig := &secrets.SecretsManagerParams{
		Logger: hclog.NewNullLogger(),
		Extra: map[string]interface{}{
			secrets.Path: directory,
		},
	}

	localManager, err := local.SecretsManagerFactory(nil, baseConfig)
	if err != nil {
		return nil, "", fmt.Errorf("unable to instantiate local secrets manager, %w", err)
	}

	nodeID, err := helper.LoadNodeID(localManager)
	if err != nil {
		return nil, "", err
	}

	account, err := wallet.NewAccountFromSecret(localManager)
	if err != nil {
		return nil, "", err
	}

	return account, nodeID, nil
}
