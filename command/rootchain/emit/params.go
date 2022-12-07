package emit

import "errors"

const (
	contractFlag = "contract"
	walletsFlag  = "wallets"
	amountsFlag  = "amounts"
	jsonRPCFlag  = "json-rpc"
	adminKeyFlag = "admin-key"
)

var (
	errWalletsMissing       = errors.New("wallet flag value is not provided")
	errAmountsMissing       = errors.New("amount flag value is not provided")
	errInconsistentAccounts = errors.New("wallets and amounts must be provided in pairs")
)

type emitParams struct {
	address  string
	wallets  []string
	amounts  []string
	adminKey string
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
