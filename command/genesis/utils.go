package genesis

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

const (
	StatError   = "StatError"
	ExistsError = "ExistsError"
)

var (
	ErrECDSAKeyNotFound = errors.New("ECDSA key not found in given path")
	ErrBLSKeyNotFound   = errors.New("BLS key not found in given path")
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

// getValidatorsFromPrefixPath extracts the addresses of the validators based on the directory
// prefix. It scans the directories for validator private keys and compiles a list of addresses
func getValidatorsFromPrefixPath(
	prefix string,
	validatorType validators.ValidatorType,
) (validators.ValidatorSet, error) {
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return nil, err
	}

	validatorSet := validators.NewValidatorSetFromType(validatorType)

	for _, file := range files {
		path := file.Name()

		if !file.IsDir() {
			continue
		}

		if !strings.HasPrefix(path, prefix) {
			continue
		}

		localSecretsManager, err := helper.SetupLocalSecretsManager(path)
		if err != nil {
			return nil, err
		}

		address, err := getValidatorAddressFromSecretManager(localSecretsManager)
		if err != nil {
			return nil, err
		}

		switch validatorType {
		case validators.ECDSAValidatorType:
			validatorSet.Add(&validators.ECDSAValidator{
				Address: address,
			})
		case validators.BLSValidatorType:
			blsPublicKey, err := getBLSPublicKeyBytesFromSecretManager(localSecretsManager)
			if err != nil {
				return nil, err
			}

			validatorSet.Add(&validators.BLSValidator{
				Address:      address,
				BLSPublicKey: blsPublicKey,
			})
		}
	}

	return validatorSet, nil
}

func getValidatorAddressFromSecretManager(manager secrets.SecretsManager) (types.Address, error) {
	if !manager.HasSecret(secrets.ValidatorKey) {
		return types.ZeroAddress, ErrECDSAKeyNotFound
	}

	keyBytes, err := manager.GetSecret(secrets.ValidatorKey)
	if err != nil {
		return types.ZeroAddress, err
	}

	privKey, err := crypto.BytesToECDSAPrivateKey(keyBytes)
	if err != nil {
		return types.ZeroAddress, err
	}

	return crypto.PubKeyToAddress(&privKey.PublicKey), nil
}

func getBLSPublicKeyBytesFromSecretManager(manager secrets.SecretsManager) ([]byte, error) {
	if !manager.HasSecret(secrets.ValidatorBLSKey) {
		return nil, ErrBLSKeyNotFound
	}

	keyBytes, err := manager.GetSecret(secrets.ValidatorKey)
	if err != nil {
		return nil, err
	}

	secretKey, err := crypto.BytesToBLSSecretKey(keyBytes)
	if err != nil {
		return nil, err
	}

	pubKey, err := secretKey.GetPublicKey()
	if err != nil {
		return nil, err
	}

	pubKeyBytes, err := pubKey.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return pubKeyBytes, nil
}

func ParseValidator(t validators.ValidatorType, s string) (validators.Validator, error) {
	switch t {
	case validators.ECDSAValidatorType:
		return ParseECDSAValidator(s)
	case validators.BLSValidatorType:
		return ParseBLSValidator(s)
	default:
		return nil, fmt.Errorf("invalid validator type: %s", t)
	}
}

func ParseECDSAValidator(s string) (*validators.ECDSAValidator, error) {
	bytes, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return nil, err
	}

	return &validators.ECDSAValidator{
		Address: types.BytesToAddress(bytes),
	}, nil
}

func ParseBLSValidator(s string) (*validators.BLSValidator, error) {
	subValues := strings.Split(s, ":")

	if len(subValues) != 2 {
		return nil, fmt.Errorf("invalid validator format, expected [Validator Address]:[BLS Public Key]")
	}

	addrBytes, err := hex.DecodeString(strings.TrimPrefix(subValues[0], "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse address: %w", err)
	}

	pubKeyBytes, err := hex.DecodeString(strings.TrimPrefix(subValues[1], "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse BLS Public Key: %w", err)
	}

	return &validators.BLSValidator{
		Address:      types.BytesToAddress(addrBytes),
		BLSPublicKey: pubKeyBytes,
	}, nil
}
