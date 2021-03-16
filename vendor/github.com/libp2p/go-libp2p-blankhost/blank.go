package blankhost

import (
	"context"
	"io"

	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/libp2p/go-eventbus"

	logging "github.com/ipfs/go-log"

	ma "github.com/multiformats/go-multiaddr"
	mstream "github.com/multiformats/go-multistream"
)

var log = logging.Logger("blankhost")

// BlankHost is the thinnest implementation of the host.Host interface
type BlankHost struct {
	n        network.Network
	mux      *mstream.MultistreamMuxer
	cmgr     connmgr.ConnManager
	eventbus event.Bus
	emitters struct {
		evtLocalProtocolsUpdated event.Emitter
	}
}

func NewBlankHost(n network.Network) *BlankHost {
	bh := &BlankHost{
		n:        n,
		cmgr:     &connmgr.NullConnMgr{},
		mux:      mstream.NewMultistreamMuxer(),
		eventbus: eventbus.NewBus(),
	}

	var err error
	if bh.emitters.evtLocalProtocolsUpdated, err = bh.eventbus.Emitter(&event.EvtLocalProtocolsUpdated{}); err != nil {
		return nil
	}

	n.SetStreamHandler(bh.newStreamHandler)
	return bh
}

var _ host.Host = (*BlankHost)(nil)

func (bh *BlankHost) Addrs() []ma.Multiaddr {
	addrs, err := bh.n.InterfaceListenAddresses()
	if err != nil {
		log.Debug("error retrieving network interface addrs: ", err)
		return nil
	}

	return addrs
}

func (bh *BlankHost) Close() error {
	return bh.n.Close()
}

func (bh *BlankHost) Connect(ctx context.Context, ai peer.AddrInfo) error {
	// absorb addresses into peerstore
	bh.Peerstore().AddAddrs(ai.ID, ai.Addrs, peerstore.TempAddrTTL)

	cs := bh.n.ConnsToPeer(ai.ID)
	if len(cs) > 0 {
		return nil
	}

	_, err := bh.Network().DialPeer(ctx, ai.ID)
	return err
}

func (bh *BlankHost) Peerstore() peerstore.Peerstore {
	return bh.n.Peerstore()
}

func (bh *BlankHost) ID() peer.ID {
	return bh.n.LocalPeer()
}

func (bh *BlankHost) NewStream(ctx context.Context, p peer.ID, protos ...protocol.ID) (network.Stream, error) {
	s, err := bh.n.NewStream(ctx, p)
	if err != nil {
		return nil, err
	}

	var protoStrs []string
	for _, pid := range protos {
		protoStrs = append(protoStrs, string(pid))
	}

	selected, err := mstream.SelectOneOf(protoStrs, s)
	if err != nil {
		s.Close()
		return nil, err
	}

	selpid := protocol.ID(selected)
	s.SetProtocol(selpid)
	bh.Peerstore().AddProtocols(p, selected)

	return s, nil
}

func (bh *BlankHost) RemoveStreamHandler(pid protocol.ID) {
	bh.Mux().RemoveHandler(string(pid))
	bh.emitters.evtLocalProtocolsUpdated.Emit(event.EvtLocalProtocolsUpdated{
		Removed: []protocol.ID{pid},
	})
}

func (bh *BlankHost) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
	bh.Mux().AddHandler(string(pid), func(p string, rwc io.ReadWriteCloser) error {
		is := rwc.(network.Stream)
		is.SetProtocol(protocol.ID(p))
		handler(is)
		return nil
	})
	bh.emitters.evtLocalProtocolsUpdated.Emit(event.EvtLocalProtocolsUpdated{
		Added: []protocol.ID{pid},
	})
}

func (bh *BlankHost) SetStreamHandlerMatch(pid protocol.ID, m func(string) bool, handler network.StreamHandler) {
	bh.Mux().AddHandlerWithFunc(string(pid), m, func(p string, rwc io.ReadWriteCloser) error {
		is := rwc.(network.Stream)
		is.SetProtocol(protocol.ID(p))
		handler(is)
		return nil
	})
	bh.emitters.evtLocalProtocolsUpdated.Emit(event.EvtLocalProtocolsUpdated{
		Added: []protocol.ID{pid},
	})
}

// newStreamHandler is the remote-opened stream handler for network.Network
func (bh *BlankHost) newStreamHandler(s network.Stream) {
	protoID, handle, err := bh.Mux().Negotiate(s)
	if err != nil {
		log.Warning("protocol mux failed: %s", err)
		s.Close()
		return
	}

	s.SetProtocol(protocol.ID(protoID))

	go handle(protoID, s)
}

// TODO: i'm not sure this really needs to be here
func (bh *BlankHost) Mux() protocol.Switch {
	return bh.mux
}

// TODO: also not sure this fits... Might be better ways around this (leaky abstractions)
func (bh *BlankHost) Network() network.Network {
	return bh.n
}

func (bh *BlankHost) ConnManager() connmgr.ConnManager {
	return bh.cmgr
}

func (bh *BlankHost) EventBus() event.Bus {
	return bh.eventbus
}
