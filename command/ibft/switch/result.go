package ibftswitch

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/fork"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/validators"
)

type IBFTSwitchResult struct {
	Chain             string                   `json:"chain"`
	Type              fork.IBFTType            `json:"type"`
	ValidatorType     validators.ValidatorType `json:"validator_type"`
	From              common.JSONNumber        `json:"from"`
	Deployment        *common.JSONNumber       `json:"deployment,omitempty"`
	MaxValidatorCount common.JSONNumber        `json:"maxValidatorCount"`
	MinValidatorCount common.JSONNumber        `json:"minValidatorCount"`
}

func (r *IBFTSwitchResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[NEW IBFT FORK]\n")

	outputs := []string{
		fmt.Sprintf("Chain|%s", r.Chain),
		fmt.Sprintf("Type|%s", r.Type),
		fmt.Sprintf("ValidatorType|%s", r.ValidatorType),
	}

	if r.Deployment != nil {
		outputs = append(outputs, fmt.Sprintf("Deployment|%d", r.Deployment.Value))
	}

	outputs = append(outputs, fmt.Sprintf("From|%d", r.From.Value))

	if r.Type == fork.PoS {
		outputs = append(outputs,
			fmt.Sprintf("MaxValidatorCount|%d", r.MaxValidatorCount.Value),
			fmt.Sprintf("MinValidatorCount|%d", r.MinValidatorCount.Value),
		)
	}

	buffer.WriteString(helper.FormatKV(outputs))
	buffer.WriteString("\n")

	return buffer.String()
}
