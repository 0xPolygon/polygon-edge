package tests

import (
	"context"
	"crypto/ecdsa"
	"errors"
	txpoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
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
