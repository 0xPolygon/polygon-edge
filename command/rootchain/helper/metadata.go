package helper

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	polybftWallet "github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

//nolint:gosec
const (
	TestAccountPrivKey      = "aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d"
	TestModeFlag            = "test"
	SupernetManagerFlag     = "supernet-manager"
	SupernetManagerFlagDesc = "address of supernet manager contract"
	StakeManagerFlag        = "stake-manager"
	StakeManagerFlagDesc    = "address of stake manager contract"
	NativeRootTokenFlag     = "native-root-token"
	NativeRootTokenFlagDesc = "address of native root token"
)

var (
	ErrRootchainNotFound = errors.New("rootchain not found")
	ErrRootchainPortBind = errors.New("port 8545 is not bind with localhost")
	errTestModeSecrets   = errors.New("rootchain test mode does not imply specifying secrets parameters")

	rootchainAccountKey *wallet.Key
)

// GetRootchainPrivateKey initializes a private key from provided raw private key
func GetRootchainPrivateKey(rawKey string) (ethgo.Key, error) {
	privateKeyRaw := TestAccountPrivKey
	if rawKey != "" {
		privateKeyRaw = rawKey
	}

	dec, err := hex.DecodeString(privateKeyRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key string '%s': %w", privateKeyRaw, err)
	}

	rootchainAccountKey, err = wallet.NewWalletFromPrivKey(dec)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize key from provided private key '%s': %w", privateKeyRaw, err)
	}

	return rootchainAccountKey, nil
}

func GetRootchainID() (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("rootchain id error: %w", err)
	}

	containers, err := cli.ContainerList(context.Background(), dockertypes.ContainerListOptions{})
	if err != nil {
		return "", fmt.Errorf("rootchain id error: %w", err)
	}

	for _, c := range containers {
		if c.Labels["edge-type"] == "rootchain" {
			return c.ID, nil
		}
	}

	return "", ErrRootchainNotFound
}

func ReadRootchainIP() (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("rootchain id error: %w", err)
	}

	contID, err := GetRootchainID()
	if err != nil {
		return "", err
	}

	inspect, err := cli.ContainerInspect(context.Background(), contID)
	if err != nil {
		return "", fmt.Errorf("rootchain ip error: %w", err)
	}

	ports, ok := inspect.HostConfig.PortBindings["8545/tcp"]
	if !ok || len(ports) == 0 {
		return "", ErrRootchainPortBind
	}

	return fmt.Sprintf("http://%s:%s", ports[0].HostIP, ports[0].HostPort), nil
}

func GetECDSAKey(privateKey, accountDir, accountConfig string) (ethgo.Key, error) {
	if privateKey != "" {
		key, err := GetRootchainPrivateKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize private key: %w", err)
		}

		return key, nil
	}

	secretsManager, err := polybftsecrets.GetSecretsManager(accountDir, accountConfig, true)
	if err != nil {
		return nil, err
	}

	return polybftWallet.GetEcdsaFromSecret(secretsManager)
}

// GetValidatorInfo queries SupernetManager smart contract on root
// and retrieves validator info for given address
func GetValidatorInfo(validatorAddr, supernetManagerAddr ethgo.Address,
	txRelayer txrelayer.TxRelayer) (*polybft.ValidatorInfo, error) {
	getValidatorMethod := contractsapi.CustomSupernetManager.Abi.GetMethod("validators")

	encode, err := getValidatorMethod.Encode([]interface{}{validatorAddr})
	if err != nil {
		return nil, err
	}

	response, err := txRelayer.Call(ethgo.Address(contracts.SystemCaller),
		supernetManagerAddr, encode)
	if err != nil {
		return nil, err
	}

	byteResponse, err := hex.DecodeHex(response)
	if err != nil {
		return nil, fmt.Errorf("unable to decode hex response, %w", err)
	}

	decoded, err := getValidatorMethod.Outputs.Decode(byteResponse)
	if err != nil {
		return nil, err
	}

	decodedOutputsMap, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert decoded outputs to map")
	}

	return &polybft.ValidatorInfo{
		Address:       validatorAddr.Address(),
		Stake:         decodedOutputsMap["stake"].(*big.Int),     //nolint:forcetypeassert
		IsActive:      decodedOutputsMap["isActive"].(bool),      //nolint:forcetypeassert
		IsWhitelisted: decodedOutputsMap["isWhitelisted"].(bool), //nolint:forcetypeassert
	}, nil
}

// CreateMintTxn encodes parameters for mint function on rootchain token contract
func CreateMintTxn(receiver, rootTokenAddr types.Address, amount *big.Int) (*ethgo.Transaction, error) {
	mintFn := &contractsapi.MintRootERC20Fn{
		To:     receiver,
		Amount: amount,
	}

	input, err := mintFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(rootTokenAddr)

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}

// CreateApproveERC20Txn sends approve transaction
// to ERC20 token for spender so that it is able to spend given tokens
func CreateApproveERC20Txn(amount *big.Int,
	spender, rootERC20Token types.Address) (*ethgo.Transaction, error) {
	approveFnParams := &contractsapi.ApproveRootERC20Fn{
		Spender: spender,
		Amount:  amount,
	}

	input, err := approveFnParams.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode parameters for RootERC20.approve. error: %w", err)
	}

	addr := ethgo.Address(rootERC20Token)

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}
