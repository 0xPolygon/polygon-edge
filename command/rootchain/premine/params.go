package premine

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/helper"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	errMandatorySupernetManagerAddr = errors.New("custom supernet manager address not defined")
	errMandatoryRootPredicateAddr   = errors.New("root erc20 predicate address not defined")
)

const rootERC20PredicateFlag = "root-erc20-predicate"

type premineParams struct {
	accountDir            string
	accountConfig         string
	privateKey            string
	customSupernetManager string
	rootERC20Predicate    string
	nativeTokenRoot       string
	jsonRPC               string
	amount                string

	amountValue *big.Int
}

func (p *premineParams) validateFlags() (err error) {
	if p.nativeTokenRoot == "" {
		return rootHelper.ErrMandatoryERC20Token
	}

	if err := types.IsValidAddress(p.nativeTokenRoot); err != nil {
		return fmt.Errorf("invalid erc20 token address is provided: %w", err)
	}

	if p.customSupernetManager == "" {
		return errMandatorySupernetManagerAddr
	}

	if err := types.IsValidAddress(p.customSupernetManager); err != nil {
		return fmt.Errorf("invalid supernet manager address is provided: %w", err)
	}

	if p.rootERC20Predicate == "" {
		return errMandatoryRootPredicateAddr
	}

	if err := types.IsValidAddress(p.rootERC20Predicate); err != nil {
		return fmt.Errorf("invalid root erc20 predicate address is provided: %w", err)
	}

	if p.amountValue, err = helper.ParseAmount(p.amount); err != nil {
		return err
	}

	// validate jsonrpc address
	if _, err := helper.ParseJSONRPCAddress(p.jsonRPC); err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	if p.privateKey == "" {
		return sidechainHelper.ValidateSecretFlags(p.accountDir, p.accountConfig)
	}

	return nil
}

type premineResult struct {
	Address string   `json:"address"`
	Amount  *big.Int `json:"amount"`
}

func (p premineResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[NATIVE ROOT TOKEN PREMINE]\n")

	vals := make([]string, 0, 2)
	vals = append(vals, fmt.Sprintf("Address|%s", p.Address))
	vals = append(vals, fmt.Sprintf("Amount Premined|%d", p.Amount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
