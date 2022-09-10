package output

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	dataDirFlag   = "data-dir"
	configFlag    = "config"
	validatorFlag = "validator"
	blsFlag       = "bls"
	nodeIDFlag    = "node-id"
)

var (
	params = &outputParams{}
)

var (
	errInvalidConfig   = errors.New("invalid secrets configuration")
	errInvalidParams   = errors.New("no config file or data directory passed in")
	errUnsupportedType = errors.New("unsupported secrets manager")
)

type outputParams struct {
	dataDir    string
	configPath string

	outputNodeID    bool
	outputValidator bool
	outputBLS       bool

	secretsManager secrets.SecretsManager
	secretsConfig  *secrets.SecretsManagerConfig

	validatorAddress string
	blsPubkey        string

	nodeID string
}

func (op *outputParams) validateFlags() error {
	if op.dataDir == "" && op.configPath == "" {
		return errInvalidParams
	}

	return nil
}

func (op *outputParams) initSecrets() error {
	if err := op.initSecretsManager(); err != nil {
		return err
	}

	if err := op.initValidatorAddress(); err != nil {
		return err
	}

	if err := op.initBLSPublicKey(); err != nil {
		return err
	}

	return op.initNodeID()
}

func (op *outputParams) initSecretsManager() error {
	var err error
	if op.hasConfigPath() {
		if err = op.parseConfig(); err != nil {
			return err
		}

		op.secretsManager, err = helper.InitCloudSecretsManager(op.secretsConfig)

		return err
	}

	return op.initLocalSecretsManager()
}

func (op *outputParams) hasConfigPath() bool {
	return op.configPath != ""
}

func (op *outputParams) parseConfig() error {
	secretsConfig, readErr := secrets.ReadConfig(op.configPath)
	if readErr != nil {
		return errInvalidConfig
	}

	if !secrets.SupportedServiceManager(secretsConfig.Type) {
		return errUnsupportedType
	}

	op.secretsConfig = secretsConfig

	return nil
}

func (op *outputParams) initLocalSecretsManager() error {
	validatorPathPrefix := filepath.Join(op.dataDir, secrets.ConsensusFolderLocal)
	networkPathPrefix := filepath.Join(op.dataDir, secrets.NetworkFolderLocal)
	dataDirAbs, _ := filepath.Abs(op.dataDir)

	if !common.DirectoryExists(op.dataDir) {
		return fmt.Errorf("the data directory provided does not exist: %s", dataDirAbs)
	}

	errs := make([]string, 0, 2)
	if !common.DirectoryExists(validatorPathPrefix) {
		errs = append(errs, fmt.Sprintf("no validator keys found in the data directory provided: %s", dataDirAbs))
	}

	if !common.DirectoryExists(networkPathPrefix) {
		errs = append(errs, fmt.Sprintf("no network key found in the data directory provided: %s", dataDirAbs))
	}

	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "\n"))
	}

	local, err := helper.SetupLocalSecretsManager(op.dataDir)
	if err != nil {
		return err
	}

	op.secretsManager = local

	return nil
}

func (op *outputParams) initValidatorAddress() error {
	validatorAddress, err := helper.LoadValidatorAddress(op.secretsManager)
	if err != nil {
		return err
	}

	if validatorAddress == types.ZeroAddress {
		op.validatorAddress = ""
	} else {
		op.validatorAddress = validatorAddress.String()
	}

	return nil
}

func (op *outputParams) initBLSPublicKey() error {
	blsPubkey, err := helper.LoadBLSPublicKey(op.secretsManager)
	if err != nil {
		return err
	}

	op.blsPubkey = blsPubkey

	return nil
}

func (op *outputParams) initNodeID() error {
	nodeID, err := helper.LoadNodeID(op.secretsManager)
	if err != nil {
		return err
	}

	op.nodeID = nodeID

	return nil
}

func (op *outputParams) getResult() command.CommandResult {
	outputResults := &SecretsOutputResult{
		outputValidator: op.outputValidator,
		outputBLS:       op.outputBLS,
		outputNodeID:    op.outputNodeID,
	}

	if op.outputNodeID {
		outputResults.NodeID = op.nodeID

		return outputResults
	}

	if op.outputValidator {
		outputResults.Address = op.validatorAddress

		return outputResults
	}

	if op.outputBLS {
		outputResults.BLSPubkey = op.blsPubkey

		return outputResults
	}

	outputResults.NodeID = op.nodeID
	outputResults.BLSPubkey = op.blsPubkey
	outputResults.Address = op.validatorAddress

	return outputResults
}
