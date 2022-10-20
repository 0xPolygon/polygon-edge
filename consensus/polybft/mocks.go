package polybft

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/mock"
)

var _ blockBuilder = &blockBuilderMock{}

type blockBuilderMock struct {
	mock.Mock
}

func (m *blockBuilderMock) Reset() error {
	_ = m.Called()
	return nil
}

func (m *blockBuilderMock) WriteTx(tx *types.Transaction) error {
	args := m.Called(tx)
	if len(args) == 0 {
		return nil
	}
	return args.Error(0)
}

func (m *blockBuilderMock) Fill() error {
	args := m.Called()
	if len(args) == 0 {
		return nil
	}
	return args.Error(0)
}

func (m *blockBuilderMock) Build(handler func(*types.Header)) (*StateBlock, error) {
	args := m.Called(handler)
	builtBlock := args.Get(0).(*StateBlock)
	handler(builtBlock.Block.Header)
	return builtBlock, nil
}

func (m *blockBuilderMock) GetState() *state.Transition {
	args := m.Called()
	return args.Get(0).(*state.Transition)
}

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
