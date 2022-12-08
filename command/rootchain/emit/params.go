package emit

import (
	"errors"
	"fmt"
	"os"
)

const (
	manifestPathFlag = "manifest"
	contractFlag     = "contract"
	walletsFlag      = "wallets"
	amountsFlag      = "amounts"
	jsonRPCFlag      = "json-rpc"
	adminKeyFlag     = "admin-key"
)

var (
	errWalletsMissing       = errors.New("wallet flag value is not provided")
	errAmountsMissing       = errors.New("amount flag value is not provided")
	errInconsistentAccounts = errors.New("wallets and amounts must be provided in pairs")
)

type emitParams struct {
	manifestPath   string
	address        string
	wallets        []string
	amounts        []string
	jsonRPCAddress string
	adminKey       string
}

func (ep *emitParams) validateFlags() error {
	if _, err := os.Stat(ep.manifestPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided manifest path '%s' doesn't exist", ep.manifestPath)
	}

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
