package initcontracts

const (
	contractsPathFlag       = "path"
	validatorPrefixPathFlag = "validator-prefix"
	validatorPathFlag       = "validator-path"

	defaultValidatorPrefixPath = "test-chain-"
	defaultValidatorPath       = "./"
)

type initContractsParams struct {
	contractsPath       string
	validatorPrefixPath string
	validatorPath       string
}

func (ep *initContractsParams) validateFlags() error {
	return nil
}
