package network

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/protocol"
)

type dummyHandler struct{}

func (d *dummyHandler) Init() error {
	return nil
}

func (d *dummyHandler) Close() error {
	return nil
}

func testServers(config *Config) (*Server, *Server) {
	logger := log.New(ioutil.Discard, "", log.LstdFlags)

	prv0, _ := crypto.GenerateKey()
	prv1, _ := crypto.GenerateKey()

	rand.Seed(int64(time.Now().Nanosecond()))
	port := rand.Intn(9000-5000) + 5000 // Random port between 5000 and 9000

	c0 := *config
	c0.BindPort = port
	s0 := NewServer("test", prv0, &c0, logger)

	c1 := *config
	c1.BindPort = port + 1
	s1 := NewServer("test", prv1, &c1, logger)

	return s0, s1
}

type handler struct {
}

func (h *handler) Info() string {
	return "info"
}

type sampleProtocol struct {
}

func (s *sampleProtocol) Protocol() protocol.Protocol {
	return protocol.Protocol{Name: "test", Version: 1, Length: 7}
}

func (s *sampleProtocol) Add(conn net.Conn, peerID string) (protocol.Handler, error) {
	h := &handler{}
	return h, nil
}

func TestProtocolConnection(t *testing.T) {
	s0, s1 := testServers(DefaultConfig())

	p0 := &sampleProtocol{}
	p1 := &sampleProtocol{}

	s0.RegisterProtocol(p0)
	s1.RegisterProtocol(p1)

	if err := s0.Schedule(); err != nil {
		t.Fatal(err)
	}
	if err := s1.Schedule(); err != nil {
		t.Fatal(err)
	}

	fmt.Println(s0)
	fmt.Println(s1)

	if err := s0.DialSync(s1.Enode.String()); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	fmt.Println(s0.peers)
	fmt.Println(s1.peers)
}

/*
// Include once we decouple matchprotocol again
func TestMatchProtocols(t *testing.T) {
	convert := func(p protocol.Protocol, offset int) protocolMatch {
		return protocolMatch{Name: p.Name, Version: p.Version, Offset: offset}
	}

	var cases = []struct {
		Name     string
		Local    []protocol.Protocol
		Remote   rlpx.Capabilities
		Expected []protocolMatch
	}{
		{
			Name: "Only remote",
			Remote: []*rlpx.Cap{
				toCap(protocol.ETH63),
			},
		},
		{
			Name: "Only local",
			Local: []protocol.Protocol{
				protocol.ETH63,
			},
		},
		{
			Name: "Match",
			Local: []protocol.Protocol{
				protocol.ETH63,
			},
			Remote: []*rlpx.Cap{
				toCap(protocol.ETH63),
			},
			Expected: []protocolMatch{
				convert(protocol.ETH63, 5),
			},
		},
	}

	// p := &Peer{}
	callback := func(session rlpx.Conn, peer *Peer) protocol.Handler {
		return nil
	}

	for _, cc := range cases {
		t.Run(cc.Name, func(t *testing.T) {
			prv, _ := crypto.GenerateKey()
			s := Server{Protocols: []*protocolStub{}, key: prv}

			for _, p := range cc.Local {
				s.RegisterProtocol(p, callback)
			}
			s.buildInfo()

			res := s.matchProtocols2(cc.Remote)

			if i := len(res); i == len(cc.Expected) && i != 0 {
				if !reflect.DeepEqual(res, cc.Expected) {
					t.Fatal("bad")
				}
			}
		})
	}
}
*/
