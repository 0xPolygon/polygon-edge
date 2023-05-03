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

	bridgeCommon "github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/command/rootchain/server"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/types"
	"golang.org/x/sync/errgroup"
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
	receivers, amounts, tokenIDs string) error {
	args := []string{}

	if receivers == "" {
		return errors.New("provide at least one receiver address value")
	}

	switch token {
	case bridgeCommon.ERC20:
		if amounts == "" {
			return errors.New("provide at least one amount value")
		}

		if tokenIDs != "" {
			return errors.New("not expected to provide token ids for ERC-20 deposits")
		}

		args = append(args,
			"bridge",
			"deposit-erc20",
			"--test",
			"--root-token", rootTokenAddr.String(),
			"--root-predicate", rootPredicateAddr.String(),
			"--receivers", receivers,
			"--amounts", amounts)

	case bridgeCommon.ERC721:
		if tokenIDs == "" {
			return errors.New("provide at least one token id value")
		}

		args = append(args,
			"bridge",
			"deposit-erc721",
			"--test",
			"--root-token", rootTokenAddr.String(),
			"--root-predicate", rootPredicateAddr.String(),
			"--receivers", receivers,
			"--token-ids", tokenIDs)

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
			"--test",
			"--root-token", rootTokenAddr.String(),
			"--root-predicate", rootPredicateAddr.String(),
			"--receivers", receivers,
			"--amounts", amounts,
			"--token-ids", tokenIDs)
	}

	return t.cmdRun(args...)
}

// Withdraw function is used to invoke bridge withdrawals for any kind of ERC tokens (from the child to the root chain)
// with given receivers, amounts and/or token ids
func (t *TestBridge) Withdraw(token bridgeCommon.TokenType,
	senderKey, receivers,
	amounts, tokenIDs, jsonRPCEndpoint string, childToken types.Address) error {
	if senderKey == "" {
		return errors.New("provide hex-encoded sender private key")
	}

	if receivers == "" {
		return errors.New("provide at least one receiver address value")
	}

	if jsonRPCEndpoint == "" {
		return errors.New("provide a JSON RPC endpoint URL")
	}

	args := []string{}

	switch token {
	case bridgeCommon.ERC20:
		if amounts == "" {
			return errors.New("provide at least one amount value")
		}

		if tokenIDs != "" {
			return errors.New("not expected to provide token ids for ERC-20 withdrawals")
		}

		args = append(args,
			"bridge",
			"withdraw-erc20",
			"--sender-key", senderKey,
			"--receivers", receivers,
			"--amounts", amounts,
			"--json-rpc", jsonRPCEndpoint)

	case bridgeCommon.ERC721:
		if tokenIDs == "" {
			return errors.New("provide at least one token id value")
		}

		args = append(args,
			"bridge",
			"withdraw-erc721",
			"--sender-key", senderKey,
			"--receivers", receivers,
			"--token-ids", tokenIDs,
			"--json-rpc", jsonRPCEndpoint,
			"--child-token", childToken.String())

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
			"--sender-key", senderKey,
			"--receivers", receivers,
			"--amounts", amounts,
			"--token-ids", tokenIDs,
			"--json-rpc", jsonRPCEndpoint,
			"--child-token", childToken.String())
	}

	return t.cmdRun(args...)
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
func (t *TestBridge) fundRootchainValidators(polybftConfig polybft.PolyBFTConfig) error {
	validatorSecrets, err := genesis.GetValidatorKeyFiles(t.clusterConfig.TmpDir, t.clusterConfig.ValidatorPrefix)
	if err != nil {
		return fmt.Errorf("could not get validator secrets on initial rootchain funding of genesis validators: %w", err)
	}

	for i, secret := range validatorSecrets {
		err := t.FundSingleValidator(
			polybftConfig.Bridge.RootNativeERC20Addr,
			path.Join(t.clusterConfig.TmpDir, secret),
			polybftConfig.InitialValidatorSet[i].Balance)
		if err != nil {
			return fmt.Errorf("failed to fund validators on the rootchain: %w", err)
		}
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

func (t *TestBridge) initialStakingOfGenesisValidators(
	polybftConfig polybft.PolyBFTConfig, chainID int64) error {
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
					"--amount", polybftConfig.InitialValidatorSet[i].Stake.String(),
					"--chain-id", strconv.FormatInt(chainID, 10),
					"--native-root-token", polybftConfig.Bridge.RootNativeERC20Addr.String(),
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

// FundSingleValidator sends tokens to a rootchain validators
func (t *TestBridge) FundSingleValidator(tookenAddress types.Address, secretsPath string, amount *big.Int) error {
	args := []string{
		"rootchain",
		"fund",
		"--" + polybftsecrets.AccountDirFlag, secretsPath,
		"--amount", amount.String(),
		"--native-root-token", tookenAddress.String(),
		"--mint",
	}

	if err := t.cmdRun(args...); err != nil {
		return fmt.Errorf("failed to fund a validator on the rootchain: %w", err)
	}

	return nil
}
