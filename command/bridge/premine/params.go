package premine

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/helper"
	validatorHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	premineAmountFlag      = "premine-amount"
	stakedAmountFlag       = "stake-amount"
	rootERC20PredicateFlag = "root-erc20-predicate"
)

type premineParams struct {
	accountDir         string
	accountConfig      string
	privateKey         string
	bladeManager       string
	rootERC20Predicate string
	nativeTokenRoot    string
	jsonRPC            string
	stakedAmount       string
	premineAmount      string

	nonStakedValue *big.Int
	stakedValue    *big.Int
}

func (p *premineParams) validateFlags() (err error) {
	if err := types.IsValidAddress(p.nativeTokenRoot); err != nil {
		return fmt.Errorf("invalid erc20 token address is provided: %w", err)
	}

	if types.StringToAddress(p.nativeTokenRoot) == types.ZeroAddress {
		return errors.New("native erc20 token address must be non-zero")
	}

	if err := types.IsValidAddress(p.rootERC20Predicate); err != nil {
		return fmt.Errorf("invalid root erc20 predicate address is provided: %w", err)
	}

	if types.StringToAddress(p.rootERC20Predicate) == types.ZeroAddress {
		return errors.New("root erc20 predicate address must be non-zero")
	}

	if err := types.IsValidAddress(p.bladeManager); err != nil {
		return fmt.Errorf("invalid blade manager address is provided: %w", err)
	}

	if types.StringToAddress(p.bladeManager) == types.ZeroAddress {
		return errors.New("blade manager address must be non-zero")
	}

	if p.nonStakedValue, err = common.ParseUint256orHex(&p.premineAmount); err != nil {
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
