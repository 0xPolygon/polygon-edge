package framework

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/command"
	bridgeCommon "github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/command/rootchain/server"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
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

// Deposit function invokes bridge deposit of ERC tokens (from the root to the child chain)
// with given receivers, amounts and/or token ids
func (t *TestBridge) Deposit(token bridgeCommon.TokenType, rootTokenAddr, rootPredicateAddr types.Address,
	senderKey, receivers, amounts, tokenIDs, jsonRPCAddr, minterKey string, childChainMintable bool) error {
	args := []string{}

	if receivers == "" {
		return errors.New("provide at least one receiver address value")
	}

	if jsonRPCAddr == "" {
		return errors.New("provide a JSON RPC endpoint URL")
	}

	switch token {
	case bridgeCommon.ERC20:
		if amounts == "" {
			return errors.New("provide at least one amount value")
		}

		if tokenIDs != "" {
			return errors.New("not expected to provide token ids for ERC 20 deposits")
		}

		args = append(args,
			"bridge",
			"deposit-erc20",
			"--root-token", rootTokenAddr.String(),
			"--root-predicate", rootPredicateAddr.String(),
			"--receivers", receivers,
			"--amounts", amounts,
			"--sender-key", senderKey,
			"--minter-key", minterKey,
			"--json-rpc", jsonRPCAddr)

		if childChainMintable {
			args = append(args, "--child-chain-mintable")
		}

	case bridgeCommon.ERC721:
		if tokenIDs == "" {
			return errors.New("provide at least one token id value")
		}

		args = append(args,
			"bridge",
			"deposit-erc721",
			"--root-token", rootTokenAddr.String(),
			"--root-predicate", rootPredicateAddr.String(),
			"--receivers", receivers,
			"--token-ids", tokenIDs,
			"--sender-key", senderKey,
			"--minter-key", minterKey,
			"--json-rpc", jsonRPCAddr)

		if childChainMintable {
			args = append(args, "--child-chain-mintable")
		}

	case bridgeCommon.ERC1155:
		if amounts == "" {
			return errors.New("provide at least one amount value")
		}

		if tokenIDs == "" {
			return errors.New("provide at least one token id value")
		}

		args = append(args,
			"bridge",
			"deposit-erc1155",
			"--root-token", rootTokenAddr.String(),
			"--root-predicate", rootPredicateAddr.String(),
			"--receivers", receivers,
			"--amounts", amounts,
			"--token-ids", tokenIDs,
			"--sender-key", senderKey,
			"--minter-key", minterKey,
			"--json-rpc", jsonRPCAddr)

		if childChainMintable {
			args = append(args, "--child-chain-mintable")
		}
	}

	return t.cmdRun(args...)
}

// Withdraw function is used to invoke bridge withdrawals for any kind of ERC tokens (from the child to the root chain)
// with given receivers, amounts and/or token ids
func (t *TestBridge) Withdraw(token bridgeCommon.TokenType,
	senderKey, receivers, amounts, tokenIDs, jsonRPCAddr string,
	childPredicate, childToken types.Address, childChainMintable bool) error {
	if senderKey == "" {
		return errors.New("provide hex-encoded sender private key")
	}

	if receivers == "" {
		return errors.New("provide at least one receiver address value")
	}

	if jsonRPCAddr == "" {
		return errors.New("provide a JSON RPC endpoint URL")
	}

	args := []string{}

	switch token {
	case bridgeCommon.ERC20:
		if amounts == "" {
			return errors.New("provide at least one amount value")
		}

		if tokenIDs != "" {
			return errors.New("not expected to provide token ids for ERC 20 withdrawals")
		}

		args = append(args,
			"bridge",
			"withdraw-erc20",
			"--child-predicate", childPredicate.String(),
			"--child-token", childToken.String(),
			"--sender-key", senderKey,
			"--receivers", receivers,
			"--amounts", amounts,
			"--json-rpc", jsonRPCAddr)

		if childChainMintable {
			args = append(args, "--child-chain-mintable")
		}

	case bridgeCommon.ERC721:
		if tokenIDs == "" {
			return errors.New("provide at least one token id value")
		}

		args = append(args,
			"bridge",
			"withdraw-erc721",
			"--child-predicate", childPredicate.String(),
			"--child-token", childToken.String(),
			"--sender-key", senderKey,
			"--receivers", receivers,
			"--token-ids", tokenIDs,
			"--json-rpc", jsonRPCAddr)

		if childChainMintable {
			args = append(args, "--child-chain-mintable")
		}

	case bridgeCommon.ERC1155:
		if amounts == "" {
			return errors.New("provide at least one amount value")
		}

		if tokenIDs == "" {
			return errors.New("provide at least one token id value")
		}

		args = append(args,
			"bridge",
			"withdraw-erc1155",
			"--child-predicate", childPredicate.String(),
			"--child-token", childToken.String(),
			"--sender-key", senderKey,
			"--receivers", receivers,
			"--amounts", amounts,
			"--token-ids", tokenIDs,
			"--json-rpc", jsonRPCAddr)

		if childChainMintable {
			args = append(args, "--child-chain-mintable")
		}
	}

	return t.cmdRun(args...)
}

// SendExitTransaction sends exit transaction to the root chain
func (t *TestBridge) SendExitTransaction(exitHelper types.Address, exitID uint64, childJSONRPCAddr string) error {
	if childJSONRPCAddr == "" {
		return errors.New("provide a child chain JSON RPC endpoint URL")
	}

	return t.cmdRun(
		"bridge",
		"exit",
		"--exit-helper", exitHelper.String(),
		"--exit-id", strconv.FormatUint(exitID, 10),
		"--root-json-rpc", t.JSONRPCAddr(),
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
	polybftConfig, err := polybft.LoadPolyBFTConfig(genesisPath)
	if err != nil {
		return err
	}

	args := []string{
		"rootchain",
		"deploy",
		"--stake-manager", polybftConfig.Bridge.StakeManagerAddr.String(),
		"--stake-token", polybftConfig.Bridge.StakeTokenAddr.String(),
		"--genesis", genesisPath,
		"--test",
	}

	if err := t.cmdRun(args...); err != nil {
		return fmt.Errorf("failed to deploy rootchain contracts: %w", err)
	}

	return nil
}

// fundRootchainValidators sends predefined amount of tokens to rootchain validators
func (t *TestBridge) fundRootchainValidators(polybftConfig polybft.PolyBFTConfig) error {
	validatorSecrets, err := genesis.GetValidatorKeyFiles(t.clusterConfig.TmpDir, t.clusterConfig.ValidatorPrefix)
	if err != nil {
		return fmt.Errorf("could not get validator secrets on initial rootchain funding of genesis validators: %w", err)
	}

	balances := make([]*big.Int, len(polybftConfig.InitialValidatorSet))
	secrets := make([]string, len(validatorSecrets))

	for i, secret := range validatorSecrets {
		secrets[i] = path.Join(t.clusterConfig.TmpDir, secret)
		balances[i] = command.DefaultPremineBalance
	}

	if err := t.FundValidators(polybftConfig.Bridge.StakeTokenAddr,
		secrets, balances); err != nil {
		return fmt.Errorf("failed to fund validators on the rootchain: %w", err)
	}

	return nil
}

func (t *TestBridge) whitelistValidators(validatorAddresses []types.Address,
	polybftConfig polybft.PolyBFTConfig) error {
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

func (t *TestBridge) registerGenesisValidators(polybftConfig polybft.PolyBFTConfig) error {
	validatorSecrets, err := genesis.GetValidatorKeyFiles(t.clusterConfig.TmpDir, t.clusterConfig.ValidatorPrefix)
	if err != nil {
		return fmt.Errorf("could not get validator secrets on whitelist of genesis validators: %w", err)
	}

	g, ctx := errgroup.WithContext(context.Background())

	for _, secret := range validatorSecrets {
		secret := secret

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				args := []string{
					"polybft",
					"register-validator",
					"--jsonrpc", t.JSONRPCAddr(),
					"--supernet-manager", polybftConfig.Bridge.CustomSupernetManagerAddr.String(),
					"--" + polybftsecrets.AccountDirFlag, path.Join(t.clusterConfig.TmpDir, secret),
				}

				if err := t.cmdRun(args...); err != nil {
					return fmt.Errorf("failed to register genesis validator on supernet manager: %w", err)
				}

				return nil
			}
		})
	}

	return g.Wait()
}

func (t *TestBridge) initialStakingOfGenesisValidators(polybftConfig polybft.PolyBFTConfig) error {
	validatorSecrets, err := genesis.GetValidatorKeyFiles(t.clusterConfig.TmpDir, t.clusterConfig.ValidatorPrefix)
	if err != nil {
		return fmt.Errorf("could not get validator secrets on initial staking of genesis validators: %w", err)
	}

	g, ctx := errgroup.WithContext(context.Background())

	for i, secret := range validatorSecrets {
		secret := secret
		i := i

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				args := []string{
					"polybft",
					"stake",
					"--jsonrpc", t.JSONRPCAddr(),
					"--stake-manager", polybftConfig.Bridge.StakeManagerAddr.String(),
					"--" + polybftsecrets.AccountDirFlag, path.Join(t.clusterConfig.TmpDir, secret),
					"--amount", t.getStakeAmount(i).String(),
					"--supernet-id", strconv.FormatInt(polybftConfig.SupernetID, 10),
					"--stake-token", polybftConfig.Bridge.StakeTokenAddr.String(),
				}

				if err := t.cmdRun(args...); err != nil {
					return fmt.Errorf("failed to do initial staking for genesis validator on stake manager: %w", err)
				}

				return nil
			}
		})
	}

	return g.Wait()
}

func (t *TestBridge) getStakeAmount(validatorIndex int) *big.Int {
	l := len(t.clusterConfig.StakeAmounts)
	if l == 0 || l <= validatorIndex {
		return command.DefaultStake
	}

	return t.clusterConfig.StakeAmounts[validatorIndex]
}

func (t *TestBridge) finalizeGenesis(genesisPath string, polybftConfig polybft.PolyBFTConfig) error {
	args := []string{
		"polybft",
		"supernet",
		"--jsonrpc", t.JSONRPCAddr(),
		"--private-key", rootHelper.TestAccountPrivKey,
		"--genesis", genesisPath,
		"--supernet-manager", polybftConfig.Bridge.CustomSupernetManagerAddr.String(),
		"--stake-manager", polybftConfig.Bridge.StakeManagerAddr.String(),
		"--finalize-genesis-set",
		"--enable-staking",
	}

	if err := t.cmdRun(args...); err != nil {
		return fmt.Errorf("failed to finalize genesis validators on supernet manager: %w", err)
	}

	return nil
}

// FundValidators sends tokens to a rootchain validators
func (t *TestBridge) FundValidators(tokenAddress types.Address, secretsPaths []string, amounts []*big.Int) error {
	if len(secretsPaths) != len(amounts) {
		return errors.New("expected the same length of secrets paths and amounts")
	}

	args := []string{
		"rootchain",
		"fund",
		"--stake-token", tokenAddress.String(),
		"--mint",
	}

	for i := 0; i < len(secretsPaths); i++ {
		secretsManager, err := polybftsecrets.GetSecretsManager(secretsPaths[i], "", true)
		if err != nil {
			return err
		}

		key, err := wallet.GetEcdsaFromSecret(secretsManager)
		if err != nil {
			return err
		}

		args = append(args, "--addresses", key.Address().String())
		args = append(args, "--amounts", amounts[i].String())
	}

	if err := t.cmdRun(args...); err != nil {
		return err
	}

	return nil
}

func (t *TestBridge) deployStakeManager(genesisPath string) error {
	args := []string{
		"polybft",
		"stake-manager-deploy",
		"--jsonrpc", t.JSONRPCAddr(),
		"--genesis", genesisPath,
		"--test",
	}

	if err := t.cmdRun(args...); err != nil {
		return fmt.Errorf("failed to deploy stake manager contract: %w", err)
	}

	return nil
}
