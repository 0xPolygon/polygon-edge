package validator

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

type TestValidators struct {
	Validators map[string]*TestValidator
}

func NewTestValidators(tb testing.TB, validatorsCount int) *TestValidators {
	tb.Helper()

	aliases := make([]string, validatorsCount)
	for i := 0; i < validatorsCount; i++ {
		aliases[i] = strconv.Itoa(i)
	}

	return NewTestValidatorsWithAliases(tb, aliases)
}

func NewTestValidatorsWithAliases(tb testing.TB, aliases []string, votingPowers ...[]uint64) *TestValidators {
	tb.Helper()

	validators := map[string]*TestValidator{}

	for i, alias := range aliases {
		votingPower := uint64(1)
		if len(votingPowers) == 1 {
			votingPower = votingPowers[0][i]
		}

		validators[alias] = NewTestValidator(tb, alias, votingPower)
	}

	return &TestValidators{Validators: validators}
}

func (v *TestValidators) Create(t *testing.T, alias string, votingPower uint64) {
	t.Helper()

	if _, ok := v.Validators[alias]; !ok {
		v.Validators[alias] = NewTestValidator(t, alias, votingPower)
	}
}

func (v *TestValidators) IterAcct(aliases []string, handle func(t *TestValidator)) {
	if len(aliases) == 0 {
		// loop over the whole set
		for k := range v.Validators {
			aliases = append(aliases, k)
		}
		// sort the names since they get queried randomly
		sort.Strings(aliases)
	}

	for _, alias := range aliases {
		handle(v.GetValidator(alias))
	}
}

func (v *TestValidators) GetParamValidators(aliases ...string) (res []*GenesisValidator) {
	v.IterAcct(aliases, func(t *TestValidator) {
		res = append(res, t.ParamsValidator())
	})

	return
}

func (v *TestValidators) GetValidators(aliases ...string) (res []*TestValidator) {
	v.IterAcct(aliases, func(t *TestValidator) {
		res = append(res, t)
	})

	return
}

func (v *TestValidators) GetPublicIdentities(aliases ...string) (res AccountSet) {
	v.IterAcct(aliases, func(t *TestValidator) {
		res = append(res, t.ValidatorMetadata())
	})

	return
}

func (v *TestValidators) GetPrivateIdentities(aliases ...string) (res []*wallet.Account) {
	v.IterAcct(aliases, func(t *TestValidator) {
		res = append(res, t.Account)
	})

	return
}

func (v *TestValidators) GetValidator(alias string) *TestValidator {
	vv, ok := v.Validators[alias]
	if !ok {
		panic(fmt.Sprintf("Validator %s does not exist", alias)) //nolint:gocritic
	}

	return vv
}

func (v *TestValidators) ToValidatorSet() ValidatorSet {
	return NewValidatorSet(v.GetPublicIdentities(), hclog.NewNullLogger())
}

func (v *TestValidators) UpdateVotingPowers(votingPowersMap map[string]uint64) AccountSet {
	if len(votingPowersMap) == 0 {
		return AccountSet{}
	}

	aliases := []string{}
	for alias := range votingPowersMap {
		aliases = append(aliases, alias)
	}

	v.IterAcct(aliases, func(t *TestValidator) {
		t.VotingPower = votingPowersMap[t.Alias]
	})

	return v.GetPublicIdentities(aliases...)
}

type TestValidator struct {
	Alias       string
	Account     *wallet.Account
	VotingPower uint64
}

func NewTestValidator(tb testing.TB, alias string, votingPower uint64) *TestValidator {
	tb.Helper()

	return &TestValidator{
		Alias:       alias,
		VotingPower: votingPower,
		Account:     generateTestAccount(tb),
	}
}

func (v *TestValidator) Address() types.Address {
	return types.Address(v.Account.Ecdsa.Address())
}

func (v *TestValidator) Key() *wallet.Key {
	return wallet.NewKey(v.Account)
}

func (v *TestValidator) ParamsValidator() *GenesisValidator {
	bls := v.Account.Bls.PublicKey().Marshal()

	return &GenesisValidator{
		Address: v.Address(),
		BlsKey:  hex.EncodeToString(bls),
		Stake:   big.NewInt(1000),
	}
}

func (v *TestValidator) ValidatorMetadata() *ValidatorMetadata {
	return &ValidatorMetadata{
		Address:     types.Address(v.Account.Ecdsa.Address()),
		BlsKey:      v.Account.Bls.PublicKey(),
		VotingPower: new(big.Int).SetUint64(v.VotingPower),
	}
}

func (v *TestValidator) MustSign(hash, domain []byte) *bls.Signature {
	signature, err := v.Account.Bls.Sign(hash, domain)
	if err != nil {
		panic(fmt.Sprintf("BUG: failed to sign: %v", err)) //nolint:gocritic
	}

	return signature
}

func generateTestAccount(tb testing.TB) *wallet.Account {
	tb.Helper()

	acc, err := wallet.GenerateAccount()
	require.NoError(tb, err)

	return acc
}

// CreateValidatorSetDelta calculates ValidatorSetDelta based on the provided old and new validator sets
func CreateValidatorSetDelta(oldValidatorSet, newValidatorSet AccountSet) (*ValidatorSetDelta, error) {
	var addedValidators, updatedValidators AccountSet

	oldValidatorSetMap := make(map[types.Address]*ValidatorMetadata)
	removedValidators := map[types.Address]int{}

	for i, validator := range oldValidatorSet {
		if (validator.Address != types.Address{}) {
			removedValidators[validator.Address] = i
			oldValidatorSetMap[validator.Address] = validator
		}
	}

	for _, newValidator := range newValidatorSet {
		// Check if the validator is among both old and new validator set
		oldValidator, validatorExists := oldValidatorSetMap[newValidator.Address]
		if validatorExists {
			if !oldValidator.EqualAddressAndBlsKey(newValidator) {
				return nil, fmt.Errorf("validator '%s' found in both old and new validator set, but its BLS keys differ",
					newValidator.Address.String())
			}

			// If it is, then discard it from removed validators...
			delete(removedValidators, newValidator.Address)

			if !oldValidator.Equals(newValidator) {
				updatedValidators = append(updatedValidators, newValidator)
			}
		} else {
			// ...otherwise it is added
			addedValidators = append(addedValidators, newValidator)
		}
	}

	removedValsBitmap := bitmap.Bitmap{}
	for _, i := range removedValidators {
		removedValsBitmap.Set(uint64(i))
	}

	delta := &ValidatorSetDelta{
		Added:   addedValidators,
		Updated: updatedValidators,
		Removed: removedValsBitmap,
	}

	return delta, nil
}
