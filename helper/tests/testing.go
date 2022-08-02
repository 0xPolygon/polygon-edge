package tests

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/golang/protobuf/ptypes/any"
	libp2pCrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/umbracle/ethgo"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/ethgo/jsonrpc"
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
func WaitUntilTxPoolEmpty(
	ctx context.Context,
	client txpoolOp.TxnPoolOperatorClient,
) (
	*txpoolOp.TxnPoolStatusResp,
	error,
) {
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

	status, ok := res.(*txpoolOp.TxnPoolStatusResp)
	if !ok {
		return nil, errors.New("invalid type assertion to txpool status response")
	}

	return status, nil
}

func WaitForNonce(
	ctx context.Context,
	ethClient *jsonrpc.Eth,
	addr ethgo.Address,
	expectedNonce uint64,
) (
	interface{},
	error,
) {
	type result struct {
		nonce uint64
		err   error
	}

	resObj, err := RetryUntilTimeout(ctx, func() (interface{}, bool) {
		nonce, err := ethClient.GetNonce(addr, ethgo.Latest)
		if err != nil {
			//	error -> stop retrying
			return result{nonce, err}, false
		}

		if nonce >= expectedNonce {
			//	match -> return result
			return result{nonce, nil}, false
		}

		//	continue retrying
		return nil, true
	})

	if err != nil {
		return nil, err
	}

	res, ok := resObj.(result)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	return res.nonce, res.err
}

// WaitForReceipt waits transaction receipt
func WaitForReceipt(ctx context.Context, client *jsonrpc.Eth, hash ethgo.Hash) (*ethgo.Receipt, error) {
	type result struct {
		receipt *ethgo.Receipt
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

			netAddr, ok := l.Addr().(*net.TCPAddr)
			if !ok {
				return 0, errors.New("invalid type assert to TCPAddr")
			}

			return netAddr.Port, nil
		}
	}

	return
}

type GenerateTxReqParams struct {
	Nonce         uint64
	ReferenceAddr types.Address
	ReferenceKey  *ecdsa.PrivateKey
	ToAddress     types.Address
	GasPrice      *big.Int
	Value         *big.Int
	Input         []byte
}

func generateTx(params GenerateTxReqParams) (*types.Transaction, error) {
	signer := crypto.NewEIP155Signer(100)

	signedTx, signErr := signer.SignTx(&types.Transaction{
		Nonce:    params.Nonce,
		From:     params.ReferenceAddr,
		To:       &params.ToAddress,
		GasPrice: params.GasPrice,
		Gas:      1000000,
		Value:    params.Value,
		Input:    params.Input,
		V:        big.NewInt(27), // it is necessary to encode in rlp
	}, params.ReferenceKey)

	if signErr != nil {
		return nil, fmt.Errorf("unable to sign transaction, %w", signErr)
	}

	return signedTx, nil
}

func GenerateAddTxnReq(params GenerateTxReqParams) (*txpoolOp.AddTxnReq, error) {
	txn, err := generateTx(params)
	if err != nil {
		return nil, err
	}

	msg := &txpoolOp.AddTxnReq{
		Raw: &any.Any{
			Value: txn.MarshalRLP(),
		},
		From: types.ZeroAddress.String(),
	}

	return msg, nil
}
