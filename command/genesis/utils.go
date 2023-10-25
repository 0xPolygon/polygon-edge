package genesis

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/common"
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

// parseTrackerStartBlocks parses provided event tracker start blocks configuration.
// It is set in a following format: <contractAddress>:<startBlock>.
// In case smart contract address isn't provided in the string, it is assumed its starting block is 0 implicitly.
func parseTrackerStartBlocks(trackerStartBlocksRaw []string) (map[types.Address]uint64, error) {
	trackerStartBlocksConfig := make(map[types.Address]uint64, len(trackerStartBlocksRaw))

	for _, startBlockRaw := range trackerStartBlocksRaw {
		delimiterIdx := strings.Index(startBlockRaw, ":")
		if delimiterIdx == -1 {
			return nil, fmt.Errorf("invalid event tracker start block configuration provided: %s", trackerStartBlocksRaw)
		}

		// <contractAddress>:<startBlock>
		address := types.StringToAddress(startBlockRaw[:delimiterIdx])
		startBlockRaw := startBlockRaw[delimiterIdx+1:]

		startBlock, err := strconv.ParseUint(startBlockRaw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse provided start block %s: %w", startBlockRaw, err)
		}

		trackerStartBlocksConfig[address] = startBlock
	}

	return trackerStartBlocksConfig, nil
}

// parseBurnContractInfo parses provided burn contract information and returns burn contract block and address
func parseBurnContractInfo(burnContractInfoRaw string) (*polybft.BurnContractInfo, error) {
	// <block>:<address>[:<burn destination address>]
	burnContractParts := strings.Split(burnContractInfoRaw, ":")
	if len(burnContractParts) < 2 || len(burnContractParts) > 3 {
		return nil, fmt.Errorf("expected format: <block>:<address>[:<burn destination>]")
	}

	blockRaw := burnContractParts[0]

	blockNum, err := common.ParseUint64orHex(&blockRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse block number %s: %w", blockRaw, err)
	}

	contractAddress := burnContractParts[1]
	if err = types.IsValidAddress(contractAddress); err != nil {
		return nil, fmt.Errorf("failed to parse contract address %s: %w", contractAddress, err)
	}

	if len(burnContractParts) == 2 {
		return &polybft.BurnContractInfo{
			BlockNumber:        blockNum,
			Address:            types.StringToAddress(contractAddress),
			DestinationAddress: types.ZeroAddress,
		}, nil
	}

	destinationAddress := burnContractParts[2]
	if err = types.IsValidAddress(destinationAddress); err != nil {
		return nil, fmt.Errorf("failed to parse burn destination address %s: %w", destinationAddress, err)
	}

	return &polybft.BurnContractInfo{
		BlockNumber:        blockNum,
		Address:            types.StringToAddress(contractAddress),
		DestinationAddress: types.StringToAddress(destinationAddress),
	}, nil
}

type baseFeeInfo struct {
	baseFee            uint64
	baseFeeEM          uint64
	baseFeeChangeDenom uint64
}

// parseBaseFeeConfig parses provided base fee configuration and returns baseFeeInfo
func parseBaseFeeConfig(baseFeeConfigRaw string) (*baseFeeInfo, error) {
	baseFeeInfo := &baseFeeInfo{
		command.DefaultGenesisBaseFee,
		command.DefaultGenesisBaseFeeEM,
		command.DefaultGenesisBaseFeeChangeDenom,
	}

	baseFeeConfig := strings.Split(baseFeeConfigRaw, ":")
	if len(baseFeeConfig) > 3 {
		return baseFeeInfo, errors.New("invalid number of arguments for base fee configuration")
	}

	if len(baseFeeConfig) >= 1 && baseFeeConfig[0] != "" {
		baseFee, err := common.ParseUint64orHex(&baseFeeConfig[0])
		if err != nil {
			return baseFeeInfo, err
		}

		baseFeeInfo.baseFee = baseFee
	}

	if len(baseFeeConfig) >= 2 && baseFeeConfig[1] != "" {
		baseFeeEM, err := common.ParseUint64orHex(&baseFeeConfig[1])
		if err != nil {
			return baseFeeInfo, err
		}

		baseFeeInfo.baseFeeEM = baseFeeEM
	}

	if len(baseFeeConfig) == 3 && baseFeeConfig[2] != "" {
		baseFeeChangeDenom, err := common.ParseUint64orHex(&baseFeeConfig[2])
		if err != nil {
			return baseFeeInfo, err
		}

		baseFeeInfo.baseFeeChangeDenom = baseFeeChangeDenom
	}

	return baseFeeInfo, nil
}

// GetValidatorKeyFiles returns file names which has validator secrets
func GetValidatorKeyFiles(rootDir, filePrefix string) ([]string, error) {
	if rootDir == "" {
		rootDir = "."
	}

	files, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}

	matchedFiles := 0
	fileNames := make([]string, len(files))

	for _, file := range files {
		fileName := file.Name()
		if file.IsDir() && strings.HasPrefix(fileName, filePrefix) {
			fileNames[matchedFiles] = fileName
			matchedFiles++
		}
	}
	// reslice to remove empty entries
	fileNames = fileNames[:matchedFiles]

	// we must sort files by number after the prefix not by name string
	sort.Slice(fileNames, func(i, j int) bool {
		first := strings.TrimPrefix(fileNames[i], filePrefix)
		second := strings.TrimPrefix(fileNames[j], filePrefix)
		num1, _ := strconv.Atoi(strings.TrimLeft(first, "-"))
		num2, _ := strconv.Atoi(strings.TrimLeft(second, "-"))

		return num1 < num2
	})

	return fileNames, nil
}

// ReadValidatorsByPrefix reads validators secrets on a given root directory and with given folder prefix
func ReadValidatorsByPrefix(dir, prefix string) ([]*validator.GenesisValidator, error) {
	validatorKeyFiles, err := GetValidatorKeyFiles(dir, prefix)
	if err != nil {
		return nil, err
	}

	validators := make([]*validator.GenesisValidator, len(validatorKeyFiles))

	for i, file := range validatorKeyFiles {
		path := filepath.Join(dir, file)

		account, nodeID, err := getSecrets(path)
		if err != nil {
			return nil, err
		}

		validators[i] = &validator.GenesisValidator{
			Address:   types.Address(account.Ecdsa.Address()),
			BlsKey:    hex.EncodeToString(account.Bls.PublicKey().Marshal()),
			MultiAddr: fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", "127.0.0.1", bootnodePortStart+int64(i), nodeID),
			Stake:     big.NewInt(0),
		}
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

	return account, nodeID, err
}

// GenerateExtraDataPolyBft populates Extra with specific fields required for polybft consensus protocol
func GenerateExtraDataPolyBft(validators []*validator.ValidatorMetadata) ([]byte, error) {
	delta := &validator.ValidatorSetDelta{
		Added:   validators,
		Removed: bitmap.Bitmap{},
	}

	extra := polybft.Extra{Validators: delta, Checkpoint: &polybft.CheckpointData{}}

	return extra.MarshalRLPTo(nil), nil
}
