package helper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/umbracle/ethgo"

	polybftsecrets "github.com/0xPolygon/polygon-edge/command/secrets/init"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	polybftWallet "github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

//nolint:gosec
const (
	TestAccountPrivKey      = "aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d"
	TestModeFlag            = "test"
	GenesisPathFlag         = "genesis"
	GenesisPathFlagDesc     = "genesis file path, which contains chain configuration"
	DefaultGenesisPath      = "./genesis.json"
	ProxyContractsAdminFlag = "proxy-contracts-admin"
	ProxyContractsAdminDesc = "admin for proxy contracts"
	StakeTokenFlag          = "stake-token"
	StakeTokenFlagDesc      = "address of ERC20 token used for staking"
	AddressesFlag           = "addresses"
	AmountsFlag             = "amounts"
	Erc20TokenFlag          = "erc20-token" //nolint:gosec
	BladeManagerFlag        = "blade-manager"
	BladeManagerFlagDesc    = "address of blade manager contract on a rootchain"
	TxTimeoutFlag           = "tx-timeout"
	TxPollFreqFlag          = "tx-poll-freq"
)

var (
	ErrRootchainNotFound = errors.New("rootchain not found")
	ErrRootchainPortBind = errors.New("port 8545 is not bind with localhost")
	errTestModeSecrets   = errors.New("rootchain test mode does not imply specifying secrets parameters")

	ErrNoAddressesProvided = errors.New("no addresses provided")
	ErrInconsistentLength  = errors.New("addresses and amounts must be equal length")

	rootchainAccountKey *crypto.ECDSAKey
)

type MessageResult struct {
	Message string `json:"message"`
}

func (r MessageResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString(r.Message)
	buffer.WriteString("\n")

	return buffer.String()
}

// DecodePrivateKey decodes a private key from provided raw private key
func DecodePrivateKey(rawKey string) (crypto.Key, error) {
	privateKeyRaw := TestAccountPrivKey
	if rawKey != "" {
		privateKeyRaw = rawKey
	}

	dec, err := hex.DecodeString(privateKeyRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key string '%s': %w", privateKeyRaw, err)
	}

	rootchainAccountKey, err = crypto.NewECDSAKeyFromRawPrivECDSA(dec)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize key from provided private key '%s': %w", privateKeyRaw, err)
	}

	return rootchainAccountKey, nil
}

func GetRootchainID() (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
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

// GetECDSAKey returns the key based on provided parameters
// If private key is provided, it will return that key
// if not, it will return the key from the secrets manager
func GetECDSAKey(privateKey, accountDir, accountConfig string) (crypto.Key, error) {
	if privateKey != "" {
		key, err := DecodePrivateKey(privateKey)
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
func GetValidatorInfo(validatorAddr types.Address, supernetManagerAddr, stakeManagerAddr types.Address,
	txRelayer txrelayer.TxRelayer) (*polybft.ValidatorInfo, error) {
	caller := contracts.SystemCaller
	getValidatorMethod := contractsapi.StakeManager.Abi.GetMethod("stakeOf")

	encode, err := getValidatorMethod.Encode([]interface{}{validatorAddr})
	if err != nil {
		return nil, err
	}

	response, err := txRelayer.Call(caller, supernetManagerAddr, encode)
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

	innerMap, ok := decodedOutputsMap["0"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert decoded outputs map to inner map")
	}

	//nolint:forcetypeassert
	validatorInfo := &polybft.ValidatorInfo{
		Address:       validatorAddr,
		IsActive:      innerMap["isActive"].(bool),
		IsWhitelisted: innerMap["isWhitelisted"].(bool),
	}

	stakeOfFn := &contractsapi.StakeOfStakeManagerFn{
		Validator: types.Address(validatorAddr),
	}

	encode, err = stakeOfFn.EncodeAbi()
	if err != nil {
		return nil, err
	}

	response, err = txRelayer.Call(caller, stakeManagerAddr, encode)
	if err != nil {
		return nil, err
	}

	stake, err := common.ParseUint256orHex(&response)
	if err != nil {
		return nil, err
	}

	validatorInfo.Stake = stake

	return validatorInfo, nil
}

// CreateMintTxn encodes parameters for mint function on rootchain token contract
func CreateMintTxn(receiver, erc20TokenAddr types.Address,
	amount *big.Int, rootchainTx bool) (*types.Transaction, error) {
	mintFn := &contractsapi.MintRootERC20Fn{
		To:     receiver,
		Amount: amount,
	}

	input, err := mintFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	txn := CreateTransaction(types.ZeroAddress, &erc20TokenAddr, input, nil, rootchainTx)

	return txn, nil
}

// CreateApproveERC20Txn sends approve transaction
// to ERC20 token for spender so that it is able to spend given tokens
func CreateApproveERC20Txn(amount *big.Int,
	spender, erc20TokenAddr types.Address, rootchainTx bool) (*types.Transaction, error) {
	approveFnParams := &contractsapi.ApproveRootERC20Fn{
		Spender: spender,
		Amount:  amount,
	}

	input, err := approveFnParams.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode parameters for RootERC20.approve. error: %w", err)
	}

	return CreateTransaction(types.ZeroAddress, &erc20TokenAddr, input, nil, rootchainTx), nil
}

// SendTransaction sends provided transaction
func SendTransaction(txRelayer txrelayer.TxRelayer, addr types.Address, input []byte, contractName string,
	deployerKey crypto.Key) (*ethgo.Receipt, error) {
	txn := CreateTransaction(types.ZeroAddress, &addr, input, nil, true)

	receipt, err := txRelayer.SendTransaction(txn, deployerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction to %s contract (%s). error: %w",
			contractName, txn.To(), err)
	}

	if receipt == nil || receipt.Status != uint64(types.ReceiptSuccess) {
		return nil, fmt.Errorf("transaction execution failed on %s contract", contractName)
	}

	return receipt, nil
}

// CreateTransaction is a helper function that creates either dynamic fee or legacy transaction based on provided flag
func CreateTransaction(sender types.Address, receiver *types.Address,
	input []byte, value *big.Int, isDynamicFeeTx bool) *types.Transaction {
	var txData types.TxData
	if isDynamicFeeTx {
		txData = types.NewDynamicFeeTx(types.WithFrom(sender),
			types.WithTo(receiver), types.WithValue(value), types.WithInput(input))
	} else {
		txData = types.NewLegacyTx(types.WithFrom(sender),
			types.WithTo(receiver), types.WithValue(value), types.WithInput(input))
	}

	return types.NewTx(txData)
}

func DeployProxyContract(txRelayer txrelayer.TxRelayer, deployerKey crypto.Key, proxyContractName string,
	proxyAdmin, logicAddress types.Address) (*ethgo.Receipt, error) {
	proxyConstructorFn := contractsapi.TransparentUpgradeableProxyConstructorFn{
		Logic:  logicAddress,
		Admin_: proxyAdmin,
		Data:   []byte{},
	}

	constructorInput, err := proxyConstructorFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode proxy constructor function for %s contract. error: %w",
			proxyContractName, err)
	}

	var proxyDeployInput []byte

	proxyDeployInput = append(proxyDeployInput, contractsapi.TransparentUpgradeableProxy.Bytecode...)
	proxyDeployInput = append(proxyDeployInput, constructorInput...)

	txn := CreateTransaction(types.ZeroAddress, nil, proxyDeployInput, nil, true)

	receipt, err := txRelayer.SendTransaction(txn, deployerKey)
	if err != nil {
		return nil, fmt.Errorf("failed sending %s contract deploy transaction: %w", proxyContractName, err)
	}

	return receipt, nil
}
