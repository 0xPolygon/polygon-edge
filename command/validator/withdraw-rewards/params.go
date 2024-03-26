package rewards

import (
	"bytes"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/command/helper"
	validatorHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
)

type withdrawRewardsParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
	txTimeout     time.Duration
}

type withdrawRewardResult struct {
	ValidatorAddress string `json:"validatorAddress"`
	RewardAmount     uint64 `json:"rewardAmount"`
}

func (w *withdrawRewardsParams) validateFlags() error {
	if _, err := helper.ParseJSONRPCAddress(w.jsonRPC); err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	return validatorHelper.ValidateSecretFlags(w.accountDir, w.accountConfig)
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
