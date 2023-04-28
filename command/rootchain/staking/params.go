package staking

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

type stakeParams struct {
	accountDir          string
	accountConfig       string
	stakeManagerAddr    string
	nativeRootTokenAddr string
	jsonRPC             string
	amount              uint64
	chainID             uint64
}

func (sp *stakeParams) validateFlags() error {
	// validate jsonrpc address
	_, err := helper.ParseJSONRPCAddress(sp.jsonRPC)
	if err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	return sidechainHelper.ValidateSecretFlags(sp.accountDir, sp.accountConfig)
}

type stakeResult struct {
	validatorAddress string
	amount           uint64
}

func (sr stakeResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[VALIDATOR STAKE]\n")

	vals := make([]string, 0, 2)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", sr.validatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Staked|%v", sr.amount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
