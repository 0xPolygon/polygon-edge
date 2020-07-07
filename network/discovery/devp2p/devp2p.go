package discv4

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/enode"
	"github.com/0xPolygon/minimal/helper/hex"
	kbucket "github.com/ferranbt/go-kademlia-bucket"
	"github.com/hashicorp/go-hclog"

	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"

	"github.com/0xPolygon/minimal/network/discovery"
	"github.com/armon/go-metrics"
)

var (
	bucketSize         = 16
	bondExpiration     = 24 * time.Hour
	respTimeout        = 10 * time.Second
	revalidateInterval = 10 * time.Second
	lookupInterval     = 1 * time.Minute
	numProbeTasks      = 2
)

const (
	nodeIDBytes  = 512 / 8
	maxNeighbors = 6
	alpha        = 3
)

// Peer is the discovery peer
type Peer struct {
	ID      string
	Bytes   []byte
	UDPAddr *net.UDPAddr
	Last    *time.Time // last time pinged
	TCP     uint16
}

// Enode returns an enode address
func (p *Peer) Enode() string {
	id := strings.Replace(hex.EncodeToHex(p.Bytes), "0x", "", -1)
	return fmt.Sprintf("enode://%s@%s:%d", id, p.UDPAddr.IP.String(), p.TCP)
}

func (p *Peer) addr() string {
	return p.UDPAddr.String()
}

func (p *Peer) hasExpired(now time.Time, expiration time.Duration) bool {
	if p.Last == nil {
		return true
	}
	return p.Last.Add(expiration).Before(now)
}

func (p *Peer) toRPCNode() rpcNode {
	r := rpcNode{}
	r.ID = p.Bytes
	r.IP = p.UDPAddr.IP
	r.UDP = uint16(p.UDPAddr.Port)
	r.TCP = 0
	return r
}

func (p *Peer) toRPCEndpoint() rpcEndpoint {
	r := rpcEndpoint{}
	r.IP = p.UDPAddr.IP
	r.UDP = uint16(p.UDPAddr.Port)
	r.TCP = p.TCP
	return r
}

func newPeer(id string, addr *net.UDPAddr, tcp uint16) (*Peer, error) {
	if strings.HasPrefix(id, "0x") {
		id = id[2:]
	}
	if len(id) != 128 {
		return nil, fmt.Errorf("id should be 128 in length not %d", len(id))
	}

	bytes, err := hex.DecodeHex("0x" + id)
	if err != nil {
		return nil, err
	}

	return &Peer{id, bytes, addr, nil, tcp}, nil
}

const (
	macSize  = 256 / 8
	sigSize  = 520 / 8
	headSize = macSize + sigSize // space of packet frame data
)

var (
	headSpace = make([]byte, headSize)
)

// RPC packet types
const (
	pingPacket = iota + 1 // zero is 'reserved'
	pongPacket
	findnodePacket
	neighborsPacket
)

type rlpMessage interface {
	MarshalRLP(dst []byte) []byte
}

// default rlp arena pool for discovery messages
var defaultArenaPool fastrlp.ArenaPool

// default rlp parser pool for discovery messages
var defaultParserPool fastrlp.ParserPool

type pingRequest struct {
	Version    uint64
	From       rpcEndpoint
	To         rpcEndpoint
	Expiration uint64 `rlp:"tail"`
}

func (p *pingRequest) UnmarshalRLP(v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if len(elems) < 4 {
		return fmt.Errorf("bad")
	}
	p.Version, err = elems[0].GetUint64()
	if err != nil {
		return err
	}
	if err := p.From.UnmarshalRLP(elems[1]); err != nil {
		return err
	}
	if err := p.To.UnmarshalRLP(elems[2]); err != nil {
		return err
	}
	p.Expiration, err = elems[3].GetUint64()
	if err != nil {
		return err
	}
	return nil
}

func (p *pingRequest) MarshalRLP(dst []byte) []byte {
	a := defaultArenaPool.Get()

	v := a.NewArray()
	v.Set(a.NewUint(p.Version))
	v.Set(p.From.MarshalRLP(a))
	v.Set(p.To.MarshalRLP(a))
	v.Set(a.NewUint(p.Expiration))

	dst = v.MarshalTo(dst)
	defaultArenaPool.Put(a)
	return dst
}

// pong is the reply to ping. call is response
type pongResponse struct {
	To         rpcEndpoint
	ReplyTok   []byte
	Expiration uint64 `rlp:"tail"`
}

func (p *pongResponse) MarshalRLP(dst []byte) []byte {
	a := defaultArenaPool.Get()

	v := a.NewArray()
	v.Set(p.To.MarshalRLP(a))
	v.Set(a.NewCopyBytes(p.ReplyTok))
	v.Set(a.NewUint(p.Expiration))

	dst = v.MarshalTo(dst)
	defaultArenaPool.Put(a)
	return dst
}

type findNodeRequest struct {
	Target     []byte
	Expiration uint64 `rlp:"tail"`
}

func (f *findNodeRequest) UnmarshalRLP(v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if len(elems) < 2 {
		return fmt.Errorf("bad")
	}
	f.Target, err = elems[0].GetBytes(f.Target[:0])
	if err != nil {
		return err
	}
	f.Expiration, err = elems[1].GetUint64()
	if err != nil {
		return err
	}
	return nil
}

func (f *findNodeRequest) MarshalRLP(dst []byte) []byte {
	a := defaultArenaPool.Get()

	v := a.NewArray()
	v.Set(a.NewCopyBytes(f.Target))
	v.Set(a.NewUint(f.Expiration))

	dst = v.MarshalTo(dst)
	defaultArenaPool.Put(a)
	return dst
}

// reply to findnode
type neighborsResponse struct {
	Nodes      []rpcNode
	Expiration uint64 `rlp:"tail"`
}

func (n *neighborsResponse) UnmarshalRLP(v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if len(elems) < 2 {
		return fmt.Errorf("bad")
	}
	n.Nodes = n.Nodes[:0]

	nodes, err := elems[0].GetElems()
	if err != nil {
		return err
	}
	for _, i := range nodes {
		node := rpcNode{}
		if err := node.UnmarshalRLP(i); err != nil {
			return err
		}
		n.Nodes = append(n.Nodes, node)
	}

	n.Expiration, err = elems[1].GetUint64()
	if err != nil {
		return err
	}
	return nil
}

func (n *neighborsResponse) MarshalRLP(dst []byte) []byte {
	a := defaultArenaPool.Get()

	v := a.NewArray()
	if len(n.Nodes) == 0 {
		v.Set(a.NewNullArray())
	} else {
		vv := a.NewArray()
		for _, i := range n.Nodes {
			vv.Set(i.MarshalRLP(a))
		}
		v.Set(vv)
	}
	v.Set(a.NewUint(n.Expiration))

	dst = v.MarshalTo(dst)
	defaultArenaPool.Put(a)
	return dst
}

type rpcNode struct {
	IP  net.IP // len 4 for IPv4 or 16 for IPv6
	UDP uint16 // for discovery protocol
	TCP uint16 // for RLPx protocol
	ID  []byte
}

func (r *rpcNode) UnmarshalRLP(v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	r.IP, err = elems[0].GetBytes(r.IP[:0])
	if err != nil {
		return fmt.Errorf("failed to decode ip")
	}
	udp, err := elems[1].GetUint64()
	if err != nil {
		return err
	}
	tcp, err := elems[2].GetUint64()
	if err != nil {
		return err
	}
	r.ID, err = elems[3].GetBytes(r.ID[:0])
	if err != nil {
		return err
	}

	r.UDP = uint16(udp)
	r.TCP = uint16(tcp)
	return nil
}

func (r *rpcNode) MarshalRLP(a *fastrlp.Arena) *fastrlp.Value {
	v := a.NewArray()
	v.Set(a.NewCopyBytes(r.IP))
	v.Set(a.NewUint(uint64(r.UDP)))
	v.Set(a.NewUint(uint64(r.TCP)))
	v.Set(a.NewCopyBytes(r.ID))
	return v
}

func (r *rpcNode) toPeer() (*Peer, error) {
	return newPeer(hex.EncodeToHex(r.ID), &net.UDPAddr{IP: r.IP, Port: int(r.UDP)}, r.TCP)
}

type rpcEndpoint struct {
	IP  net.IP // len 4 for IPv4 or 16 for IPv6
	UDP uint16 // for discovery protocol
	TCP uint16 // for RLPx protocol
}

func (r *rpcEndpoint) UnmarshalRLP(v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	r.IP, err = elems[0].GetBytes(r.IP[:0])
	if err != nil {
		return err
	}
	udp, err := elems[1].GetUint64()
	if err != nil {
		return err
	}
	tcp, err := elems[2].GetUint64()
	if err != nil {
		return err
	}

	r.UDP = uint16(udp)
	r.TCP = uint16(tcp)
	return nil
}

func (r *rpcEndpoint) MarshalRLP(a *fastrlp.Arena) *fastrlp.Value {
	v := a.NewArray()
	v.Set(a.NewCopyBytes(r.IP))
	v.Set(a.NewUint(uint64(r.UDP)))
	v.Set(a.NewUint(uint64(r.TCP)))
	return v
}

// Backend is the p2p discover backend
type Backend struct {
	logger     hclog.Logger
	ID         *ecdsa.PrivateKey
	timer      *time.Timer
	handlers   map[string]func(payload []byte, timestamp *time.Time)
	respLock   sync.Mutex
	validLock  sync.Mutex
	table      *kbucket.RoutingTable
	nodes      map[string]*Peer
	local      *Peer
	shutdownCh chan bool
	shutdown   int32
	active     bool // set to true when the bootnodes are loaded
	eventCh    chan string
	tasks      chan *Peer
	inlookup   int32
	addr       *net.UDPAddr
	transport  Transport
	packetCh   chan *Packet

	bootnodes []string
}

func Factory(ctx context.Context, conf *discovery.BackendConfig) (discovery.Backend, error) {
	addr := conf.Enode.IP.String()
	port := int(conf.Enode.UDP)

	bootnodes := []string{}
	bootnodesRaw, ok := conf.Config["bootnodes"]
	if ok {
		bootnodes, ok = bootnodesRaw.(chain.Bootnodes)
		if !ok {
			return nil, fmt.Errorf("could not convert %s to a list of bootnodes", bootnodesRaw)
		}
	}

	udpAddr := &net.UDPAddr{IP: net.ParseIP(addr), Port: port}

	transport, err := newUDPTransport(udpAddr)
	if err != nil {
		return nil, err
	}
	d, err := NewBackend(conf.Logger, conf.Key, transport)
	if err != nil {
		return nil, err
	}
	d.SetBootnodes(bootnodes)
	return d, nil
}

// NewBackend creates a new p2p discovery protocol
func NewBackend(logger hclog.Logger, key *ecdsa.PrivateKey, transport Transport) (*Backend, error) {
	addr := transport.Addr()

	pub := &key.PublicKey
	id := hex.EncodeToHex(elliptic.Marshal(pub.Curve, pub.X, pub.Y)[1:])

	localPeer, err := newPeer(id, addr, 0)
	if err != nil {
		return nil, err
	}

	hashFn := sha3.NewLegacyKeccak256()

	r := &Backend{
		logger:     logger,
		ID:         key,
		addr:       addr,
		handlers:   map[string]func(payload []byte, timestamp *time.Time){},
		respLock:   sync.Mutex{},
		validLock:  sync.Mutex{},
		nodes:      map[string]*Peer{},
		local:      localPeer,
		table:      kbucket.NewRoutingTable(bucketSize, localPeer.ID, 1000*time.Second, hashFn),
		shutdownCh: make(chan bool),
		eventCh:    make(chan string, 10),
		tasks:      make(chan *Peer, 100),
		inlookup:   0,
		transport:  transport,
	}

	go r.listen()

	// Start probe tasks
	for i := 0; i < numProbeTasks; i++ {
		go r.probeTask(strconv.Itoa(i))
	}

	return r, nil
}

// SetBootnodes sets the bootnodes
func (b *Backend) SetBootnodes(bootnodes []string) {
	b.bootnodes = bootnodes
}

func (b *Backend) listen() {
	for {
		select {
		case packet := <-b.transport.PacketCh():
			if b.packetCh != nil {
				b.packetCh <- packet
			} else {
				go func() {
					if err := b.HandlePacket(packet); err != nil {
						//b.logger.Trace("failed to handle packet", "err", err.Error())
					}
				}()
			}
		case <-b.shutdownCh:
			return
		}
	}
}

func (b *Backend) sendTask(peer *Peer) {
	select {
	case b.tasks <- peer:
	default:
	}
}

func (b *Backend) probeTask(id string) {
	for {
		select {
		case task := <-b.tasks:
			b.probeNode(task)

		case <-b.shutdownCh:
			return
		}
	}
}

// Schedule starts the discovery protocol
func (b *Backend) Schedule() {
	go b.schedule()
	go b.loadBootnodes()
}

func (b *Backend) loadBootnodes() {
	// load bootnodes
	errr := make(chan error, len(b.bootnodes))

	for _, p := range b.bootnodes {
		go func(p string) {
			err := b.AddNode(p)
			errr <- err
		}(p)
	}

	for i := 0; i < len(b.bootnodes); i++ {
		<-errr
	}

	// start the initial lookup
	b.active = true

	b.logger.Info("Finished probing bootnodes")
	b.Lookup()
}

// Close closes the discover
func (b *Backend) Close() error {
	close(b.shutdownCh)
	b.transport.Shutdown()
	return nil
}

func (b *Backend) schedule() {
	revalidate := time.NewTicker(revalidateInterval)
	lookup := time.NewTicker(lookupInterval)

	for {
		select {
		case <-lookup.C:
			go b.LookupRandom()

		case <-revalidate.C:
			go b.revalidatePeer()

		case <-b.shutdownCh:
			return
		}
	}
}

func (b *Backend) revalidatePeer() {
	var id string
	for _, i := range rand.Perm(len(b.table.Buckets)) {
		if bucket := b.table.Buckets[i]; bucket.Len() != 0 {
			id = bucket.Peers()[bucket.Len()-1]
			break
		}
	}

	peer, ok := b.getPeer(id)
	if !ok {
		// log, failed to get peer, weird
	} else {
		b.probeNode(peer)
	}
}

func (b *Backend) NearestPeers() ([]*Peer, error) {
	return b.NearestPeersFromTarget(b.local.Bytes)
}

func (b *Backend) NearestPeersFromTarget(target []byte) ([]*Peer, error) {
	key := hex.EncodeToHex(target)

	peers := []*Peer{}
	for _, p := range b.table.NearestPeers(key, bucketSize) {
		peer, ok := b.getPeer(p)
		if !ok {
			return nil, fmt.Errorf("peer %s not found", p)
		}
		peers = append(peers, peer)
	}
	return peers, nil
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

// LookupRandom performs a lookup in a random target
// TODO, remove peers from lookup response, nobody uses it
func (b *Backend) LookupRandom() ([]*Peer, error) {
	var t [64]byte
	crand.Read(t[:])

	target := []byte{}
	for _, i := range t {
		target = append(target, i)
	}

	return b.LookupTarget(target)
}

// Lookup does a kademlia lookup with the local key as target
func (b *Backend) Lookup() ([]*Peer, error) {
	return b.LookupTarget(b.local.Bytes)
}

// LookupTarget does a kademlia lookup around target
func (b *Backend) LookupTarget(target []byte) ([]*Peer, error) {
	// Only allow one lookup at a time
	if atomic.LoadInt32(&(b.inlookup)) == 1 {
		return nil, nil
	}

	defer func() {
		atomic.StoreInt32(&b.inlookup, 0)
	}()

	if !b.active {
		return []*Peer{}, nil
	}

	visited := map[string]bool{}

	// initialize the queue
	queue, err := b.NearestPeersFromTarget(target)
	if err != nil {
		return nil, err
	}

	reply := make(chan []*Peer)

	var peer *Peer

	pending := 0

	findNodes := func(p *Peer) {
		pending++
		go func() {
			nodes, err := b.findNodes(p, target)
			if err != nil {
				// log
			}
			reply <- nodes
		}()
	}

	// seed up to maxPending nodes
	for i := 0; i < min(len(queue), alpha); i++ {
		peer, queue = queue[0], queue[1:]
		findNodes(peer)
	}

	for pending > 0 {

		var nodes []*Peer
		select {
		case nodes = <-reply:
		case <-b.shutdownCh:
			return nil, nil
		}

		pending--

		v := 0
		for _, p := range nodes {
			if _, ok := visited[p.ID]; !ok {
				visited[p.ID] = true
				queue = append(queue, p)
				v++
			}
		}

		for pending < alpha && len(queue) != 0 {
			peer, queue = queue[0], queue[1:]
			findNodes(peer)
		}
	}

	discovered := []*Peer{}
	for id := range visited {
		p, ok := b.getPeer(id)
		if !ok {
			return nil, fmt.Errorf("")
		}

		discovered = append(discovered, p)
	}

	return discovered, nil
}

// GetPeers return the peers
func (b *Backend) GetPeers() []*Peer {
	peers := []*Peer{}
	for _, peer := range b.nodes {
		peers = append(peers, peer)
	}
	return peers
}

func decodePacket(payload []byte) ([]byte, []byte, []byte, error) {
	if len(payload) < headSize+1 {
		return nil, nil, nil, fmt.Errorf("Payload size %d exceeds heapSize limits %d", len(payload), headSize+1)
	}

	mac, sig, sigdata := payload[:macSize], payload[macSize:headSize], payload[headSize:]
	if !bytes.Equal(mac, crypto.Keccak256(payload[macSize:])) {
		return nil, nil, nil, fmt.Errorf("macs do not match")
	}

	pubkey, err := crypto.RecoverPubkey(sig, crypto.Keccak256(sigdata))
	if err != nil {
		return nil, nil, nil, err
	}
	return mac, sigdata, crypto.MarshallPublicKey(pubkey), nil
}

// Used in tests
func decodePeerFromPacket(packet *Packet) (*Peer, error) {
	_, _, pubkey, err := decodePacket(packet.Buf)
	if err != nil {
		return nil, err
	}

	addr, ok := packet.From.(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("expected udp addr")
	}

	peer, err := newPeer(hex.EncodeToHex(pubkey[1:]), addr, 0)
	if err != nil {
		return nil, err
	}
	peer.Last = &packet.Timestamp
	return peer, nil
}

// HandlePacket handles an incoming udp packet
func (b *Backend) HandlePacket(packet *Packet) error {
	mac, sigdata, pubkey, err := decodePacket(packet.Buf)
	if err != nil {
		return err
	}

	addr, ok := packet.From.(*net.UDPAddr)
	if !ok {
		return fmt.Errorf("expected udp addr")
	}

	peer, err := newPeer(hex.EncodeToHex(pubkey[1:]), addr, 0)
	if err != nil {
		return err
	}
	peer.Last = &packet.Timestamp

	msgcode, payload := sigdata[0], sigdata[1:]

	if callback, ok := b.getCallback(peer.ID, msgcode); ok {
		callback(payload, &packet.Timestamp)

		// We can also create callbacks for pingPackets that work as notifications
		// but we still have to send the pong
		if msgcode != pingPacket {
			return nil
		}
	}

	switch msgcode {
	case pingPacket:
		return b.handlePingPacket(payload, mac, peer)
	case findnodePacket:
		return b.handleFindNodePacket(payload, peer)
	case neighborsPacket:
		return fmt.Errorf("neighborsPacket not expected")
	case pongPacket:
		return fmt.Errorf("pongPacket not expected %20s %20s %s", peer.addr(), peer.ID, hex.EncodeToHex(peer.Bytes))
	default:
		return fmt.Errorf("message code %d not found", msgcode)
	}
}

func (b *Backend) getCallback(id string, code byte) (func([]byte, *time.Time), bool) {
	key := fmt.Sprintf("%s_%v", id, code)
	b.respLock.Lock()
	c, ok := b.handlers[key]
	b.respLock.Unlock()
	return c, ok
}

func (b *Backend) handlePingPacket(payload []byte, mac []byte, peer *Peer) error {
	/*
		var req pingRequest
		if err := rlp.DecodeBytes(payload, &req); err != nil {
			panic(err)
		}
	*/

	var req pingRequest
	p := &fastrlp.Parser{}
	v, err := p.Parse(payload)
	if err != nil {
		panic(err)
	}
	if err := req.UnmarshalRLP(v); err != nil {
		panic(err)
	}

	/*
		if !reflect.DeepEqual(req, req2) {
			fmt.Println("__ BAD RLP ENCODING __")
			fmt.Println(payload)
			fmt.Println(req)
			fmt.Println(req2)
			panic("Xa")
		}
	*/

	if hasExpired(req.Expiration) {
		return fmt.Errorf("ping: Message has expired")
	}

	reply := &pongResponse{
		To:         peer.toRPCEndpoint(),
		ReplyTok:   mac,
		Expiration: uint64(time.Now().Add(20 * time.Second).Unix()),
	}

	b.sendPacket(peer, pongPacket, reply)

	// received a ping, probe back it it has expired
	if b.hasExpired(peer) {
		b.sendTask(peer)
	} else {
		b.updatePeer(peer)
	}

	return nil
}

func (b *Backend) hasExpired(p *Peer) bool {
	b.validLock.Lock()
	defer b.validLock.Unlock()

	// If we check hasExpired if the node has not been probed yet
	// i.e. first findNodes query
	if p.Last == nil {
		return true
	}

	receivedTime := p.Last
	p, ok := b.nodes[p.ID]
	if !ok {
		return true
	}

	return p.hasExpired(*receivedTime, bondExpiration)
}

func (b *Backend) handleFindNodePacket(payload []byte, peer *Peer) error {
	if b.hasExpired(peer) {
		return nil
	}

	/*
		var req2 findNodeRequest
		if err := rlp.DecodeBytes(payload, &req); err != nil {
			return err
		}
	*/

	var req findNodeRequest
	p := &fastrlp.Parser{}
	v, err := p.Parse(payload)
	if err != nil {
		panic(err)
	}
	if err := req.UnmarshalRLP(v); err != nil {
		panic(err)
	}
	/*
		if !reflect.DeepEqual(req, req2) {
			panic("panic: findNodeRequest")
		}
	*/

	peers, err := b.NearestPeersFromTarget(req.Target)
	if err != nil {
		return err
	}

	// NOTE: number of peers on each requests is fixed to 'maxNeighbors'
	for i := 0; i < len(peers); i += maxNeighbors {
		nodes := []rpcNode{}
		for _, p := range peers[i:min(i+maxNeighbors, len(peers))] {
			nodes = append(nodes, p.toRPCNode())
		}

		err := b.sendPacket(peer, neighborsPacket, &neighborsResponse{
			Nodes: nodes,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func hasExpired(ts uint64) bool {
	return time.Unix(int64(ts), 0).Before(time.Now())
}

// AddNode adds a new node to the discover process (NOTE: its a sync process)
// TODO. split in nodeStrToPeer and probe
func (b *Backend) AddNode(nodeStr string) error {
	node, err := enode.ParseURL(nodeStr)
	if err != nil {
		return err
	}

	id := make([]byte, 64)
	for i := 0; i < len(id); i++ {
		id[i] = node.ID[i]
	}

	peer, err := newPeer(hex.EncodeToHex(id), &net.UDPAddr{IP: node.IP, Port: int(node.UDP)}, node.TCP)
	if err != nil {
		return err
	}

	b.probeNode(peer)
	return nil
}

func (b *Backend) probeNode(peer *Peer) bool {
	metrics.IncrCounter([]string{"discover", "probe"}, 1.0)

	// Send ping packet
	ack := make(chan respMessage)
	b.setHandler(peer.ID, pongPacket, ack, respTimeout)

	b.sendPacket(peer, pingPacket, &pingRequest{
		Version:    4,
		From:       b.local.toRPCEndpoint(),
		To:         peer.toRPCEndpoint(),
		Expiration: uint64(time.Now().Add(10 * time.Second).Unix()),
	})

	resp := <-ack

	if resp.Complete {
		peer.Last = resp.Timestamp
		b.updatePeer(peer)

		select {
		case b.eventCh <- peer.Enode():
		default:
		}
		return true
	}

	return false
}

func (b *Backend) Deliver() chan string {
	return b.eventCh
}

func (b *Backend) getPeer(id string) (*Peer, bool) {
	b.validLock.Lock()
	defer b.validLock.Unlock()

	p, ok := b.nodes[id]
	return p, ok
}

func (b *Backend) updatePeer(peer *Peer) {
	b.validLock.Lock()
	defer b.validLock.Unlock()

	b.table.Update(peer.ID)

	if p, ok := b.nodes[peer.ID]; ok {
		p.TCP = peer.TCP

		// if already in, update the timestamp
		// only update if there is a timestamp
		if peer.Last != nil {
			p.Last = peer.Last
		}
	} else {
		b.nodes[peer.ID] = peer
	}
}

func (b *Backend) findNodes(peer *Peer, target []byte) ([]*Peer, error) {
	if b.hasExpired(peer) {
		// The connect has expired with the node, lets probe again.
		if !b.probeNode(peer) {
			return nil, fmt.Errorf("failed to probe node")
		}

		// wait for a ping from the peer to ensure we are alive on his side
		ack := make(chan respMessage)
		b.setHandler(peer.ID, pingPacket, ack, respTimeout)

		if resp := <-ack; !resp.Complete {
			return nil, fmt.Errorf("We have not received probe back from other peer")
		}

		// sleep a couple of milliseconds to send the other package first
		time.Sleep(100 * time.Millisecond)
	}

	ack := make(chan respMessage)
	b.setHandler(peer.ID, neighborsPacket, ack, respTimeout)

	b.sendPacket(peer, findnodePacket, &findNodeRequest{
		Target:     b.local.Bytes,
		Expiration: uint64(time.Now().Add(20 * time.Second).Unix()),
	})

	peers := []*Peer{}
	atLeastOne := false

	for {
		resp := <-ack
		if resp.Complete {
			atLeastOne = true

			/*
				var req2 neighborsResponse
				if err := rlp.DecodeBytes(resp.Payload, &neighbors); err != nil {
					panic(err)
				}
			*/

			var neighbors neighborsResponse
			p := &fastrlp.Parser{}
			v, err := p.Parse(resp.Payload)
			if err != nil {
				panic(err)
			}
			if err := neighbors.UnmarshalRLP(v); err != nil {
				panic(err)
			}

			/*
				if !reflect.DeepEqual(neighbors, req2) {

					fmt.Println("-- payload --")
					fmt.Println(resp.Payload)

					fmt.Println("-- expected --")
					fmt.Println(neighbors)
					fmt.Println(neighbors.Nodes)
					fmt.Println(neighbors.Expiration)

					fmt.Println("-- found --")
					fmt.Println(req2)
					fmt.Println(req2.Nodes)
					fmt.Println(req2.Expiration)

					fmt.Println(reflect.DeepEqual(neighbors.Nodes, req2.Nodes))
					fmt.Println(reflect.DeepEqual(neighbors.Expiration, req2.Expiration))

					if len(neighbors.Nodes) == len(req2.Nodes) && len(req2.Nodes) == 0 {
						//
					} else {
						panic("panic: neighborsResponse")
					}
				}
			*/

			for _, n := range neighbors.Nodes {
				p, err := n.toPeer()
				if err != nil {
					return nil, err
				}
				peers = append(peers, p)
			}

			if len(peers) == bucketSize {
				break
			}
		} else {
			break
		}
	}

	// not even one response
	if !atLeastOne {
		return nil, fmt.Errorf("failed to get peers")
	}

	for _, p := range peers {
		b.sendTask(p)
	}

	return peers, nil
}

type respMessage struct {
	Complete  bool
	Payload   []byte
	Timestamp *time.Time
}

func (b *Backend) setHandler(id string, code byte, ackCh chan respMessage, expiration time.Duration) {
	key := fmt.Sprintf("%s_%v", id, code)

	callback := func(payload []byte, timestamp *time.Time) {
		select {
		case ackCh <- respMessage{true, payload, timestamp}:
		default:
		}
	}

	b.respLock.Lock()
	b.handlers[key] = callback
	b.respLock.Unlock()

	b.timer = time.AfterFunc(expiration, func() {
		b.respLock.Lock()
		delete(b.handlers, key)
		b.respLock.Unlock()

		select {
		case ackCh <- respMessage{false, nil, nil}:
		default:
		}
	})
}

func (b *Backend) sendPacket(peer *Peer, code byte, payload rlpMessage) error {
	data, err := b.encodePacket(code, payload)
	if err != nil {
		return err
	}

	if _, err := b.transport.WriteTo(data, peer.addr()); err != nil {
		return err
	}

	return nil
}

func (b *Backend) encodePacket(code byte, payload rlpMessage) ([]byte, error) {

	/*
		buf := new(bytes.Buffer)
		buf.Write(headSpace)
		buf.WriteByte(code)

		if err := rlp.Encode(buf, payload); err != nil {
			return nil, err
		}
	*/

	/*
		buf := new(bytes.Buffer)
		buf.Write(headSpace)
		buf.WriteByte(code)

		if err := rlp.Encode(buf, payload); err != nil {
			return nil, err
		}

		extra, err := rlp.EncodeToBytes(payload)
		if err != nil {
			panic(err)
		}

		extra1 := payload.MarshalRLP(nil)

		if !bytes.Equal(extra, extra1) {

			fmt.Println("__ BAD RLP ENCODING __")
			fmt.Println(payload)
			fmt.Println(extra)
			fmt.Println(extra1)

			panic("Xb")
		}
	*/

	packet := []byte{}
	packet = append(packet, headSpace...)
	packet = append(packet, code)

	packet = payload.MarshalRLP(packet)
	// TODO, not sure about this one

	// packet := buf.Bytes()

	/*
		if !bytes.Equal(packet, buf.Bytes()) {
			panic("XX")
		}
	*/

	sig, err := crypto.Sign(b.ID, crypto.Keccak256(packet[headSize:]))
	if err != nil {
		return nil, err
	}
	copy(packet[macSize:], sig)

	hash := crypto.Keccak256(packet[macSize:])
	copy(packet, hash)

	return packet, nil
}
