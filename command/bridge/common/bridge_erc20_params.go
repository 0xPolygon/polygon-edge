package common

import (
	"errors"
)

const (
	SenderKeyFlag = "sender-key"
	ReceiversFlag = "receivers"
	AmountsFlag   = "amounts"
)

var (
	errInconsistentAccounts = errors.New("receivers and amounts must be equal length")
)

// TODO: Rename to BridgeParams and make separate struct for erc 20 params
// (everything except Amounts remains here)
type ERC20BridgeParams struct {
	SenderKey string
	Receivers []string
	Amounts   []string
}

func (bp *ERC20BridgeParams) ValidateFlags() error {
	if len(bp.Receivers) != len(bp.Amounts) {
		return errInconsistentAccounts
	}

	return nil
}
