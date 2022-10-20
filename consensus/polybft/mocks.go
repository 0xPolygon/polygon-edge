package polybft

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
)

var _ blockchainBackend = &blockchainMock{}

type blockchainMock struct {
	mock.Mock
}

func (m *blockchainMock) CurrentHeader() *types.Header {
	args := m.Called()

	return args.Get(0).(*types.Header) //nolint:forcetypeassert
}

func (m *blockchainMock) CommitBlock(stateBlock *StateBlock) error {
	args := m.Called(stateBlock)

	return args.Error(0)
}

func (m *blockchainMock) NewBlockBuilder(parent *types.Header, coinbase types.Address,
	txPool txPoolInterface, blockTime time.Duration, logger hclog.Logger) (blockBuilder, error) {
	args := m.Called()

	return args.Get(0).(blockBuilder), args.Error(1) //nolint:forcetypeassert
}

func (m *blockchainMock) ProcessBlock(parent *types.Header, block *types.Block) (*StateBlock, error) {
	args := m.Called(parent, block)

	return args.Get(0).(*StateBlock), args.Error(1) //nolint:forcetypeassert
}

func (m *blockchainMock) GetStateProviderForBlock(block *types.Header) (contract.Provider, error) {
	args := m.Called(block)
	stateProvider, _ := args.Get(0).(contract.Provider)

	return stateProvider, nil
}

func (m *blockchainMock) GetStateProvider(transition *state.Transition) contract.Provider {
	args := m.Called()
	stateProvider, _ := args.Get(0).(contract.Provider)

	return stateProvider
}

func (m *blockchainMock) GetHeaderByNumber(number uint64) (*types.Header, bool) {
	args := m.Called(number)
	header, ok := args.Get(0).(*types.Header)

	if ok {
		return header, true
	}

	getHeaderCallback, ok := args.Get(0).(func(number uint64) *types.Header)
	if ok {
		return getHeaderCallback(number), true
	}

	panic("Unsupported mock for GetHeaderByNumber")
}

func (m *blockchainMock) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	return nil, false
}

func (m *blockchainMock) GetSystemState(config *PolyBFTConfig, provider contract.Provider) SystemState {
	args := m.Called(config, provider)

	return args.Get(0).(SystemState) //nolint:forcetypeassert
}

func (m *blockchainMock) SubscribeEvents() blockchain.Subscription {
	return nil
}

func (m *blockchainMock) CalculateGasLimit(number uint64) (uint64, error) {
	return 0, nil
}

var _ polybftBackend = &polybftBackendMock{}

type polybftBackendMock struct {
	mock.Mock
}

// CheckIfStuck checks if state machine is stuck.
func (p *polybftBackendMock) CheckIfStuck(num uint64) (uint64, bool) {
	args := p.Called(num)

	if len(args) == 2 {
		return args.Get(0).(uint64), args.Bool(1) //nolint:forcetypeassert
	} else if len(args) == 1 {
		peerHeight, ok := args.Get(0).(uint64)
		if ok {
			return peerHeight, num < peerHeight
		}

		return 0, args.Bool(0)
	}

	return 0, false
}

// GetValidators retrieves validator set for the given block
func (p *polybftBackendMock) GetValidators(blockNumber uint64, parents []*types.Header) (AccountSet, error) {
	args := p.Called(blockNumber, parents)
	if len(args) == 1 {
		accountSet, _ := args.Get(0).(AccountSet)

		return accountSet, nil
	} else if len(args) == 2 {
		accountSet, _ := args.Get(0).(AccountSet)

		return accountSet, args.Error(1)
	}

	panic("polybftBackendMock.GetValidators doesn't support such combination of arguments")
}

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
	builtBlock := args.Get(0).(*StateBlock) //nolint:forcetypeassert

	handler(builtBlock.Block.Header)

	return builtBlock, nil
}

func (m *blockBuilderMock) GetState() *state.Transition {
	args := m.Called()

	return args.Get(0).(*state.Transition) //nolint:forcetypeassert
}

var _ SystemState = &systemStateMock{}

type systemStateMock struct {
	mock.Mock
}

func (m *systemStateMock) GetValidatorSet() (AccountSet, error) {
	args := m.Called()
	if len(args) == 1 {
		accountSet, _ := args.Get(0).(AccountSet)

		return accountSet, nil
	} else if len(args) == 2 {
		accountSet, _ := args.Get(0).(AccountSet)

		return accountSet, args.Error(1)
	}

	panic("systemStateMock.GetValidatorSet doesn't support such combination of arguments")
}

func (m *systemStateMock) GetNextExecutionIndex() (uint64, error) {
	args := m.Called()
	if len(args) == 1 {
		index, _ := args.Get(0).(uint64)

		return index, nil
	} else if len(args) == 2 {
		index, _ := args.Get(0).(uint64)

		return index, args.Error(1)
	}

	return 0, nil
}

func (m *systemStateMock) GetNextCommittedIndex() (uint64, error) {
	args := m.Called()

	if len(args) == 1 {
		index, _ := args.Get(0).(uint64)

		return index, nil
	} else if len(args) == 2 {
		index, _ := args.Get(0).(uint64)

		return index, args.Error(1)
	}

	return 0, nil
}

func (m *systemStateMock) GetEpoch() (uint64, error) {
	args := m.Called()
	if len(args) == 1 {
		epochNumber, _ := args.Get(0).(uint64)

		return epochNumber, nil
	} else if len(args) == 2 {
		epochNumber, _ := args.Get(0).(uint64)
		err, ok := args.Get(1).(error)
		if ok {
			return epochNumber, err
		}

		return epochNumber, nil
	}

	return 0, nil
}

var _ contract.Provider = &stateProviderMock{}

type stateProviderMock struct {
	mock.Mock
}

func (s *stateProviderMock) Call(ethgo.Address, []byte, *contract.CallOpts) ([]byte, error) {
	return nil, nil
}

func (s *stateProviderMock) Txn(ethgo.Address, ethgo.Key, []byte) (contract.Txn, error) {
	return nil, nil
}

var _ Transport = &transportMock{}

type transportMock struct {
	mock.Mock
}

func (t *transportMock) Gossip(message interface{}) {
	_ = t.Called(message)
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

func (v *testValidators) getPrivateIdentities(aliases ...string) (res []*wallet.Account) {
	v.iterAcct(aliases, func(t *testValidator) {
		res = append(res, t.account)
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

func (v *testValidator) Key() *wallet.Key {
	return wallet.NewKey(v.account)
}

func (v *testValidators) create(alias string) {
	if _, ok := v.validators[alias]; !ok {
		v.validators[alias] = newTestValidator(alias)
	}
}

func (v *testValidator) mustSign(hash []byte) []byte {
	signature, err := v.Key().Sign(hash)
	if err != nil {
		panic(fmt.Sprintf("BUG: failed to sign: %v", err))
	}

	return signature
}
