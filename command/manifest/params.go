package manifest

const (
	manifestPathFlag      = "manifest-path"
	premineValidatorsFlag = "premine-validators"
	validatorsFlag        = "validators"
	validatorsPathFlat    = "validators-path"
	validatorsPrefixFlag  = "validators-prefix"

	defaultPolyBftValidatorPrefixPath = "test-chain-"
	defaultManifestPath               = "./manifest.json"
)

var (
	params = &manifestParams{}
)

type manifestParams struct {
	manifestPath      string
	premineValidators string

	validators           []string
	validatorsPrefixPath string
}
