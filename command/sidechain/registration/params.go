package registration

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	stakeFlag   = "stake"
	chainIDFlag = "chain-id"
)

type registerParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
	stake         string
	chainID       int64
}

func (rp *registerParams) validateFlags() error {
	if err := sidechainHelper.ValidateSecretFlags(rp.accountDir, rp.accountConfig); err != nil {
		return err
	}

	if rp.stake != "" {
		_, err := types.ParseUint256orHex(&rp.stake)
		if err != nil {
			return fmt.Errorf("provided stake '%s' isn't valid", rp.stake)
		}
	}

	return nil
}

type registerResult struct {
	validatorAddress string
	stakeResult      string
	amount           uint64
}

func (rr registerResult) GetOutput() string {
	var buffer bytes.Buffer

	var vals []string

	buffer.WriteString("\n[REGISTRATION]\n")

	vals = make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", rr.validatorAddress))
	vals = append(vals, fmt.Sprintf("Staking Result|%s", rr.stakeResult))
	vals = append(vals, fmt.Sprintf("Amount Staked|%v", rr.amount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
