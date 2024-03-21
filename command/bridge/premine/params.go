package premine

import (
	"bytes"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/command/helper"
	validatorHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	premineAmountFlag = "premine-amount"
	stakedAmountFlag  = "stake-amount"
)

type premineParams struct {
	accountDir      string
	accountConfig   string
	privateKey      string
	bladeManager    string
	nativeTokenRoot string
	jsonRPC         string
	stakedAmount    string
	premineAmount   string
	txTimeout       time.Duration
	txPollFreq      time.Duration

	premineAmountValue  *big.Int
	stakedValue         *big.Int
	nativeTokenRootAddr types.Address
	bladeManagerAddr    types.Address
}

func (p *premineParams) validateFlags() (err error) {
	p.nativeTokenRootAddr, err = types.IsValidAddress(p.nativeTokenRoot, false)
	if err != nil {
		return fmt.Errorf("invalid erc20 token address is provided: %w", err)
	}

	p.bladeManagerAddr, err = types.IsValidAddress(p.bladeManager, false)
	if err != nil {
		return fmt.Errorf("invalid blade manager address is provided: %w", err)
	}

	if p.premineAmountValue, err = common.ParseUint256orHex(&p.premineAmount); err != nil {
		return err
	}

	if p.stakedValue, err = common.ParseUint256orHex(&p.stakedAmount); err != nil {
		return err
	}

	// validate jsonrpc address
	if _, err := helper.ParseJSONRPCAddress(p.jsonRPC); err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	if p.privateKey == "" {
		return validatorHelper.ValidateSecretFlags(p.accountDir, p.accountConfig)
	}

	return nil
}

type premineResult struct {
	Address         string   `json:"address"`
	NonStakedAmount *big.Int `json:"nonStakedAmount"`
	StakedAmount    *big.Int `json:"stakedAmount"`
}

func (p premineResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[NATIVE ROOT TOKEN PREMINE]\n")

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Address|%s", p.Address))
	vals = append(vals, fmt.Sprintf("Non-Staked Amount Premined|%d", p.NonStakedAmount))
	vals = append(vals, fmt.Sprintf("Staked Premined|%d", p.StakedAmount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
