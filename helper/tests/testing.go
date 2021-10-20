package tests

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
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
