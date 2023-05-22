package framework

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"

	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/types"
)

func deposit(token common.TokenType, rootTokenAddr, rootPredicateAddr types.Address,
	receivers, amounts, tokenIDs string,
	binary string, stdout io.Writer) error {
	args := []string{}

	if receivers == "" {
		return errors.New("provide at least one receiver address value")
	}

	switch token {
	case common.ERC20:
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

	case common.ERC721:
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

	case common.ERC1155:
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

	return runCommand(binary, args, stdout)
}

func withdraw(token common.TokenType,
	senderKey, receivers,
	amounts, tokenIDs, jsonRPCEndpoint string, childToken types.Address,
	binary string, stdout io.Writer) error {
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
	case common.ERC20:
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

	case common.ERC721:
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

	case common.ERC1155:
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

	return runCommand(binary, args, stdout)
}

func sendExitTransaction(exitHelper types.Address, exitID uint64,
	rootJSONRPCAddr, childJSONRPCAddr string,
	binary string, stdout io.Writer) error {
	if rootJSONRPCAddr == "" {
		return errors.New("provide a root JSON RPC endpoint URL")
	}

	if childJSONRPCAddr == "" {
		return errors.New("provide a child JSON RPC endpoint URL")
	}

	args := []string{
		"bridge",
		"exit",
		"--exit-helper", exitHelper.String(),
		"--exit-id", strconv.FormatUint(exitID, 10),
		"--root-json-rpc", rootJSONRPCAddr,
		"--child-json-rpc", childJSONRPCAddr,
		"--test",
	}

	return runCommand(binary, args, stdout)
}

// runCommand executes command with given arguments
func runCommand(binary string, args []string, stdout io.Writer) error {
	var stdErr bytes.Buffer

	cmd := exec.Command(binary, args...)
	cmd.Stderr = &stdErr
	cmd.Stdout = stdout

	if err := cmd.Run(); err != nil {
		if stdErr.Len() > 0 {
			return fmt.Errorf("failed to execute command: %s", stdErr.String())
		}

		return fmt.Errorf("failed to execute command: %w", err)
	}

	if stdErr.Len() > 0 {
		return fmt.Errorf("error during command execution: %s", stdErr.String())
	}

	return nil
}
