package framework

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io/ioutil"
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
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/server/proto"
	txpoolProto "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
	"golang.org/x/crypto/sha3"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

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
		web3.Address(address),
		web3.Latest,
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

	toAddress := web3.Address(staking.AddrStakingContract)
	selector := validatorsMethod.ID()
	response, err := rpcClient.Eth().Call(
		&web3.CallMsg{
			From:     web3.Address(from),
			To:       &toAddress,
			Data:     selector,
			GasPrice: 100000000,
			Value:    big.NewInt(0),
		},
		web3.Latest,
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
		GasPrice: big.NewInt(10000),
		Gas:      1000000,
		Value:    amount,
		Input:    MethodSig("stake"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
) (*web3.Receipt, error) {
	// Stake Balance
	txn := &PreparedTransaction{
		From:     from,
		To:       &staking.AddrStakingContract,
		GasPrice: big.NewInt(DefaultGasPrice),
		Gas:      DefaultGasLimit,
		Value:    big.NewInt(0),
		Input:    MethodSig("unstake"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	toAddress := web3.Address(staking.AddrStakingContract)
	selector := stakedAmountMethod.ID()
	response, err := rpcClient.Eth().Call(
		&web3.CallMsg{
			From:     web3.Address(from),
			To:       &toAddress,
			Data:     selector,
			GasPrice: 100000000,
			Value:    big.NewInt(0),
		},
		web3.Latest,
	)

	if err != nil {
		return nil, fmt.Errorf("unable to call Staking contract method stakedAmount, %w", err)
	}

	bigResponse, decodeErr := types.ParseUint256orHex(&response)
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

	errCh := make(chan error, len(srvs)/2)

	for i := 0; i < len(srvs); i += 2 {
		src, dst := srvs[i], srvs[i+1]

		go func() {
			srcClient, dstClient := src.Operator(), dst.Operator()
			dstStatus, err := dstClient.GetStatus(context.Background(), &empty.Empty{})

			if err != nil {
				errCh <- err

				return
			}

			dstAddr := strings.Split(dstStatus.P2PAddr, ",")[0]
			_, err = srcClient.PeersAdd(context.Background(), &proto.PeersAddRequest{
				Id: dstAddr,
			})

			errCh <- err
		}()
	}

	errCount := 0

	for i := 0; i < len(srvs)/2; i++ {
		if err := <-errCh; err != nil {
			errCount++

			t.Errorf("failed to connect from %d to %d, error=%+v ", 2*i, 2*i+1, err)
		}
	}

	if errCount > 0 {
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
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(name + "()"))
	b := h.Sum(nil)

	return b[:4]
}

// tempDir returns directory path in tmp with random directory name
func tempDir() (string, error) {
	return ioutil.TempDir("/tmp", "polygon-edge-e2e-")
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

	// It is safe to use a dummy MultiAddr here, since this init method
	// is called for the Dev consensus mode, and IBFT servers are initialized with NewIBFTServersManager.
	// This method needs to be standardized in the future
	bootnodes := []string{tests.GenerateTestMultiAddr(t).String()}

	for i := 0; i < num; i++ {
		dataDir, err := tempDir()
		if err != nil {
			t.Fatal(err)
		}

		srv := NewTestServer(t, dataDir, conf)
		srv.Config.SetBootnodes(bootnodes)

		if genesisErr := srv.GenerateGenesis(); genesisErr != nil {
			t.Fatal(genesisErr)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Start(ctx); err != nil {
			t.Fatal(err)
		}

		srvs = append(srvs, srv)
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
