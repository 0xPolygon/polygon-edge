package common

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/command/sidechain"
)

const (
	SenderKeyFlag = "sender-key"
	ReceiversFlag = "receivers"
	AmountsFlag   = "amounts"
)

var (
	errInconsistentAccounts = errors.New("receivers and amounts must be equal length")
)

type ERC20BridgeParams struct {
	SecretsDataPath   string
	SecretsConfigPath string
	Receivers         []string
	Amounts           []string
}

func (bp *ERC20BridgeParams) ValidateFlags(testMode bool) error {
	// in case of test mode test rootchain account is being used as the rootchain transactions sender
	if !testMode {
		if err := sidechain.ValidateSecretFlags(bp.SecretsDataPath, bp.SecretsConfigPath); err != nil {
			return err
		}
	}

	if len(bp.Receivers) != len(bp.Amounts) {
		return errInconsistentAccounts
	}

	return nil
}
