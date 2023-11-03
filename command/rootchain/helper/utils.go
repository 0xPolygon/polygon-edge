package helper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	polybftWallet "github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
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
	GenesisPathFlag         = "genesis"
	GenesisPathFlagDesc     = "genesis file path, which contains chain configuration"
	DefaultGenesisPath      = "./genesis.json"
	StakeTokenFlag          = "stake-token"
	StakeTokenFlagDesc      = "address of ERC20 token used for staking on rootchain"
	ProxyContractsAdminFlag = "proxy-contracts-admin"
	ProxyContractsAdminDesc = "admin for proxy contracts"
	AddressesFlag           = "addresses"
	AmountsFlag             = "amounts"
	Erc20TokenFlag          = "erc20-token" //nolint:gosec
)

var (
	ErrRootchainNotFound   = errors.New("rootchain not found")
	ErrRootchainPortBind   = errors.New("port 8545 is not bind with localhost")
	ErrMandatoryStakeToken = errors.New("stake token address is mandatory")
	errTestModeSecrets     = errors.New("rootchain test mode does not imply specifying secrets parameters")

	ErrNoAddressesProvided = errors.New("no addresses provided")
	ErrInconsistentLength  = errors.New("addresses and amounts must be equal length")

	ErrMandatoryERC20Token = errors.New("erc20 token address is mandatory")

	rootchainAccountKey *wallet.Key
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
func DecodePrivateKey(rawKey string) (ethgo.Key, error) {
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
func GetECDSAKey(privateKey, accountDir, accountConfig string) (ethgo.Key, error) {
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
func GetValidatorInfo(validatorAddr ethgo.Address, supernetManagerAddr, stakeManagerAddr types.Address,
	chainID int64, txRelayer txrelayer.TxRelayer) (*polybft.ValidatorInfo, error) {
	caller := ethgo.Address(contracts.SystemCaller)
	getValidatorMethod := contractsapi.CustomSupernetManager.Abi.GetMethod("getValidator")

	encode, err := getValidatorMethod.Encode([]interface{}{validatorAddr})
	if err != nil {
		return nil, err
	}

	response, err := txRelayer.Call(caller, ethgo.Address(supernetManagerAddr), encode)
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
		ID:        new(big.Int).SetInt64(chainID),
		Validator: types.Address(validatorAddr),
	}

	encode, err = stakeOfFn.EncodeAbi()
	if err != nil {
		return nil, err
	}

	response, err = txRelayer.Call(caller, ethgo.Address(stakeManagerAddr), encode)
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
	amount *big.Int, rootchainTx bool) (*ethgo.Transaction, error) {
	mintFn := &contractsapi.MintRootERC20Fn{
		To:     receiver,
		Amount: amount,
	}

	input, err := mintFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(erc20TokenAddr)
	txn := CreateTransaction(ethgo.ZeroAddress, &addr, input, nil, rootchainTx)

	return txn, nil
}

// CreateApproveERC20Txn sends approve transaction
// to ERC20 token for spender so that it is able to spend given tokens
func CreateApproveERC20Txn(amount *big.Int,
	spender, erc20TokenAddr types.Address, rootchainTx bool) (*ethgo.Transaction, error) {
	approveFnParams := &contractsapi.ApproveRootERC20Fn{
		Spender: spender,
		Amount:  amount,
	}

	input, err := approveFnParams.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode parameters for RootERC20.approve. error: %w", err)
	}

	addr := ethgo.Address(erc20TokenAddr)

	return CreateTransaction(ethgo.ZeroAddress, &addr, input, nil, rootchainTx), nil
}

// SendTransaction sends provided transaction
func SendTransaction(txRelayer txrelayer.TxRelayer, addr ethgo.Address, input []byte, contractName string,
	deployerKey ethgo.Key) (*ethgo.Receipt, error) {
	txn := CreateTransaction(ethgo.ZeroAddress, &addr, input, nil, true)

	receipt, err := txRelayer.SendTransaction(txn, deployerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction to %s contract (%s). error: %w",
			contractName, txn.To.Address(), err)
	}

	if receipt == nil || receipt.Status != uint64(types.ReceiptSuccess) {
		return nil, fmt.Errorf("transaction execution failed on %s contract", contractName)
	}

	return receipt, nil
}

// CreateTransaction is a helper function that creates either dynamic fee or legacy transaction based on provided flag
func CreateTransaction(sender ethgo.Address, receiver *ethgo.Address,
	input []byte, value *big.Int, isDynamicFee bool) *ethgo.Transaction {
	txn := &ethgo.Transaction{
		From:  sender,
		To:    receiver,
		Input: input,
		Value: value,
	}

	if isDynamicFee {
		txn.Type = ethgo.TransactionDynamicFee
	} else {
		txn.Type = ethgo.TransactionLegacy
	}

	return txn
}

func DeployProxyContract(txRelayer txrelayer.TxRelayer, deployerKey ethgo.Key, proxyContractName string,
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

	txn := CreateTransaction(ethgo.ZeroAddress, nil, proxyDeployInput, nil, true)

	receipt, err := txRelayer.SendTransaction(txn, deployerKey)
	if err != nil {
		return nil, fmt.Errorf("failed sending %s contract deploy transaction: %w", proxyContractName, err)
	}

	return receipt, nil
}
