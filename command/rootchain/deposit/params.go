package deposit

import (
	"errors"
	"fmt"
	"os"
)

const (
	manifestPathFlag = "manifest"
	tokenFlag        = "token"
	contractFlag     = "contract"
	receiversFlag    = "receivers"
	amountsFlag      = "amounts"
	jsonRPCFlag      = "json-rpc"
	adminKeyFlag     = "admin-key"
)

var (
	errWalletsMissing       = errors.New("receivers flag value is not provided")
	errAmountsMissing       = errors.New("amount flag value is not provided")
	errInconsistentAccounts = errors.New("receivers and amounts must be provided in pairs")
)

type depositParams struct {
	manifestPath   string
	tokenTypeRaw   string
	receivers      []string
	amounts        []string
	jsonRPCAddress string
	adminKey       string
}

func (dp *depositParams) validateFlags() error {
	if _, err := os.Stat(dp.manifestPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided manifest path '%s' doesn't exist", dp.manifestPath)
	}

	if len(dp.receivers) == 0 {
		return errWalletsMissing
	}

	if len(dp.amounts) == 0 {
		return errAmountsMissing
	}

	if len(dp.receivers) != len(dp.amounts) {
		return errInconsistentAccounts
	}

	if _, exists := lookupTokenType(dp.tokenTypeRaw); !exists {
		return fmt.Errorf("unrecognized token type provided: %s", dp.tokenTypeRaw)
	}

	return nil
}
