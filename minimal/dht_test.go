package minimal

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	swarmt "github.com/libp2p/go-libp2p-swarm/testing"
	bhost "github.com/libp2p/go-libp2p/p2p/host/basic"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
)

type blankValidator struct{}

func (blankValidator) Validate(_ string, _ []byte) error        { return nil }
func (blankValidator) Select(_ string, _ [][]byte) (int, error) { return 0, nil }

var testPrefix = dht.ProtocolPrefix("/test")

func createTestDHT(ctx context.Context, t *testing.T, client bool, options ...dht.Option) *dht.IpfsDHT {
	baseOpts := []dht.Option{
		testPrefix,
		dht.NamespacedValidator("v", blankValidator{}),
		dht.DisableAutoRefresh(),
	}

	if client {
		baseOpts = append(baseOpts, dht.Mode(dht.ModeClient))
	} else {
		baseOpts = append(baseOpts, dht.Mode(dht.ModeServer))
	}

	d, err := dht.New(
		ctx,
		bhost.New(swarmt.GenSwarm(t, ctx, swarmt.OptDisableReuseport)),
		append(baseOpts, options...)...,
	)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func createTestDHTs(t *testing.T, ctx context.Context, n int, options ...dht.Option) []*dht.IpfsDHT {
	addrs := make([]ma.Multiaddr, n)
	dhts := make([]*dht.IpfsDHT, n)
	peers := make([]peer.ID, n)

	sanityAddrsMap := make(map[string]struct{})
	sanityPeersMap := make(map[string]struct{})

	for i := 0; i < n; i++ {
		dhts[i] = createTestDHT(ctx, t, false, options...)
		peers[i] = dhts[i].PeerID()
		addrs[i] = dhts[i].Host().Addrs()[0]

		if _, lol := sanityAddrsMap[addrs[i].String()]; lol {
			t.Fatal("While setting up DHTs address got duplicated.")
		} else {
			sanityAddrsMap[addrs[i].String()] = struct{}{}
		}
		if _, lol := sanityPeersMap[peers[i].String()]; lol {
			t.Fatal("While setting up DHTs peerid got duplicated.")
		} else {
			sanityPeersMap[peers[i].String()] = struct{}{}
		}
	}

	return dhts
}

func connectNoSync(t *testing.T, ctx context.Context, a, b *dht.IpfsDHT) {
	t.Helper()

	idB := b.Host().ID()
	addrB := b.Host().Peerstore().Addrs(idB)
	if len(addrB) == 0 {
		t.Fatal("peers setup incorrectly: no local address")
	}

	a.Host().Peerstore().AddAddrs(idB, addrB, peerstore.TempAddrTTL)
	pi := peer.AddrInfo{ID: idB}
	if err := a.Host().Connect(ctx, pi); err != nil {
		t.Fatal(err)
	}
}

func wait(t *testing.T, ctx context.Context, a, b *dht.IpfsDHT) {
	t.Helper()

	for a.RoutingTable().Find(b.Host().ID()) == "" {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond * 5):
		}
	}
}

func connect(t *testing.T, ctx context.Context, a, b *dht.IpfsDHT) {
	t.Helper()
	connectNoSync(t, ctx, a, b)
	wait(t, ctx, a, b)
	wait(t, ctx, b, a)
}

func createTestServer(dht *dht.IpfsDHT) *Server {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "test",
		Level: hclog.LevelFromString("debug"),
	})

	return &Server{
		logger:          logger,
		dht:             dht,
		host:            dht.Host(),
		peerAddedCh:     make(chan struct{}),
		peerRemovedCh:   make(chan struct{}),
		peerJoined:      make(map[string]bool),
		peerJoinedMutex: new(sync.RWMutex),
	}
}

func TestPeerAdded(t *testing.T) {
	type Args struct {
		pids []peer.ID
	}
	type Expected struct {
		count int
	}

	tests := []struct {
		name     string
		args     Args
		expected Expected
	}{
		{
			name: "should send peerAddedCh a signal",
			args: Args{
				pids: []peer.ID{
					peer.NewPeerRecord().PeerID,
				},
			},
			expected: Expected{
				count: 1,
			},
		},
		{
			name: "should send peerAddedCh a signal even if two peers are added in a moment",
			args: Args{
				pids: []peer.ID{
					peer.NewPeerRecord().PeerID,
					peer.NewPeerRecord().PeerID,
				},
			},
			expected: Expected{
				count: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dht := createTestDHT(context.Background(), t, true)
			s := createTestServer(dht)
			defer func() {
				dht.Close()
				dht.Host().Close()
			}()

			resCh := make(chan int)
			go func(resCh chan int) {
				cnt := 0
				for range s.peerAddedCh {
					cnt++
					time.Sleep(time.Second)
				}
				resCh <- cnt
			}(resCh)

			time.Sleep(time.Second)
			for _, p := range tt.args.pids {
				s.peerAdded(p)
			}
			time.Sleep(time.Second)
			close(s.peerAddedCh)

			res := <-resCh
			assert.Equal(t, tt.expected.count, res)
		})
	}
}

func TestPeerRemoved(t *testing.T) {
	type Args struct {
		pids []peer.ID
	}
	type Expected struct {
		count int
	}

	tests := []struct {
		name     string
		args     Args
		expected Expected
	}{
		{
			name: "should send peerRemovedCh a signal",
			args: Args{
				pids: []peer.ID{
					peer.NewPeerRecord().PeerID,
				},
			},
			expected: Expected{
				count: 1,
			},
		},
		{
			name: "should send peerRemovedCh a signal even if two peers are removed in a moment",
			args: Args{
				pids: []peer.ID{
					peer.NewPeerRecord().PeerID,
					peer.NewPeerRecord().PeerID,
				},
			},
			expected: Expected{
				count: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dht := createTestDHT(context.Background(), t, true)
			s := createTestServer(dht)
			defer func() {
				dht.Close()
				dht.Host().Close()
			}()

			resCh := make(chan int)
			go func(resCh chan int) {
				cnt := 0
				for range s.peerRemovedCh {
					cnt++
					time.Sleep(time.Second)
				}
				resCh <- cnt
			}(resCh)

			time.Sleep(time.Second)
			for _, p := range tt.args.pids {
				s.peerRemoved(p)
			}
			time.Sleep(time.Second)
			close(s.peerRemovedCh)

			res := <-resCh
			assert.Equal(t, tt.expected.count, res)
		})
	}
}

func TestGetBestPeerAddr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nDHTs := 10
	dhts := createTestDHTs(t, context.Background(), nDHTs)
	defer func() {
		for i := 0; i < nDHTs; i++ {
			dhts[i].Close()
			dhts[i].Host().Close()
		}
	}()
	for i := 0; i < nDHTs; i++ {
		connect(t, ctx, dhts[i], dhts[(i+1)%len(dhts)])
	}
	s := createTestServer(dhts[0])

	closestPeersChan, err := dhts[0].GetClosestPeers(context.Background(), string(s.host.ID()))
	closestPeer := <-closestPeersChan

	expectedPeerInfo := s.host.Peerstore().PeerInfo(closestPeer)
	expectedPeerAddr := AddrInfoToString(&expectedPeerInfo)

	addr0, err := s.getBestPeerAddr()
	assert.NoError(t, err)
	assert.NotEmpty(t, addr0)
	assert.Equal(t, expectedPeerAddr, *addr0)
}
