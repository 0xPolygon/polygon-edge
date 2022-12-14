package validators

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type validatorsInfoResult struct {
	address             string
	stake               uint64
	totalStake          uint64
	commission          uint64
	withdrawableRewards uint64
}

func (vr validatorsInfoResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[VALIDATOR INFO]\n")

	vals := make([]string, 0, 5)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", vr.address))
	vals = append(vals, fmt.Sprintf("Active Stake|%v", vr.stake))
	vals = append(vals, fmt.Sprintf("Total Stake|%v", vr.totalStake))
	vals = append(vals, fmt.Sprintf("Withdrawable Rewards|%v", vr.withdrawableRewards))
	vals = append(vals, fmt.Sprintf("Commission|%v", vr.commission))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
