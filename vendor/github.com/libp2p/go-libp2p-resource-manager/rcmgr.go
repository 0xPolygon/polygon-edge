package rcmgr

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("rcmgr")

type resourceManager struct {
	limits Limiter

	trace   *trace
	metrics *metrics

	system    *systemScope
	transient *transientScope

	cancelCtx context.Context
	cancel    func()
	wg        sync.WaitGroup

	mx    sync.Mutex
	svc   map[string]*serviceScope
	proto map[protocol.ID]*protocolScope
	peer  map[peer.ID]*peerScope

	stickyProto map[protocol.ID]struct{}
	stickyPeer  map[peer.ID]struct{}

	connId, streamId int64
}

var _ network.ResourceManager = (*resourceManager)(nil)

type systemScope struct {
	*resourceScope
}

var _ network.ResourceScope = (*systemScope)(nil)

type transientScope struct {
	*resourceScope

	system *systemScope
}

var _ network.ResourceScope = (*transientScope)(nil)

type serviceScope struct {
	*resourceScope

	name  string
	rcmgr *resourceManager

	peers map[peer.ID]*resourceScope
}

var _ network.ServiceScope = (*serviceScope)(nil)

type protocolScope struct {
	*resourceScope

	proto protocol.ID
	rcmgr *resourceManager

	peers map[peer.ID]*resourceScope
}

var _ network.ProtocolScope = (*protocolScope)(nil)

type peerScope struct {
	*resourceScope

	peer  peer.ID
	rcmgr *resourceManager
}

var _ network.PeerScope = (*peerScope)(nil)

type connectionScope struct {
	*resourceScope

	dir   network.Direction
	usefd bool
	rcmgr *resourceManager
	peer  *peerScope
}

var _ network.ConnScope = (*connectionScope)(nil)
var _ network.ConnManagementScope = (*connectionScope)(nil)

type streamScope struct {
	*resourceScope

	dir   network.Direction
	rcmgr *resourceManager
	peer  *peerScope
	svc   *serviceScope
	proto *protocolScope

	peerProtoScope *resourceScope
	peerSvcScope   *resourceScope
}

var _ network.StreamScope = (*streamScope)(nil)
var _ network.StreamManagementScope = (*streamScope)(nil)

type Option func(*resourceManager) error

func NewResourceManager(limits Limiter, opts ...Option) (network.ResourceManager, error) {
	r := &resourceManager{
		limits: limits,
		svc:    make(map[string]*serviceScope),
		proto:  make(map[protocol.ID]*protocolScope),
		peer:   make(map[peer.ID]*peerScope),
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	if err := r.trace.Start(limits); err != nil {
		return nil, err
	}

	r.system = newSystemScope(limits.GetSystemLimits(), r)
	r.system.IncRef()
	r.transient = newTransientScope(limits.GetTransientLimits(), r)
	r.transient.IncRef()

	r.cancelCtx, r.cancel = context.WithCancel(context.Background())

	r.wg.Add(1)
	go r.background()

	return r, nil
}

func (r *resourceManager) ViewSystem(f func(network.ResourceScope) error) error {
	return f(r.system)
}

func (r *resourceManager) ViewTransient(f func(network.ResourceScope) error) error {
	return f(r.transient)
}

func (r *resourceManager) ViewService(srv string, f func(network.ServiceScope) error) error {
	s := r.getServiceScope(srv)
	defer s.DecRef()

	return f(s)
}

func (r *resourceManager) ViewProtocol(proto protocol.ID, f func(network.ProtocolScope) error) error {
	s := r.getProtocolScope(proto)
	defer s.DecRef()

	return f(s)
}

func (r *resourceManager) ViewPeer(p peer.ID, f func(network.PeerScope) error) error {
	s := r.getPeerScope(p)
	defer s.DecRef()

	return f(s)
}

func (r *resourceManager) getServiceScope(svc string) *serviceScope {
	r.mx.Lock()
	defer r.mx.Unlock()

	s, ok := r.svc[svc]
	if !ok {
		s = newServiceScope(svc, r.limits.GetServiceLimits(svc), r)
		r.svc[svc] = s
	}

	s.IncRef()
	return s
}

func (r *resourceManager) getProtocolScope(proto protocol.ID) *protocolScope {
	r.mx.Lock()
	defer r.mx.Unlock()

	s, ok := r.proto[proto]
	if !ok {
		s = newProtocolScope(proto, r.limits.GetProtocolLimits(proto), r)
		r.proto[proto] = s
	}

	s.IncRef()
	return s
}

func (r *resourceManager) setStickyProtocol(proto protocol.ID) {
	r.mx.Lock()
	defer r.mx.Unlock()

	if r.stickyProto == nil {
		r.stickyProto = make(map[protocol.ID]struct{})
	}
	r.stickyProto[proto] = struct{}{}
}

func (r *resourceManager) getPeerScope(p peer.ID) *peerScope {
	r.mx.Lock()
	defer r.mx.Unlock()

	s, ok := r.peer[p]
	if !ok {
		s = newPeerScope(p, r.limits.GetPeerLimits(p), r)
		r.peer[p] = s
	}

	s.IncRef()
	return s
}

func (r *resourceManager) setStickyPeer(p peer.ID) {
	r.mx.Lock()
	defer r.mx.Unlock()

	if r.stickyPeer == nil {
		r.stickyPeer = make(map[peer.ID]struct{})
	}

	r.stickyPeer[p] = struct{}{}
}

func (r *resourceManager) nextConnId() int64 {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.connId++
	return r.connId
}

func (r *resourceManager) nextStreamId() int64 {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.streamId++
	return r.streamId
}

func (r *resourceManager) OpenConnection(dir network.Direction, usefd bool) (network.ConnManagementScope, error) {
	conn := newConnectionScope(dir, usefd, r.limits.GetConnLimits(), r)

	if err := conn.AddConn(dir, usefd); err != nil {
		conn.Done()
		r.metrics.BlockConn(dir, usefd)
		return nil, err
	}

	r.metrics.AllowConn(dir, usefd)
	return conn, nil
}

func (r *resourceManager) OpenStream(p peer.ID, dir network.Direction) (network.StreamManagementScope, error) {
	peer := r.getPeerScope(p)
	stream := newStreamScope(dir, r.limits.GetStreamLimits(p), peer, r)
	peer.DecRef() // we have the reference in edges

	err := stream.AddStream(dir)
	if err != nil {
		stream.Done()
		r.metrics.BlockStream(p, dir)
		return nil, err
	}

	r.metrics.AllowStream(p, dir)
	return stream, nil
}

func (r *resourceManager) Close() error {
	r.cancel()
	r.wg.Wait()
	r.trace.Close()

	return nil
}

func (r *resourceManager) background() {
	defer r.wg.Done()

	// periodically garbage collects unused peer and protocol scopes
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.gc()
		case <-r.cancelCtx.Done():
			return
		}
	}
}

func (r *resourceManager) gc() {
	r.mx.Lock()
	defer r.mx.Unlock()

	for proto, s := range r.proto {
		_, sticky := r.stickyProto[proto]
		if sticky {
			continue
		}
		if s.IsUnused() {
			s.Done()
			delete(r.proto, proto)
		}
	}

	var deadPeers []peer.ID
	for p, s := range r.peer {
		_, sticky := r.stickyPeer[p]
		if sticky {
			continue
		}

		if s.IsUnused() {
			s.Done()
			delete(r.peer, p)
			deadPeers = append(deadPeers, p)
		}
	}

	for _, s := range r.svc {
		s.Lock()
		for _, p := range deadPeers {
			ps, ok := s.peers[p]
			if ok {
				ps.Done()
				delete(s.peers, p)
			}
		}
		s.Unlock()
	}

	for _, s := range r.proto {
		s.Lock()
		for _, p := range deadPeers {
			ps, ok := s.peers[p]
			if ok {
				ps.Done()
				delete(s.peers, p)
			}
		}
		s.Unlock()
	}
}

func newSystemScope(limit Limit, rcmgr *resourceManager) *systemScope {
	return &systemScope{
		resourceScope: newResourceScope(limit, nil, "system", rcmgr.trace, rcmgr.metrics),
	}
}

func newTransientScope(limit Limit, rcmgr *resourceManager) *transientScope {
	return &transientScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{rcmgr.system.resourceScope},
			"transient", rcmgr.trace, rcmgr.metrics),
		system: rcmgr.system,
	}
}

func newServiceScope(name string, limit Limit, rcmgr *resourceManager) *serviceScope {
	return &serviceScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{rcmgr.system.resourceScope},
			fmt.Sprintf("service:%s", name), rcmgr.trace, rcmgr.metrics),
		name:  name,
		rcmgr: rcmgr,
	}
}

func newProtocolScope(proto protocol.ID, limit Limit, rcmgr *resourceManager) *protocolScope {
	return &protocolScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{rcmgr.system.resourceScope},
			fmt.Sprintf("protocol:%s", proto), rcmgr.trace, rcmgr.metrics),
		proto: proto,
		rcmgr: rcmgr,
	}
}

func newPeerScope(p peer.ID, limit Limit, rcmgr *resourceManager) *peerScope {
	return &peerScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{rcmgr.system.resourceScope},
			fmt.Sprintf("peer:%s", p), rcmgr.trace, rcmgr.metrics),
		peer:  p,
		rcmgr: rcmgr,
	}
}

func newConnectionScope(dir network.Direction, usefd bool, limit Limit, rcmgr *resourceManager) *connectionScope {
	return &connectionScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{rcmgr.transient.resourceScope, rcmgr.system.resourceScope},
			fmt.Sprintf("conn-%d", rcmgr.nextConnId()), rcmgr.trace, rcmgr.metrics),
		dir:   dir,
		usefd: usefd,
		rcmgr: rcmgr,
	}
}

func newStreamScope(dir network.Direction, limit Limit, peer *peerScope, rcmgr *resourceManager) *streamScope {
	return &streamScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{peer.resourceScope, rcmgr.transient.resourceScope, rcmgr.system.resourceScope},
			fmt.Sprintf("stream-%d", rcmgr.nextStreamId()), rcmgr.trace, rcmgr.metrics),
		dir:   dir,
		rcmgr: peer.rcmgr,
		peer:  peer,
	}
}

func (s *serviceScope) Name() string {
	return s.name
}

func (s *serviceScope) getPeerScope(p peer.ID) *resourceScope {
	s.Lock()
	defer s.Unlock()

	ps, ok := s.peers[p]
	if ok {
		ps.IncRef()
		return ps
	}

	l := s.rcmgr.limits.GetServicePeerLimits(s.name)

	if s.peers == nil {
		s.peers = make(map[peer.ID]*resourceScope)
	}

	ps = newResourceScope(l, nil, fmt.Sprintf("%s.peer:%s", s.name, p), s.rcmgr.trace, s.rcmgr.metrics)
	s.peers[p] = ps

	ps.IncRef()
	return ps
}

func (s *protocolScope) Protocol() protocol.ID {
	return s.proto
}

func (s *protocolScope) getPeerScope(p peer.ID) *resourceScope {
	s.Lock()
	defer s.Unlock()

	ps, ok := s.peers[p]
	if ok {
		ps.IncRef()
		return ps
	}

	l := s.rcmgr.limits.GetProtocolPeerLimits(s.proto)

	if s.peers == nil {
		s.peers = make(map[peer.ID]*resourceScope)
	}

	ps = newResourceScope(l, nil, fmt.Sprintf("%s.peer:%s", s.name, p), s.rcmgr.trace, s.rcmgr.metrics)
	s.peers[p] = ps

	ps.IncRef()
	return ps
}

func (s *peerScope) Peer() peer.ID {
	return s.peer
}

func (s *connectionScope) PeerScope() network.PeerScope {
	s.Lock()
	defer s.Unlock()

	// avoid nil is not nil footgun; go....
	if s.peer == nil {
		return nil
	}

	return s.peer
}

func (s *connectionScope) SetPeer(p peer.ID) error {
	s.Lock()
	defer s.Unlock()

	if s.peer != nil {
		return fmt.Errorf("connection scope already attached to a peer")
	}
	s.peer = s.rcmgr.getPeerScope(p)

	// juggle resources from transient scope to peer scope
	stat := s.resourceScope.rc.stat()
	if err := s.peer.ReserveForChild(stat); err != nil {
		s.peer.DecRef()
		s.peer = nil
		s.rcmgr.metrics.BlockPeer(p)
		return err
	}

	s.rcmgr.transient.ReleaseForChild(stat)
	s.rcmgr.transient.DecRef() // removed from edges

	// update edges
	edges := []*resourceScope{
		s.peer.resourceScope,
		s.rcmgr.system.resourceScope,
	}
	s.resourceScope.edges = edges

	s.rcmgr.metrics.AllowPeer(p)
	return nil
}

func (s *streamScope) ProtocolScope() network.ProtocolScope {
	s.Lock()
	defer s.Unlock()

	// avoid nil is not nil footgun; go....
	if s.proto == nil {
		return nil
	}

	return s.proto
}

func (s *streamScope) SetProtocol(proto protocol.ID) error {
	s.Lock()
	defer s.Unlock()

	if s.proto != nil {
		return fmt.Errorf("stream scope already attached to a protocol")
	}

	s.proto = s.rcmgr.getProtocolScope(proto)

	// juggle resources from transient scope to protocol scope
	stat := s.resourceScope.rc.stat()
	if err := s.proto.ReserveForChild(stat); err != nil {
		s.proto.DecRef()
		s.proto = nil
		s.rcmgr.metrics.BlockProtocol(proto)
		return err
	}

	s.peerProtoScope = s.proto.getPeerScope(s.peer.peer)
	if err := s.peerProtoScope.ReserveForChild(stat); err != nil {
		s.proto.ReleaseForChild(stat)
		s.proto.DecRef()
		s.proto = nil
		s.peerProtoScope.DecRef()
		s.peerProtoScope = nil
		s.rcmgr.metrics.BlockProtocolPeer(proto, s.peer.peer)
		return err
	}

	s.rcmgr.transient.ReleaseForChild(stat)
	s.rcmgr.transient.DecRef() // removed from edges

	// update edges
	edges := []*resourceScope{
		s.peer.resourceScope,
		s.peerProtoScope,
		s.proto.resourceScope,
		s.rcmgr.system.resourceScope,
	}
	s.resourceScope.edges = edges

	s.rcmgr.metrics.AllowProtocol(proto)
	return nil
}

func (s *streamScope) ServiceScope() network.ServiceScope {
	s.Lock()
	defer s.Unlock()

	// avoid nil is not nil footgun; go....
	if s.svc == nil {
		return nil
	}

	return s.svc
}

func (s *streamScope) SetService(svc string) error {
	s.Lock()
	defer s.Unlock()

	if s.svc != nil {
		return fmt.Errorf("stream scope already attached to a service")
	}
	if s.proto == nil {
		return fmt.Errorf("stream scope not attached to a protocol")
	}

	s.svc = s.rcmgr.getServiceScope(svc)

	// reserve resources in service
	stat := s.resourceScope.rc.stat()
	if err := s.svc.ReserveForChild(stat); err != nil {
		s.svc.DecRef()
		s.svc = nil
		s.rcmgr.metrics.BlockService(svc)
		return err
	}

	// get the per peer service scope constraint, if any
	s.peerSvcScope = s.svc.getPeerScope(s.peer.peer)
	if err := s.peerSvcScope.ReserveForChild(stat); err != nil {
		s.svc.ReleaseForChild(stat)
		s.svc.DecRef()
		s.svc = nil
		s.peerSvcScope.DecRef()
		s.peerSvcScope = nil
		s.rcmgr.metrics.BlockServicePeer(svc, s.peer.peer)
		return err
	}

	// update edges
	edges := []*resourceScope{
		s.peer.resourceScope,
		s.peerProtoScope,
		s.peerSvcScope,
		s.proto.resourceScope,
		s.svc.resourceScope,
		s.rcmgr.system.resourceScope,
	}
	s.resourceScope.edges = edges

	s.rcmgr.metrics.AllowService(svc)
	return nil
}

func (s *streamScope) PeerScope() network.PeerScope {
	s.Lock()
	defer s.Unlock()

	// avoid nil is not nil footgun; go....
	if s.peer == nil {
		return nil
	}

	return s.peer
}
