package staking

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

type stakeParams struct {
	accountDir          string
	accountConfig       string
	stakeManagerAddr    string
	nativeRootTokenAddr string
	jsonRPC             string
	chainID             uint64
	amount              string

	amountValue *big.Int
}

func (sp *stakeParams) validateFlags() (err error) {
	if sp.amountValue, err = helper.ParseAmount(sp.amount); err != nil {
		return err
	}

	// validate jsonrpc address
	if _, err := helper.ParseJSONRPCAddress(sp.jsonRPC); err != nil {
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
