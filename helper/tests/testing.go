package tests

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	libp2pCrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"net"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

var (
	ErrTimeout = errors.New("timeout")
)

func GenerateKeyAndAddr(t *testing.T) (*ecdsa.PrivateKey, types.Address) {
	t.Helper()

	key, err := crypto.GenerateKey()

	assert.NoError(t, err)

	addr := crypto.PubKeyToAddress(&key.PublicKey)

	return key, addr
}

func GenerateTestMultiAddr(t *testing.T) multiaddr.Multiaddr {
	t.Helper()

	priv, _, err := libp2pCrypto.GenerateKeyPair(libp2pCrypto.Secp256k1, 256)
	if err != nil {
		t.Fatalf("Unable to generate key pair, %v", err)
	}

	nodeID, err := peer.IDFromPrivateKey(priv)
	assert.NoError(t, err)

	port, portErr := GetFreePort()
	if portErr != nil {
		t.Fatalf("Unable to fetch free port, %v", portErr)
	}

	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", port, nodeID))
	assert.NoError(t, err)

	return addr
}

func RetryUntilTimeout(ctx context.Context, f func() (interface{}, bool)) (interface{}, error) {
	type result struct {
		data interface{}
		err  error
	}

	resCh := make(chan result, 1)

	go func() {
		defer close(resCh)

		for {
			select {
			case <-ctx.Done():
				resCh <- result{nil, ErrTimeout}

				return
			default:
				res, retry := f()
				if !retry {
					resCh <- result{res, nil}

					return
				}
			}
			time.Sleep(time.Second)
		}
	}()

	res := <-resCh

	return res.data, res.err
}

// WaitUntilTxPoolEmpty waits until node has 0 transactions in txpool,
// otherwise returns timeout
func WaitUntilTxPoolEmpty(ctx context.Context, client txpoolOp.TxnPoolOperatorClient) (*txpoolOp.TxnPoolStatusResp,
	error) {
	res, err := RetryUntilTimeout(ctx, func() (interface{}, bool) {
		subCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, _ := client.Status(subCtx, &empty.Empty{})
		if res != nil && res.Length == 0 {
			return res, false
		}

		return nil, true
	})

	if err != nil {
		return nil, err
	}

	return res.(*txpoolOp.TxnPoolStatusResp), nil
}

// WaitForReceipt waits transaction receipt
func WaitForReceipt(ctx context.Context, client *jsonrpc.Eth, hash web3.Hash) (*web3.Receipt, error) {
	type result struct {
		receipt *web3.Receipt
		err     error
	}

	res, err := RetryUntilTimeout(ctx, func() (interface{}, bool) {
		receipt, err := client.GetTransactionReceipt(hash)
		if err != nil && err.Error() != "not found" {
			return result{receipt, err}, false
		}
		if receipt != nil {
			return result{receipt, nil}, false
		}

		return nil, true
	})

	if err != nil {
		return nil, err
	}

	data, ok := res.(result)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	return data.receipt, data.err
}

// GetFreePort asks the kernel for a free open port that is ready to use
func GetFreePort() (port int, err error) {
	var addr *net.TCPAddr

	if addr, err = net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		var l *net.TCPListener

		if l, err = net.ListenTCP("tcp", addr); err == nil {
			defer func(l *net.TCPListener) {
				_ = l.Close()
			}(l)

			return l.Addr().(*net.TCPAddr).Port, nil
		}
	}

	return
}
