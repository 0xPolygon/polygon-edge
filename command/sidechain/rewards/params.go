package rewards

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

type withdrawRewardsParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
}

type withdrawRewardResult struct {
	ValidatorAddress string `json:"validatorAddress"`
	RewardAmount     uint64 `json:"rewardAmount"`
}

func (w *withdrawRewardsParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(w.accountDir, w.accountConfig)
}

func (wr withdrawRewardResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[WITHDRAW REWARDS]\n")

	vals := make([]string, 0, 2)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", wr.ValidatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Withdrawn|%v", wr.RewardAmount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
