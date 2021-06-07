package framework

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"strings"
	"testing"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/sha3"
)

func EthToWei(ethValue int64) *big.Int {
	return new(big.Int).Mul(
		big.NewInt(ethValue),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

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
			dstAddr := strings.Split(dstStatus.P2PAddr, "\n")[0]

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

// MethodSig returns the signature of a non-parametrized function
func MethodSig(name string) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(name + "()"))
	b := h.Sum(nil)
	return b[:4]
}

// TempDir returns direcotry path in tmp with random directory name
func TempDir() (string, error) {
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
