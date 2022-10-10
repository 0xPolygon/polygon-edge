package polybft

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
)

type testValidator struct {
	alias   string
	account *wallet.Account
}

type testValidators struct {
	validators map[string]*testValidator
}

func newTestValidator(alias string) *testValidator {
	return &testValidator{alias: alias, account: wallet.GenerateAccount()}
}

func newTestValidators(validatorsCount int) *testValidators {
	aliases := make([]string, validatorsCount)
	for i := 0; i < validatorsCount; i++ {
		aliases[i] = strconv.Itoa(i)
	}

	return newTestValidatorsWithAliases(aliases)
}

func (v *testValidators) getPublicIdentities(aliases ...string) (res AccountSet) { //nolint:unparam
	v.iterAcct(aliases, func(t *testValidator) {
		res = append(res, t.ValidatorAccount())
	})

	return
}

func (v *testValidators) getValidators(aliases ...string) (res []*testValidator) {
	v.iterAcct(aliases, func(t *testValidator) {
		res = append(res, t)
	})

	return
}

func (v *testValidators) toValidatorSet() *validatorSet {
	return newValidatorSet(types.Address{}, v.getPublicIdentities())
}

func (v *testValidator) ValidatorAccount() *ValidatorAccount {
	return &ValidatorAccount{
		Address: types.Address(v.account.Ecdsa.Address()),
		BlsKey:  v.account.Bls.PublicKey(),
	}
}

func (v *testValidators) iterAcct(aliases []string, handle func(t *testValidator)) {
	if len(aliases) == 0 {
		// loop over the whole set
		for k := range v.validators {
			aliases = append(aliases, k)
		}
		// sort the names since they get queried randomly
		sort.Strings(aliases)
	}

	for _, alias := range aliases {
		handle(v.getValidator(alias))
	}
}

func (v *testValidators) getValidator(alias string) *testValidator {
	vv, ok := v.validators[alias]
	if !ok {
		panic(fmt.Sprintf("BUG: validator %s does not exist", alias))
	}

	return vv
}

func (v *testValidators) getParamValidators(aliases ...string) (res []*Validator) {
	v.iterAcct(aliases, func(t *testValidator) {
		res = append(res, t.paramsValidator())
	})

	return
}

func (v *testValidator) paramsValidator() *Validator {
	bls := v.account.Bls.PublicKey().Marshal()

	return &Validator{
		Address: v.Address(),
		BlsKey:  hex.EncodeToHex(bls),
	}
}

func newTestValidatorsWithAliases(aliases []string) *testValidators {
	validators := map[string]*testValidator{}
	for _, alias := range aliases {
		validators[alias] = newTestValidator(alias)
	}

	return &testValidators{
		validators: validators,
	}
}

func (v *testValidator) Address() types.Address {
	return types.Address(v.account.Ecdsa.Address())
}

func (v *testValidators) create(alias string) {
	if _, ok := v.validators[alias]; !ok {
		v.validators[alias] = newTestValidator(alias)
	}
}
