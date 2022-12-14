package validators

import (
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

type validatorInfoParams struct {
	accountDir string
	jsonRPC    string
}

func (v *validatorInfoParams) validateFlags() error {
	return sidechainHelper.CheckIfDirectoryExist(v.accountDir)
}
