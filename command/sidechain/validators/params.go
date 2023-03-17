package validators

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

type validatorInfoParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
}

func (v *validatorInfoParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
}

type validatorsInfoResult struct {
	address             string
	stake               uint64
	totalStake          uint64
	commission          uint64
	withdrawableRewards uint64
	active              bool
}

func (vr validatorsInfoResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[VALIDATOR INFO]\n")

	vals := make([]string, 0, 5)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", vr.address))
	vals = append(vals, fmt.Sprintf("Self Stake|%v", vr.stake))
	vals = append(vals, fmt.Sprintf("Total Stake|%v", vr.totalStake))
	vals = append(vals, fmt.Sprintf("Withdrawable Rewards|%v", vr.withdrawableRewards))
	vals = append(vals, fmt.Sprintf("Commission|%v", vr.commission))
	vals = append(vals, fmt.Sprintf("Is Active|%v", vr.active))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
