package show

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/common"
)

type ShowResult struct {
	GenesisConfig *chain.Chain
}

func (r *ShowResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[WHITELISTS]\n\n")

	deploymentWhitelist, err := common.FetchDeploymentWhitelist(r.GenesisConfig)
	if err != nil {
		return err.Error()
	}

	buffer.WriteString(fmt.Sprintf("Contract deployment whitelist : %s,\n", deploymentWhitelist))

	return buffer.String()
}
