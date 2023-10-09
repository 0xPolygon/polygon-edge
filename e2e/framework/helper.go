package framework

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/contracts/abis"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/server/proto"
	txpoolProto "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"golang.org/x/crypto/sha3"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

var (
	DefaultTimeout = time.Minute
)

type AtomicErrors struct {
	sync.RWMutex
	errors []error
}

func NewAtomicErrors(capacity int) AtomicErrors {
	return AtomicErrors{
		errors: make([]error, 0, capacity),
	}
}

func (a *AtomicErrors) Append(err error) {
	a.Lock()
	defer a.Unlock()

	a.errors = append(a.errors, err)
}

func (a *AtomicErrors) Errors() []error {
	a.RLock()
	defer a.RUnlock()

	return a.errors
}

func EthToWei(ethValue int64) *big.Int {
	return EthToWeiPrecise(ethValue, 18)
}

func EthToWeiPrecise(ethValue int64, decimals int64) *big.Int {
	return new(big.Int).Mul(
		big.NewInt(ethValue),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil))
}

// GetAccountBalance is a helper method for fetching the Balance field of an account
func GetAccountBalance(t *testing.T, address types.Address, rpcClient *jsonrpc.Client) *big.Int {
	t.Helper()

	accountBalance, err := rpcClient.Eth().GetBalance(
		ethgo.Address(address),
		ethgo.Latest,
	)

	assert.NoError(t, err)

	return accountBalance
}

// GetValidatorSet returns the validator set from the SC
func GetValidatorSet(from types.Address, rpcClient *jsonrpc.Client) ([]types.Address, error) {
	validatorsMethod, ok := abis.StakingABI.Methods["validators"]
	if !ok {
		return nil, errors.New("validators method doesn't exist in Staking contract ABI")
	}

	toAddress := ethgo.Address(staking.AddrStakingContract)
	selector := validatorsMethod.ID()
	response, err := rpcClient.Eth().Call(
		&ethgo.CallMsg{
			From:     ethgo.Address(from),
			To:       &toAddress,
			Data:     selector,
			GasPrice: 1000000000,
			Value:    big.NewInt(0),
		},
		ethgo.Latest,
	)

	if err != nil {
		return nil, fmt.Errorf("unable to call Staking contract method validators, %w", err)
	}

	byteResponse, decodeError := hex.DecodeHex(response)
	if decodeError != nil {
		return nil, fmt.Errorf("unable to decode hex response, %w", decodeError)
	}

	return staking.DecodeValidators(validatorsMethod, byteResponse)
}

// StakeAmount is a helper function for staking an amount on the Staking SC
func StakeAmount(
	from types.Address,
	senderKey *ecdsa.PrivateKey,
	amount *big.Int,
	srv *TestServer,
) error {
	// Stake Balance
	txn := &PreparedTransaction{
		From:     from,
		To:       &staking.AddrStakingContract,
		GasPrice: ethgo.Gwei(1),
		Gas:      1000000,
		Value:    amount,
		Input:    MethodSig("stake"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	_, err := srv.SendRawTx(ctx, txn, senderKey)

	if err != nil {
		return fmt.Errorf("unable to call Staking contract method stake, %w", err)
	}

	return nil
}

// UnstakeAmount is a helper function for unstaking the entire amount on the Staking SC
func UnstakeAmount(
	from types.Address,
	senderKey *ecdsa.PrivateKey,
	srv *TestServer,
) (*ethgo.Receipt, error) {
	// Stake Balance
	txn := &PreparedTransaction{
		From:     from,
		To:       &staking.AddrStakingContract,
		GasPrice: ethgo.Gwei(1),
		Gas:      DefaultGasLimit,
		Value:    big.NewInt(0),
		Input:    MethodSig("unstake"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	receipt, err := srv.SendRawTx(ctx, txn, senderKey)

	if err != nil {
		return nil, fmt.Errorf("unable to call Staking contract method unstake, %w", err)
	}

	return receipt, nil
}

// GetStakedAmount is a helper function for getting the staked amount on the Staking SC
func GetStakedAmount(from types.Address, rpcClient *jsonrpc.Client) (*big.Int, error) {
	stakedAmountMethod, ok := abis.StakingABI.Methods["stakedAmount"]
	if !ok {
		return nil, errors.New("stakedAmount method doesn't exist in Staking contract ABI")
	}

	toAddress := ethgo.Address(staking.AddrStakingContract)
	selector := stakedAmountMethod.ID()
	response, err := rpcClient.Eth().Call(
		&ethgo.CallMsg{
			From:     ethgo.Address(from),
			To:       &toAddress,
			Data:     selector,
			GasPrice: 1000000000,
			Value:    big.NewInt(0),
		},
		ethgo.Latest,
	)

	if err != nil {
		return nil, fmt.Errorf("unable to call Staking contract method stakedAmount, %w", err)
	}

	bigResponse, decodeErr := common.ParseUint256orHex(&response)
	if decodeErr != nil {
		return nil, fmt.Errorf("unable to decode hex response")
	}

	return bigResponse, nil
}

func EcrecoverFromBlockhash(hash types.Hash, signature []byte) (types.Address, error) {
	pubKey, err := crypto.RecoverPubkey(signature, crypto.Keccak256(hash.Bytes()))
	if err != nil {
		return types.Address{}, err
	}

	return crypto.PubKeyToAddress(pubKey), nil
}

func MultiJoinSerial(t *testing.T, srvs []*TestServer) {
	t.Helper()

	dials := []*TestServer{}

	for i := 0; i < len(srvs)-1; i++ {
		srv, dst := srvs[i], srvs[i+1]
		dials = append(dials, srv, dst)
	}
	MultiJoin(t, dials...)
}

func MultiJoin(t *testing.T, srvs ...*TestServer) {
	t.Helper()

	if len(srvs)%2 != 0 {
		t.Fatal("not an even number")
	}

	errors := NewAtomicErrors(len(srvs) / 2)

	var wg sync.WaitGroup

	for i := 0; i < len(srvs); i += 2 {
		src, dst := srvs[i], srvs[i+1]
		srcIndex, dstIndex := i, i+1

		wg.Add(1)

		go func() {
			defer wg.Done()

			srcClient, dstClient := src.Operator(), dst.Operator()
			ctxFotStatus, cancelForStatus := context.WithTimeout(context.Background(), DefaultTimeout)

			defer cancelForStatus()

			dstStatus, err := dstClient.GetStatus(ctxFotStatus, &empty.Empty{})
			if err != nil {
				errors.Append(fmt.Errorf("failed to get status from server %d, error=%w", dstIndex, err))

				return
			}

			dstAddr := strings.Split(dstStatus.P2PAddr, ",")[0]
			ctxForConnecting, cancelForConnecting := context.WithTimeout(context.Background(), DefaultTimeout)

			defer cancelForConnecting()

			_, err = srcClient.PeersAdd(ctxForConnecting, &proto.PeersAddRequest{
				Id: dstAddr,
			})

			if err != nil {
				errors.Append(fmt.Errorf("failed to connect from %d to %d, error=%w", srcIndex, dstIndex, err))
			}
		}()
	}

	wg.Wait()

	for _, err := range errors.Errors() {
		t.Error(err)
	}

	if len(errors.Errors()) > 0 {
		t.Fail()
	}
}

// WaitUntilPeerConnects waits until server connects to required number of peers
// otherwise returns timeout
func WaitUntilPeerConnects(ctx context.Context, srv *TestServer, requiredNum int) (*proto.PeersListResponse, error) {
	clt := srv.Operator()
	res, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		subCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, _ := clt.PeersList(subCtx, &empty.Empty{})
		if res != nil && len(res.Peers) >= requiredNum {
			return res, false
		}

		return nil, true
	})

	if err != nil {
		return nil, err
	}

	peersListResponse, ok := res.(*proto.PeersListResponse)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	return peersListResponse, nil
}

// WaitUntilTxPoolFilled waits until node has required number of transactions in txpool,
// otherwise returns timeout
func WaitUntilTxPoolFilled(
	ctx context.Context,
	srv *TestServer,
	requiredNum uint64,
) (*txpoolProto.TxnPoolStatusResp, error) {
	clt := srv.TxnPoolOperator()
	res, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		subCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, _ := clt.Status(subCtx, &empty.Empty{})
		if res != nil && res.Length >= requiredNum {
			return res, false
		}

		return nil, true
	})

	if err != nil {
		return nil, err
	}

	status, ok := res.(*txpoolProto.TxnPoolStatusResp)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	return status, nil
}

// WaitUntilBlockMined waits until server mined block with bigger height than given height
// otherwise returns timeout
func WaitUntilBlockMined(ctx context.Context, srv *TestServer, desiredHeight uint64) (uint64, error) {
	clt := srv.JSONRPC().Eth()
	res, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		height, err := clt.BlockNumber()
		if err == nil && height >= desiredHeight {
			return height, false
		}

		return nil, true
	})

	if err != nil {
		return 0, err
	}

	blockNum, ok := res.(uint64)
	if !ok {
		return 0, errors.New("invalid type assert")
	}

	return blockNum, nil
}

// MethodSig returns the signature of a non-parametrized function
func MethodSig(name string) []byte {
	return MethodSigWithParams(fmt.Sprintf("%s()", name))
}

// MethodSigWithParams returns the signature of a function
func MethodSigWithParams(nameWithParams string) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(nameWithParams))
	b := h.Sum(nil)

	return b[:4]
}

// tempDir returns directory path in tmp with random directory name
func tempDir() (string, error) {
	return os.MkdirTemp("/tmp", "polygon-edge-e2e-")
}

func ToLocalIPv4LibP2pAddr(port int, nodeID string) string {
	return fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", port, nodeID)
}

// ReservedPort keeps available port until use
type ReservedPort struct {
	port     int
	listener net.Listener
	isClosed bool
}

func (p *ReservedPort) Port() int {
	return p.port
}

func (p *ReservedPort) IsClosed() bool {
	return p.isClosed
}

func (p *ReservedPort) Close() error {
	if p.isClosed {
		return nil
	}

	err := p.listener.Close()
	p.isClosed = true

	return err
}

func FindAvailablePort(from, to int) *ReservedPort {
	for port := from; port < to; port++ {
		addr := fmt.Sprintf("localhost:%d", port)
		if l, err := net.Listen("tcp", addr); err == nil {
			return &ReservedPort{port: port, listener: l}
		}
	}

	return nil
}

func FindAvailablePorts(n, from, to int) ([]ReservedPort, error) {
	ports := make([]ReservedPort, 0, n)
	nextFrom := from

	for i := 0; i < n; i++ {
		newPort := FindAvailablePort(nextFrom, to)
		if newPort == nil {
			// Close current reserved ports
			for _, p := range ports {
				p.Close()
			}

			return nil, errors.New("couldn't reserve required number of ports")
		}

		ports = append(ports, *newPort)
		nextFrom = newPort.Port() + 1
	}

	return ports, nil
}

func NewTestServers(t *testing.T, num int, conf func(*TestServerConfig)) []*TestServer {
	t.Helper()

	srvs := make([]*TestServer, 0, num)

	t.Cleanup(func() {
		for _, srv := range srvs {
			srv.Stop()
			if err := os.RemoveAll(srv.Config.RootDir); err != nil {
				t.Log(err)
			}
		}
	})

	logsDir, err := initLogsDir(t)
	if err != nil {
		t.Fatal(err)
	}

	// It is safe to use a dummy MultiAddr here, since this init method
	// is called for the Dev consensus mode, and IBFT servers are initialized with NewIBFTServersManager.
	// This method needs to be standardized in the future
	bootnodes := []string{tests.GenerateTestMultiAddr(t).String()}

	for i := 0; i < num; i++ {
		dataDir, err := tempDir()
		if err != nil {
			t.Fatal(err)
		}

		srv := NewTestServer(t, dataDir, func(c *TestServerConfig) {
			c.SetLogsDir(logsDir)
			c.SetSaveLogs(true)
			conf(c)
		})
		srv.Config.SetBootnodes(bootnodes)

		srvs = append(srvs, srv)
	}

	errors := NewAtomicErrors(len(srvs))

	var wg sync.WaitGroup

	for i, srv := range srvs {
		i, srv := i, srv

		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := srv.GenerateGenesis(); err != nil {
				errors.Append(fmt.Errorf("server %d failed genesis command, error=%w", i, err))

				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
			defer cancel()

			if err := srv.Start(ctx); err != nil {
				errors.Append(fmt.Errorf("server %d failed to start, error=%w", i, err))
			}
		}()
	}

	wg.Wait()

	for _, err := range errors.Errors() {
		t.Error(err)
	}

	if len(errors.Errors()) > 0 {
		t.Fail()
	}

	return srvs
}

func WaitForServersToSeal(servers []*TestServer, desiredHeight uint64) []error {
	waitErrors := make([]error, 0)

	var waitErrorsLock sync.Mutex

	appendWaitErr := func(waitErr error) {
		waitErrorsLock.Lock()
		defer waitErrorsLock.Unlock()

		waitErrors = append(waitErrors, waitErr)
	}

	var wg sync.WaitGroup
	for i := 0; i < len(servers); i++ {
		wg.Add(1)

		go func(indx int) {
			waitCtx, waitCancelFn := context.WithTimeout(context.Background(), time.Minute)
			defer func() {
				waitCancelFn()
				wg.Done()
			}()

			_, waitErr := WaitUntilBlockMined(waitCtx, servers[indx], desiredHeight)
			if waitErr != nil {
				appendWaitErr(fmt.Errorf("unable to wait for block, %w", waitErr))
			}
		}(i)
	}
	wg.Wait()

	return waitErrors
}
