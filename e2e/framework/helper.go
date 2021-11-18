package framework

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/server/proto"
	txpoolProto "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
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
func GetAccountBalance(
	address types.Address,
	rpcClient *jsonrpc.Client,
	t *testing.T,
) *big.Int {
	accountBalance, err := rpcClient.Eth().GetBalance(
		web3.Address(address),
		web3.Latest,
	)

	assert.NoError(t, err)

	return accountBalance
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
	return res.(*proto.PeersListResponse), nil
}

// WaitUntilTxPoolFilled waits until node has required number of transactions in txpool,
// otherwise returns timeout
func WaitUntilTxPoolFilled(ctx context.Context, srv *TestServer, requiredNum uint64) (*txpoolProto.TxnPoolStatusResp, error) {
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
	return res.(*txpoolProto.TxnPoolStatusResp), nil
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
	return res.(uint64), nil
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
	return ioutil.TempDir("/tmp", "polygon-sdk-e2e-")
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

	for i := 0; i < num; i++ {
		dataDir, err := tempDir()
		if err != nil {
			t.Fatal(err)
		}
		srv := NewTestServer(t, dataDir, conf)
		if err := srv.GenerateGenesis(); err != nil {
			t.Fatal(err)
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
