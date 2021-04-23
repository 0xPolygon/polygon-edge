package framework

import (
	"context"
	"crypto/ecdsa"
	"testing"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/assert"
)

func GenerateKeyAndAddr(t *testing.T) (*ecdsa.PrivateKey, types.Address) {
	t.Helper()
	key, err := crypto.GenerateKey()
	assert.NoError(t, err)
	addr := crypto.PubKeyToAddress(&key.PublicKey)
	return key, addr
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

	errCh := make(chan error)
	for i := 0; i < len(srvs); i += 2 {
		go func(src, dst *TestServer, errCh chan<- error) {
			srcClient, dstClient := src.Operator(), dst.Operator()
			dstStatus, err := dstClient.GetStatus(context.Background(), &empty.Empty{})
			if err != nil {
				errCh <- err
				return
			}
			dstAddr := dstStatus.P2PAddr

			_, err = srcClient.PeersAdd(context.Background(), &proto.PeersAddRequest{
				Id: dstAddr,
			})
			if err != nil {
				errCh <- err
			}
			errCh <- nil
		}(srvs[i], srvs[i+1], errCh)
	}

	errCount := 0
	for i := 0; i < len(srvs)/2; i++ {
		err := <-errCh
		if err != nil {
			errCount++
			t.Errorf("failed to connect from %d to %d, err=%+v ", 2*i, 2*i+1, err)
		}
	}
	if errCount > 0 {
		t.Fail()
	}
}
