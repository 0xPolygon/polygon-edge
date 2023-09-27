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
	if _, err := helper.ParseJSONRPCAddress(v.jsonRPC); err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
}

type validatorsInfoResult struct {
	Address     string `json:"address"`
	Stake       uint64 `json:"stake"`
	Active      bool   `json:"active"`
	Whitelisted bool   `json:"whitelisted"`
}

func (vr validatorsInfoResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[VALIDATOR INFO]\n")

	vals := make([]string, 4)
	vals[0] = fmt.Sprintf("Validator Address|%s", vr.Address)
	vals[1] = fmt.Sprintf("Stake|%v", vr.Stake)
	vals[2] = fmt.Sprintf("Is Whitelisted|%v", vr.Whitelisted)
	vals[3] = fmt.Sprintf("Is Active|%v", vr.Active)

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
