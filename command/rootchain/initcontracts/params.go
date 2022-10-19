package initcontracts

const (
	contractsPathFlag       = "path"
	validatorPrefixPathFlag = "validator-prefix"
	validatorPathFlag       = "validator-path"

	defaultContractsPath       = "./v3-contracts/artifacts/contracts/"
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
