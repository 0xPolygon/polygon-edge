package validators

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

type validatorInfoParams struct {
	accountDir             string
	accountConfig          string
	jsonRPC                string
	supernetManagerAddress string
	stakeManagerAddress    string
	chainID                int64
}

func (v *validatorInfoParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
}

type validatorsInfoResult struct {
	address     string
	stake       uint64
	active      bool
	whitelisted bool
}

func (vr validatorsInfoResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[VALIDATOR INFO]\n")

	vals := make([]string, 4)
	vals[0] = fmt.Sprintf("Validator Address|%s", vr.address)
	vals[1] = fmt.Sprintf("Stake|%v", vr.stake)
	vals[2] = fmt.Sprintf("Is Whitelisted|%v", vr.whitelisted)
	vals[3] = fmt.Sprintf("Is Active|%v", vr.active)

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
