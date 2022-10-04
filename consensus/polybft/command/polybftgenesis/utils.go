package polybftgenesis

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
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
func fillPremineMap(
	premineMap map[types.Address]*chain.GenesisAccount,
	premine []string,
) error {
	for _, prem := range premine {
		var addr types.Address

		val := command.DefaultPremineBalance

		if indx := strings.Index(prem, ":"); indx != -1 {
			// <addr>:<balance>
			addr, val = types.StringToAddress(prem[:indx]), prem[indx+1:]
		} else {
			// <addr>
			addr = types.StringToAddress(prem)
		}

		amount, err := types.ParseUint256orHex(&val)
		if err != nil {
			return fmt.Errorf("failed to parse amount %s: %w", val, err)
		}

		premineMap[addr] = &chain.GenesisAccount{
			Balance: amount,
		}
	}

	return nil
}

type GenesisTarget struct {
	Account *wallet.Account
	NodeID  string
}

func ReadValidatorsByRegexp(dir, prefix string) ([]GenesisTarget, error) {
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

	files = files[0:validCnt] // files is local variable, gc will collect everything after function returns

	// we must sort files by number after the prefix not by name string -> eq test-dir-10 should be larger than test-dir-4
	sort.Slice(files, func(i, j int) bool {
		f := strings.TrimPrefix(files[i].Name(), prefix)
		s := strings.TrimPrefix(files[j].Name(), prefix)
		num1, _ := strconv.Atoi(strings.TrimLeft(f, "-"))
		num2, _ := strconv.Atoi(strings.TrimLeft(s, "-"))

		return num1 < num2
	})

	validators := make([]GenesisTarget, 0, len(files))

	for _, file := range files {
		path := filepath.Join(dir, file.Name())

		account, nodeID, err := getSecrets(path)
		if err != nil {
			return nil, err
		}

		target := GenesisTarget{Account: account, NodeID: nodeID}
		validators = append(validators, target)
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

	account, err := wallet.GenerateAccountFromSecrets(localManager)
	if err != nil {
		return nil, "", err
	}

	return account, nodeID, nil
}
