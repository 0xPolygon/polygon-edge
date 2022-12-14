package withdraw

import (
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

var (
	addressToFlag = "to"
)

type withdrawParams struct {
	accountDir string
	jsonRPC    string
	addressTo  string
}

func (v *withdrawParams) validateFlags() error {
	return sidechainHelper.CheckIfDirectoryExist(v.accountDir)
}
