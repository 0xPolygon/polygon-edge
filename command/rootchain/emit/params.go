package emit

import "errors"

const (
	contractFlag = "contract"
	walletsFlag  = "wallets"
	amountsFlag  = "amounts"
)

var (
	errWalletsMissing       = errors.New("wallet flag value is not provided")
	errAmountsMissing       = errors.New("amount flag value is not provided")
	errInconsistentAccounts = errors.New("wallets and amounts must be provided in same numbers")
)

type emitParams struct {
	contractAddrRaw string
	wallets         []string
	amounts         []string
}

func (ep *emitParams) validateFlags() error {
	if len(ep.wallets) == 0 {
		return errWalletsMissing
	}

	if len(ep.amounts) == 0 {
		return errAmountsMissing
	}

	if len(ep.wallets) != len(ep.amounts) {
		return errInconsistentAccounts
	}

	return nil
}
