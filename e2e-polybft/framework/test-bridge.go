package framework

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/command/rootchain/server"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/types"
)

type TestBridge struct {
	t             *testing.T
	clusterConfig *TestClusterConfig
	node          *node
}

func NewTestBridge(t *testing.T, clusterConfig *TestClusterConfig) (*TestBridge, error) {
	t.Helper()

	bridge := &TestBridge{
		t:             t,
		clusterConfig: clusterConfig,
	}

	err := bridge.Start()
	if err != nil {
		return nil, err
	}

	return bridge, nil
}

func (t *TestBridge) Start() error {
	// Build arguments
	args := []string{
		"rootchain",
		"server",
		"--data-dir", t.clusterConfig.Dir("test-rootchain"),
	}

	stdout := t.clusterConfig.GetStdout("bridge")

	bridgeNode, err := newNode(t.clusterConfig.Binary, args, stdout)
	if err != nil {
		return err
	}

	t.node = bridgeNode

	if err = server.PingServer(nil); err != nil {
		return err
	}

	return nil
}

func (t *TestBridge) Stop() {
	if err := t.node.Stop(); err != nil {
		t.t.Error(err)
	}

	t.node = nil
}

func (t *TestBridge) JSONRPCAddr() string {
	return fmt.Sprintf("http://%s:%d", hostIP, 8545)
}

func (t *TestBridge) WaitUntil(pollFrequency, timeout time.Duration, handler func() (bool, error)) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout")
		case <-time.After(pollFrequency):
		}

		isConditionMet, err := handler()
		if err != nil {
			return err
		}

		if isConditionMet {
			return nil
		}
	}
}

// DepositERC20 function invokes bridge deposit ERC20 tokens (from the root to the child chain)
// with given receivers and amounts
func (t *TestBridge) DepositERC20(rootTokenAddr, rootPredicateAddr types.Address, receivers, amounts string) error {
	if receivers == "" {
		return errors.New("provide at least one receiver address value")
	}

	if amounts == "" {
		return errors.New("provide at least one amount value")
	}

	return t.cmdRun(
		"bridge",
		"deposit-erc20",
		"--test",
		"--root-token", rootTokenAddr.String(),
		"--root-predicate", rootPredicateAddr.String(),
		"--receivers", receivers,
		"--amounts", amounts)
}

// WithdrawERC20 function is used to invoke bridge withdraw ERC20 tokens (from the child to the root chain)
// with given receivers and amounts
func (t *TestBridge) WithdrawERC20(senderKey, receivers, amounts, jsonRPCEndpoint string) error {
	if senderKey == "" {
		return errors.New("provide hex encoded sender private key")
	}

	if receivers == "" {
		return errors.New("provide at least one receiver address value")
	}

	if amounts == "" {
		return errors.New("provide at least one amount value")
	}

	if jsonRPCEndpoint == "" {
		return errors.New("provide a JSON RPC endpoint URL")
	}

	return t.cmdRun(
		"bridge",
		"withdraw-erc20",
		"--sender-key", senderKey,
		"--receivers", receivers,
		"--amounts", amounts,
		"--json-rpc", jsonRPCEndpoint,
	)
}

// SendExitTransaction sends exit transaction to the root chain
func (t *TestBridge) SendExitTransaction(exitHelper types.Address, exitID uint64,
	rootJSONRPCAddr, childJSONRPCAddr string) error {
	if rootJSONRPCAddr == "" {
		return errors.New("provide a root JSON RPC endpoint URL")
	}

	if childJSONRPCAddr == "" {
		return errors.New("provide a child JSON RPC endpoint URL")
	}

	return t.cmdRun(
		"bridge",
		"exit",
		"--exit-helper", exitHelper.String(),
		"--exit-id", strconv.FormatUint(exitID, 10),
		"--root-json-rpc", rootJSONRPCAddr,
		"--child-json-rpc", childJSONRPCAddr,
		"--test",
	)
}

// cmdRun executes arbitrary command from the given binary
func (t *TestBridge) cmdRun(args ...string) error {
	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("bridge"))
}

// deployRootchainContracts deploys and initializes rootchain contracts
func (t *TestBridge) deployRootchainContracts(genesisPath string) error {
	args := []string{
		"rootchain",
		"deploy",
		"--genesis", genesisPath,
		"--test",
	}

	if err := t.cmdRun(args...); err != nil {
		return fmt.Errorf("failed to deploy rootchain contracts: %w", err)
	}

	return nil
}

// fundRootchainValidators sends predefined amount of tokens to rootchain validators
func (t *TestBridge) fundRootchainValidators(genesisPath string) error {
	polybftConfig, _, err := readPolybftConfig(genesisPath)
	if err != nil {
		return fmt.Errorf("could not fund validators on rootchain: %w", err)
	}

	args := []string{
		"rootchain",
		"fund",
		"--" + polybftsecrets.AccountDirFlag, path.Join(t.clusterConfig.TmpDir, t.clusterConfig.ValidatorPrefix),
		"--num", strconv.Itoa(int(t.clusterConfig.ValidatorSetSize) + t.clusterConfig.NonValidatorCount),
		"--native-root-token", polybftConfig.Bridge.RootNativeERC20Addr.String(),
		"--mint",
	}

	if err := t.cmdRun(args...); err != nil {
		return fmt.Errorf("failed to fund validators on the rootchain: %w", err)
	}

	return nil
}

func (t *TestBridge) whitelistValidators(validatorAddresses []types.Address, genesisPath string) error {
	polybftConfig, _, err := readPolybftConfig(genesisPath)
	if err != nil {
		return fmt.Errorf("could not whitelist genesis validators on supernet manager: %w", err)
	}

	addressesAsString := make([]string, len(validatorAddresses))
	for i := 0; i < len(validatorAddresses); i++ {
		addressesAsString[i] = validatorAddresses[i].String()
	}

	args := []string{
		"polybft",
		"whitelist-validators",
		"--addresses", strings.Join(addressesAsString, ","),
		"--jsonrpc", t.JSONRPCAddr(),
		"--supernet-manager", polybftConfig.Bridge.CustomSupernetManagerAddr.String(),
		"--private-key", rootHelper.TestAccountPrivKey,
	}

	if err := t.cmdRun(args...); err != nil {
		return fmt.Errorf("failed to whitelist genesis validators on supernet manager: %w", err)
	}

	return nil
}

func (t *TestBridge) registerGenesisValidators(genesisPath string) error {
	validatorSecrets, err := genesis.GetValidatorKeyFiles(t.clusterConfig.TmpDir, t.clusterConfig.ValidatorPrefix)
	if err != nil {
		return fmt.Errorf("could not get validator secrets on whitelist of genesis validators: %w", err)
	}

	polybftConfig, _, err := readPolybftConfig(genesisPath)
	if err != nil {
		return fmt.Errorf("could not whitelist genesis validators on supernet manager: %w", err)
	}

	for _, secret := range validatorSecrets {
		args := []string{
			"polybft",
			"register-validator",
			"--jsonrpc", t.JSONRPCAddr(),
			"--supernet-manager", polybftConfig.Bridge.CustomSupernetManagerAddr.String(),
			"--" + polybftsecrets.AccountDirFlag, path.Join(t.clusterConfig.TmpDir, secret),
		}

		if err := t.cmdRun(args...); err != nil {
			return fmt.Errorf("failed to whitelist genesis validators on supernet manager: %w", err)
		}
	}

	return nil
}

func (t *TestBridge) initialStakingOfGenesisValidators(genesisPath string) error {
	validatorSecrets, err := genesis.GetValidatorKeyFiles(t.clusterConfig.TmpDir, t.clusterConfig.ValidatorPrefix)
	if err != nil {
		return fmt.Errorf("could not get validator secrets on initial staking of genesis validators: %w", err)
	}

	polybftConfig, chainID, err := readPolybftConfig(genesisPath)
	if err != nil {
		return fmt.Errorf("could not do initial staking of genesis validators on stake manager: %w", err)
	}

	for i, secret := range validatorSecrets {
		args := []string{
			"polybft",
			"stake",
			"--jsonrpc", t.JSONRPCAddr(),
			"--stake-manager", polybftConfig.Bridge.StakeManagerAddr.String(),
			"--" + polybftsecrets.AccountDirFlag, path.Join(t.clusterConfig.TmpDir, secret),
			"--amount", strconv.FormatUint(polybftConfig.InitialValidatorSet[i].Stake.Uint64(), 10),
			"--chain-id", strconv.FormatUint(chainID, 10),
			"--native-root-token", polybftConfig.Bridge.RootNativeERC20Addr.String(),
		}

		if err := t.cmdRun(args...); err != nil {
			return fmt.Errorf("failed to do initial staking for genesis validator on stake manager: %w", err)
		}
	}

	return nil
}

func (t *TestBridge) finalizeGenesis(genesisPath string) error {
	polybftConfig, _, err := readPolybftConfig(genesisPath)
	if err != nil {
		return fmt.Errorf("could not finalize genesis validators on supernet manager: %w", err)
	}

	args := []string{
		"polybft",
		"supernet",
		"--jsonrpc", t.JSONRPCAddr(),
		"--private-key", rootHelper.TestAccountPrivKey,
		"supernet-manager", polybftConfig.Bridge.CustomSupernetManagerAddr.String(),
		"--finalize-genesis-set",
		"--enable-staking",
	}

	if err := t.cmdRun(args...); err != nil {
		return fmt.Errorf("failed to finalize genesis validators on supernet manager: %w", err)
	}

	return nil
}

func readPolybftConfig(genesisPath string) (*polybft.PolyBFTConfig, uint64, error) {
	chainConfig, err := chain.ImportFromFile(genesisPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read chain configuration: %w", err)
	}

	consensusConfig, err := polybft.GetPolyBFTConfig(chainConfig)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to retrieve consensus configuration: %w", err)
	}

	return &consensusConfig, uint64(chainConfig.Params.ChainID), nil
}
