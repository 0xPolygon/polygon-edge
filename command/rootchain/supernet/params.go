package supernet

import (
	"bytes"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

const (
	finalizeGenesisSetFlag = "finalize-genesis-set"
	enableStakingFlag      = "enable-staking"
)

type supernetParams struct {
	accountDir             string
	accountConfig          string
	privateKey             string
	jsonRPC                string
	supernetManagerAddress string
	genesisPath            string
	stakeManagerAddress    string
	finalizeGenesisSet     bool
	enableStaking          bool
}

func (sp *supernetParams) validateFlags() error {
	if sp.privateKey == "" {
		return sidechainHelper.ValidateSecretFlags(sp.accountDir, sp.accountConfig)
	}

	if _, err := os.Stat(sp.genesisPath); err != nil {
		return fmt.Errorf("provided genesis path '%s' is invalid. Error: %w ", sp.genesisPath, err)
	}

	// validate jsonrpc address
	_, err := helper.ParseJSONRPCAddress(sp.jsonRPC)

	return err
}

type supernetResult struct {
	IsGenesisSetFinalized bool `json:"isGenesisSetFinalized"`
	IsStakingEnabled      bool `json:"isStakingEnabled"`
}

func (sr supernetResult) GetOutput() string {
	var (
		buffer bytes.Buffer
		vals   []string
	)

	buffer.WriteString("\n[SUPERNET COMMAND]\n")

	if sr.IsGenesisSetFinalized {
		vals = append(vals, "Genesis validator set finalized on supernet manager")
	}

	if sr.IsStakingEnabled {
		vals = append(vals, "Staking enabled on supernet manager")
	}

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
