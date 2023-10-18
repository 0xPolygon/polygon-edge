package polybft

import (
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/syncer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
	bolt "go.etcd.io/bbolt"
)

var _ blockchainBackend = (*blockchainMock)(nil)

type blockchainMock struct {
	mock.Mock
}

func (m *blockchainMock) CurrentHeader() *types.Header {
	args := m.Called()

	return args.Get(0).(*types.Header) //nolint:forcetypeassert
}

func (m *blockchainMock) CommitBlock(block *types.FullBlock) error {
	args := m.Called(block)

	return args.Error(0)
}

func (m *blockchainMock) NewBlockBuilder(parent *types.Header, coinbase types.Address,
	txPool txPoolInterface, blockTime time.Duration, logger hclog.Logger) (blockBuilder, error) {
	args := m.Called()

	return args.Get(0).(blockBuilder), args.Error(1) //nolint:forcetypeassert
}

func (m *blockchainMock) ProcessBlock(parent *types.Header, block *types.Block) (*types.FullBlock, error) {
	args := m.Called(parent, block)

	return args.Get(0).(*types.FullBlock), args.Error(1) //nolint:forcetypeassert
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

	if len(args) == 1 {
		header, ok := args.Get(0).(*types.Header)

		if ok {
			return header, true
		}

		getHeaderCallback, ok := args.Get(0).(func(number uint64) *types.Header)
		if ok {
			h := getHeaderCallback(number)

			return h, h != nil
		}
	} else if len(args) == 2 {
		return args.Get(0).(*types.Header), args.Get(1).(bool) //nolint:forcetypeassert
	}

	panic("Unsupported mock for GetHeaderByNumber") //nolint:gocritic
}

func (m *blockchainMock) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	args := m.Called(hash)
	header, ok := args.Get(0).(*types.Header)

	if ok {
		return header, true
	}

	getHeaderCallback, ok := args.Get(0).(func(hash types.Hash) *types.Header)
	if ok {
		h := getHeaderCallback(hash)

		return h, h != nil
	}

	panic("Unsupported mock for GetHeaderByHash") //nolint:gocritic
}

func (m *blockchainMock) GetSystemState(provider contract.Provider) SystemState {
	args := m.Called(provider)

	return args.Get(0).(SystemState) //nolint:forcetypeassert
}

func (m *blockchainMock) SubscribeEvents() blockchain.Subscription {
	return nil
}

func (m *blockchainMock) UnubscribeEvents(blockchain.Subscription) {
}

func (m *blockchainMock) CalculateGasLimit(number uint64) (uint64, error) {
	return 0, nil
}

func (m *blockchainMock) GetChainID() uint64 {
	return 0
}

func (m *blockchainMock) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	args := m.Called(hash)

	return args.Get(0).([]*types.Receipt), args.Error(1) //nolint:forcetypeassert
}

var _ polybftBackend = (*polybftBackendMock)(nil)

type polybftBackendMock struct {
	mock.Mock
}

// GetValidators retrieves validator set for the given block
func (p *polybftBackendMock) GetValidators(blockNumber uint64, parents []*types.Header) (validator.AccountSet, error) {
	args := p.Called(blockNumber, parents)
	if len(args) == 1 {
		accountSet, _ := args.Get(0).(validator.AccountSet)

		return accountSet, nil
	} else if len(args) == 2 {
		accountSet, _ := args.Get(0).(validator.AccountSet)

		return accountSet, args.Error(1)
	}

	panic("polybftBackendMock.GetValidators doesn't support such combination of arguments") //nolint:gocritic
}

func (p *polybftBackendMock) GetValidatorsWithTx(blockNumber uint64, parents []*types.Header,
	dbTx *bolt.Tx) (validator.AccountSet, error) {
	args := p.Called(blockNumber, parents, dbTx)
	if len(args) == 1 {
		accountSet, _ := args.Get(0).(validator.AccountSet)

		return accountSet, nil
	} else if len(args) == 2 {
		accountSet, _ := args.Get(0).(validator.AccountSet)

		return accountSet, args.Error(1)
	}

	panic("polybftBackendMock.GetValidatorsWithTx doesn't support such combination of arguments") //nolint:gocritic
}

var _ blockBuilder = (*blockBuilderMock)(nil)

type blockBuilderMock struct {
	mock.Mock
}

func (m *blockBuilderMock) Reset() error {
	args := m.Called()
	if len(args) == 0 {
		return nil
	}

	return args.Error(0)
}

func (m *blockBuilderMock) WriteTx(tx *types.Transaction) error {
	args := m.Called(tx)
	if len(args) == 0 {
		return nil
	}

	return args.Error(0)
}

func (m *blockBuilderMock) Fill() {
	m.Called()
}

// Receipts returns the collection of transaction receipts for given block
func (m *blockBuilderMock) Receipts() []*types.Receipt {
	args := m.Called()

	return args.Get(0).([]*types.Receipt) //nolint:forcetypeassert
}

func (m *blockBuilderMock) Build(handler func(*types.Header)) (*types.FullBlock, error) {
	args := m.Called(handler)
	builtBlock := args.Get(0).(*types.FullBlock) //nolint:forcetypeassert

	handler(builtBlock.Block.Header)

	return builtBlock, nil
}

func (m *blockBuilderMock) GetState() *state.Transition {
	args := m.Called()

	return args.Get(0).(*state.Transition) //nolint:forcetypeassert
}

var _ SystemState = (*systemStateMock)(nil)

type systemStateMock struct {
	mock.Mock
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

var _ contract.Provider = (*stateProviderMock)(nil)

type stateProviderMock struct {
	mock.Mock
}

func (s *stateProviderMock) Call(ethgo.Address, []byte, *contract.CallOpts) ([]byte, error) {
	return nil, nil
}

func (s *stateProviderMock) Txn(ethgo.Address, ethgo.Key, []byte) (contract.Txn, error) {
	return nil, nil
}

var _ BridgeTransport = (*transportMock)(nil)

type transportMock struct {
	mock.Mock
}

func (t *transportMock) Multicast(msg interface{}) {
	_ = t.Called(msg)
}

type testHeadersMap struct {
	headersByNumber map[uint64]*types.Header
}

func (t *testHeadersMap) addHeader(header *types.Header) {
	if t.headersByNumber == nil {
		t.headersByNumber = map[uint64]*types.Header{}
	}

	t.headersByNumber[header.Number] = header
}

func (t *testHeadersMap) getHeader(number uint64) *types.Header {
	return t.headersByNumber[number]
}

func (t *testHeadersMap) getHeaderByHash(hash types.Hash) *types.Header {
	for _, header := range t.headersByNumber {
		if header.Hash == hash {
			return header
		}
	}

	return nil
}

func (t *testHeadersMap) getHeaders() []*types.Header {
	headers := make([]*types.Header, 0, len(t.headersByNumber))
	for _, header := range t.headersByNumber {
		headers = append(headers, header)
	}

	return headers
}

var _ txPoolInterface = (*txPoolMock)(nil)

type txPoolMock struct {
	mock.Mock
}

func (tp *txPoolMock) Prepare() {
	tp.Called()
}

func (tp *txPoolMock) Length() uint64 {
	args := tp.Called()

	return args[0].(uint64) //nolint
}

func (tp *txPoolMock) Peek() *types.Transaction {
	args := tp.Called()

	return args[0].(*types.Transaction) //nolint
}

func (tp *txPoolMock) Pop(tx *types.Transaction) {
	tp.Called(tx)
}

func (tp *txPoolMock) Drop(tx *types.Transaction) {
	tp.Called(tx)
}

func (tp *txPoolMock) Demote(tx *types.Transaction) {
	tp.Called(tx)
}

func (tp *txPoolMock) SetSealing(v bool) {
	tp.Called(v)
}

func (tp *txPoolMock) ResetWithHeaders(values ...*types.Header) {
	tp.Called(values)
}

var _ syncer.Syncer = (*syncerMock)(nil)

type syncerMock struct {
	mock.Mock
}

func (tp *syncerMock) Start() error {
	args := tp.Called()

	return args.Error(0)
}

func (tp *syncerMock) Close() error {
	args := tp.Called()

	return args.Error(0)
}

func (tp *syncerMock) GetSyncProgression() *progress.Progression {
	args := tp.Called()

	return args[0].(*progress.Progression) //nolint
}

func (tp *syncerMock) HasSyncPeer() bool {
	args := tp.Called()

	return args[0].(bool) //nolint
}

func (tp *syncerMock) Sync(func(*types.FullBlock) bool) error {
	args := tp.Called()

	return args.Error(0)
}

func init() {
	// setup custom hash header func
	setupHeaderHashFunc()
}
