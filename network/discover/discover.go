package discover

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/armon/go-metrics"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/rlp"

	crand "crypto/rand"

	kb "github.com/libp2p/go-libp2p-kbucket"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
)

const (
	// udpPacketBufSize is used to buffer incoming packets during read
	// operations.
	udpPacketBufSize = 1280
)

const (
	nodeIDBytes               = 512 / 8
	defaultBucketSize         = 16
	defaultBondExpiration     = 24 * time.Hour
	defaultRespTimeout        = 10 * time.Second
	defaultRevalidateInterval = 10 * time.Second
	defaultNumTasks           = 4
	maxNeighbors              = 6
	alpha                     = 3
)

// Config is the discover configuration
type Config struct {
	BindAddr           string
	BindPort           int
	BucketSize         int
	BondExpiration     time.Duration
	RespTimeout        time.Duration
	RevalidateInterval time.Duration
	LookupInterval     time.Duration
	NumProbeTasks      int
}

// DefaultConfig is the default config
func DefaultConfig() *Config {
	c := &Config{
		BucketSize:         defaultBucketSize,
		BondExpiration:     defaultBondExpiration,
		RespTimeout:        defaultRespTimeout,
		RevalidateInterval: defaultRevalidateInterval,
		LookupInterval:     1 * time.Minute,
		NumProbeTasks:      2,
		BindAddr:           "127.0.0.1",
		BindPort:           30303,
	}
	return c
}

// Peer is the discovery peer
type Peer struct {
	ID      peer.ID
	Bytes   []byte
	UDPAddr *net.UDPAddr
	Last    *time.Time // last time pinged
	TCP     uint16
}

// Enode returns an enode address
func (p *Peer) Enode() string {
	id := strings.Replace(hexutil.Encode(p.Bytes), "0x", "", -1)
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

	bytes, err := hexutil.Decode("0x" + id)
	if err != nil {
		return nil, err
	}

	return &Peer{peer.ID(id), bytes, addr, nil, tcp}, nil
}

type Node = discv5.Node

func nodeToRpcEndpoint(node *Node) *rpcEndpoint {
	return nil
}

type NodeID = discv5.NodeID

func ParseNode(node string) (*Node, error) {
	return discv5.ParseNode(node)
}

func PubkeyToNodeID(pub *ecdsa.PublicKey) NodeID {
	return discv5.PubkeyID(pub)
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

type pingRequest struct {
	Version    uint
	From       rpcEndpoint
	To         rpcEndpoint
	Expiration uint64
	Rest       []rlp.RawValue `rlp:"tail"`
}

// pong is the reply to ping. call is response
type pongResponse struct {
	To         rpcEndpoint
	ReplyTok   []byte
	Expiration uint64
	Rest       []rlp.RawValue `rlp:"tail"`
}

type findNodeRequest struct {
	Target     []byte
	Expiration uint64
	Rest       []rlp.RawValue `rlp:"tail"`
}

// reply to findnode
type neighborsResponse struct {
	Nodes      []rpcNode
	Expiration uint64
	Rest       []rlp.RawValue `rlp:"tail"`
}

type rpcNode struct {
	IP  net.IP // len 4 for IPv4 or 16 for IPv6
	UDP uint16 // for discovery protocol
	TCP uint16 // for RLPx protocol
	ID  []byte
}

func (r *rpcNode) toPeer() (*Peer, error) {
	return newPeer(hexutil.Encode(r.ID), &net.UDPAddr{IP: r.IP, Port: int(r.UDP)}, r.TCP)
}

type rpcEndpoint struct {
	IP  net.IP // len 4 for IPv4 or 16 for IPv6
	UDP uint16 // for discovery protocol
	TCP uint16 // for RLPx protocol
}

// Discover is the p2p discover protocol
type Discover struct {
	logger *log.Logger
	ID     *ecdsa.PrivateKey
	config *Config
	// transport  *transport
	timer      *time.Timer
	handlers   map[string]func(payload []byte, timestamp *time.Time)
	respLock   sync.Mutex
	validLock  sync.Mutex
	table      *kb.RoutingTable
	nodes      map[peer.ID]*Peer
	local      *Peer
	shutdownCh chan bool
	shutdown   int32
	active     bool // set to true when the bootnodes are loaded
	EventCh    chan string
	bootnodes  []string
	tasks      chan *Peer
	inlookup   int32
	addr       *net.UDPAddr
	listener   *net.UDPConn
	pool       sync.Pool
	packetCh   chan *Packet
}

// Packet is used to provide some metadata about incoming packets from peers
type Packet struct {
	Buf       []byte
	From      net.Addr
	Timestamp time.Time
}

// NewDiscover creates a new routing table
func NewDiscover(logger *log.Logger, key *ecdsa.PrivateKey, config *Config) (*Discover, error) {
	addr := &net.UDPAddr{IP: net.ParseIP(config.BindAddr), Port: config.BindPort}

	pub := &key.PublicKey
	id := hexutil.Encode(elliptic.Marshal(pub.Curve, pub.X, pub.Y)[1:])

	localPeer, err := newPeer(id, addr, 0)
	if err != nil {
		return nil, err
	}

	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("Failed to start UDP listener on %q: %v", addr.String(), err)
	}

	r := &Discover{
		logger:     logger,
		ID:         key,
		config:     config,
		addr:       addr,
		listener:   l,
		handlers:   map[string]func(payload []byte, timestamp *time.Time){},
		respLock:   sync.Mutex{},
		validLock:  sync.Mutex{},
		nodes:      map[peer.ID]*Peer{},
		local:      localPeer,
		table:      kb.NewRoutingTable(config.BucketSize, kb.ConvertPeerID(localPeer.ID), 1000*time.Second, peerstore.NewMetrics()),
		shutdownCh: make(chan bool),
		EventCh:    make(chan string, 10),
		bootnodes:  []string{},
		tasks:      make(chan *Peer, 100),
		inlookup:   0,
	}

	r.pool = sync.Pool{
		New: func() interface{} {
			return make([]byte, udpPacketBufSize)
		},
	}

	go r.listen()

	// Start probe tasks
	for i := 0; i < config.NumProbeTasks; i++ {
		go r.probeTask(strconv.Itoa(i))
	}

	return r, nil
}

func (d *Discover) listen() {
	for {
		vbuf := d.pool.Get()
		buf := vbuf.([]byte)

		n, addr, err := d.listener.ReadFrom(buf)
		ts := time.Now()
		if err != nil {
			if s := atomic.LoadInt32(&d.shutdown); s == 1 {
				break
			}
			d.logger.Printf("Error reading UDP packet: %v", err)
			continue
		}

		// Check the length - it needs to have at least one byte
		if n < 1 {
			d.logger.Printf("UDP packet too short (%d bytes) %s", len(buf), addr)
			continue
		}

		packet := &Packet{
			Buf:       buf[:n],
			From:      addr,
			Timestamp: ts,
		}
		go func() {
			if d.packetCh != nil {
				d.packetCh <- packet
			} else {
				if err := d.HandlePacket(packet); err != nil {
					d.logger.Printf(err.Error())
				}
				d.pool.Put(vbuf)
			}
		}()
	}
}

// SetBootnodes set the bootnodes for discovering
func (d *Discover) SetBootnodes(bootnodes []string) {
	d.bootnodes = bootnodes
}

func (d *Discover) sendTask(peer *Peer) {
	select {
	case d.tasks <- peer:
	default:
	}
}

func (d *Discover) probeTask(id string) {
	for {
		select {
		case task := <-d.tasks:
			d.probeNode(task)

		case <-d.shutdownCh:
			return
		}
	}
}

// Schedule starts the discovery protocol
func (d *Discover) Schedule() {
	go d.schedule()
	go d.loadBootnodes()
}

func (d *Discover) loadBootnodes() {
	// load bootnodes
	errr := make(chan error, len(d.bootnodes))

	for _, p := range d.bootnodes {
		go func(p string) {
			err := d.AddNode(p)
			errr <- err
		}(p)
	}

	for i := 0; i < len(d.bootnodes); i++ {
		<-errr
	}

	// start the initial lookup
	d.active = true

	d.logger.Printf("Finished probing bootnodes")
	d.Lookup()
}

// Close closes the discover
func (d *Discover) Close() error {
	close(d.shutdownCh)
	atomic.StoreInt32(&d.shutdown, 1)
	return d.listener.Close()
}

func (d *Discover) schedule() {
	revalidate := time.NewTicker(d.config.RevalidateInterval)
	lookup := time.NewTicker(d.config.LookupInterval)

	for {
		select {
		case <-lookup.C:
			go d.LookupRandom()

		case <-revalidate.C:
			go d.revalidatePeer()

		case <-d.shutdownCh:
			return
		}
	}
}

func (d *Discover) revalidatePeer() {
	var id peer.ID
	for _, i := range rand.Perm(len(d.table.Buckets)) {
		if bucket := d.table.Buckets[i]; bucket.Len() != 0 {
			id = bucket.Peers()[bucket.Len()-1]
			break
		}
	}

	peer, ok := d.getPeer(id)
	if !ok {
		// log, failed to get peer, weird
	} else {
		d.probeNode(peer)
	}
}

func (d *Discover) NearestPeers() ([]*Peer, error) {
	return d.NearestPeersFromTarget(d.local.Bytes)
}

func (d *Discover) NearestPeersFromTarget(target []byte) ([]*Peer, error) {
	key := peer.ID(hexutil.Encode(target))

	peers := []*Peer{}
	for _, p := range d.table.NearestPeers(kb.ConvertPeerID(key), d.config.BucketSize) {
		peer, ok := d.getPeer(p)
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
func (d *Discover) LookupRandom() ([]*Peer, error) {
	var t [64]byte
	crand.Read(t[:])

	target := []byte{}
	for _, i := range t {
		target = append(target, i)
	}

	return d.LookupTarget(target)
}

// Lookup does a kademlia lookup with the local key as target
func (d *Discover) Lookup() ([]*Peer, error) {
	return d.LookupTarget(d.local.Bytes)
}

// LookupTarget does a kademlia lookup around target
func (d *Discover) LookupTarget(target []byte) ([]*Peer, error) {
	// Only allow one lookup at a time
	if atomic.LoadInt32(&(d.inlookup)) == 1 {
		return nil, nil
	}

	defer func() {
		atomic.StoreInt32(&d.inlookup, 0)
	}()

	if !d.active {
		return []*Peer{}, nil
	}

	visited := map[peer.ID]bool{}

	// initialize the queue
	queue, err := d.NearestPeersFromTarget(target)
	if err != nil {
		return nil, err
	}

	reply := make(chan []*Peer)

	var peer *Peer

	pending := 0

	findNodes := func(p *Peer) {
		pending++
		go func() {
			nodes, err := d.findNodes(p, target)
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
		case <-d.shutdownCh:
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
		p, ok := d.getPeer(id)
		if !ok {
			return nil, fmt.Errorf("")
		}

		discovered = append(discovered, p)
	}

	return discovered, nil
}

// GetPeers return the peers
func (d *Discover) GetPeers() []*Peer {
	peers := []*Peer{}
	for _, peer := range d.nodes {
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

	// create the peer object
	pubkey, err := secp256k1.RecoverPubkey(crypto.Keccak256(sigdata), sig)
	if err != nil {
		return nil, nil, nil, err
	}

	return mac, sigdata, pubkey, nil
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

	peer, err := newPeer(hexutil.Encode(pubkey[1:]), addr, 0)
	if err != nil {
		return nil, err
	}
	peer.Last = &packet.Timestamp
	return peer, nil
}

// HandlePacket handles an incoming udp packet
func (d *Discover) HandlePacket(packet *Packet) error {
	mac, sigdata, pubkey, err := decodePacket(packet.Buf)
	if err != nil {
		return err
	}

	addr, ok := packet.From.(*net.UDPAddr)
	if !ok {
		return fmt.Errorf("expected udp addr")
	}

	peer, err := newPeer(hexutil.Encode(pubkey[1:]), addr, 0)
	if err != nil {
		return err
	}
	peer.Last = &packet.Timestamp

	msgcode, payload := sigdata[0], sigdata[1:]

	if callback, ok := d.getCallback(peer.ID, msgcode); ok {
		callback(payload, &packet.Timestamp)

		// We can also create callbacks for pingPackets that work as notifications
		// but we still have to send the pong
		if msgcode != pingPacket {
			return nil
		}
	}

	switch msgcode {
	case pingPacket:
		return d.handlePingPacket(payload, mac, peer)
	case findnodePacket:
		return d.handleFindNodePacket(payload, peer)
	case neighborsPacket:
		return fmt.Errorf("neighborsPacket not expected")
	case pongPacket:
		return fmt.Errorf("pongPacket not expected %20s %20s %s", peer.addr(), peer.ID, hexutil.Encode(peer.Bytes))
	default:
		return fmt.Errorf("message code %d not found", msgcode)
	}
}

func (d *Discover) getCallback(id peer.ID, code byte) (func([]byte, *time.Time), bool) {
	key := fmt.Sprintf("%s_%v", id, code)
	d.respLock.Lock()
	c, ok := d.handlers[key]
	d.respLock.Unlock()
	return c, ok
}

func (d *Discover) handlePingPacket(payload []byte, mac []byte, peer *Peer) error {
	var req pingRequest
	if err := rlp.DecodeBytes(payload, &req); err != nil {
		panic(err)
	}

	if hasExpired(req.Expiration) {
		return fmt.Errorf("ping: Message has expired")
	}

	reply := pongResponse{
		To:         peer.toRPCEndpoint(),
		ReplyTok:   mac,
		Expiration: uint64(time.Now().Add(20 * time.Second).Unix()),
	}

	d.sendPacket(peer, pongPacket, reply)

	// received a ping, probe back it it has expired
	if d.hasExpired(peer) {
		// go d.probeNode(peer)
		d.sendTask(peer)
	} else {
		d.updatePeer(peer)
	}

	return nil
}

func (d *Discover) hasExpired(p *Peer) bool {
	d.validLock.Lock()
	defer d.validLock.Unlock()

	// If we check hasExpired if the node has not been probed yet
	// i.e. first findNodes query
	if p.Last == nil {
		return true
	}

	receivedTime := p.Last
	p, ok := d.nodes[p.ID]
	if !ok {
		return true
	}

	return p.hasExpired(*receivedTime, d.config.BondExpiration)
}

func (d *Discover) handleFindNodePacket(payload []byte, peer *Peer) error {
	if d.hasExpired(peer) {
		return nil
	}

	var req findNodeRequest
	if err := rlp.DecodeBytes(payload, &req); err != nil {
		return err
	}

	peers, err := d.NearestPeersFromTarget(req.Target)
	if err != nil {
		return err
	}

	// NOTE: number of peers on each requests is fixed to 'maxNeighbors'
	for i := 0; i < len(peers); i += maxNeighbors {
		nodes := []rpcNode{}
		for _, p := range peers[i:min(i+maxNeighbors, len(peers))] {
			nodes = append(nodes, p.toRPCNode())
		}

		err := d.sendPacket(peer, neighborsPacket, &neighborsResponse{
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
func (d *Discover) AddNode(nodeStr string) error {
	node, err := ParseNode(nodeStr)
	if err != nil {
		return err
	}

	id := make([]byte, 64)
	for i := 0; i < len(id); i++ {
		id[i] = node.ID[i]
	}

	peer, err := newPeer(hexutil.Encode(id), &net.UDPAddr{IP: node.IP, Port: int(node.UDP)}, node.TCP)
	if err != nil {
		return err
	}

	d.probeNode(peer)
	return nil
}

func (d *Discover) probeNode(peer *Peer) bool {
	metrics.IncrCounter([]string{"discover", "probe"}, 1.0)

	// Send ping packet
	d.sendPacket(peer, pingPacket, pingRequest{
		Version:    4,
		From:       d.local.toRPCEndpoint(),
		To:         peer.toRPCEndpoint(),
		Expiration: uint64(time.Now().Add(10 * time.Second).Unix()),
	})

	ack := make(chan respMessage)
	d.setHandler(peer.ID, pongPacket, ack, d.config.RespTimeout)

	resp := <-ack

	if resp.Complete {
		peer.Last = resp.Timestamp
		d.updatePeer(peer)

		select {
		case d.EventCh <- peer.Enode():
		default:
		}
		return true
	}

	return false
}

func (d *Discover) getPeer(id peer.ID) (*Peer, bool) {
	d.validLock.Lock()
	defer d.validLock.Unlock()

	p, ok := d.nodes[id]
	return p, ok
}

func (d *Discover) updatePeer(peer *Peer) {
	d.validLock.Lock()
	defer d.validLock.Unlock()

	d.table.Update(peer.ID)

	if p, ok := d.nodes[peer.ID]; ok {
		p.TCP = peer.TCP

		// if already in, update the timestamp
		// only update if there is a timestamp
		if peer.Last != nil {
			p.Last = peer.Last
		}
	} else {
		d.nodes[peer.ID] = peer
	}
}

func (d *Discover) findNodes(peer *Peer, target []byte) ([]*Peer, error) {
	if d.hasExpired(peer) {
		// The connect has expired with the node, lets probe again.
		if !d.probeNode(peer) {
			return nil, fmt.Errorf("failed to probe node")
		}

		// wait for a ping from the peer to ensure we are alive on his side
		ack := make(chan respMessage)
		d.setHandler(peer.ID, pingPacket, ack, d.config.RespTimeout)

		if resp := <-ack; !resp.Complete {
			return nil, fmt.Errorf("We have not received probe back from other peer")
		}

		// sleep a couple of milliseconds to send the other package first
		time.Sleep(100 * time.Millisecond)
	}

	d.sendPacket(peer, findnodePacket, &findNodeRequest{
		Target:     d.local.Bytes,
		Expiration: uint64(time.Now().Add(20 * time.Second).Unix()),
	})

	ack := make(chan respMessage)
	d.setHandler(peer.ID, neighborsPacket, ack, d.config.RespTimeout)

	peers := []*Peer{}
	atLeastOne := false

	for {
		resp := <-ack
		if resp.Complete {
			atLeastOne = true

			var neighbors neighborsResponse
			if err := rlp.DecodeBytes(resp.Payload, &neighbors); err != nil {
				panic(err)
			}

			for _, n := range neighbors.Nodes {
				p, err := n.toPeer()
				if err != nil {
					return nil, err
				}
				peers = append(peers, p)
			}

			if len(peers) == d.config.BucketSize {
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
		d.sendTask(p)
	}

	return peers, nil
}

type respMessage struct {
	Complete  bool
	Payload   []byte
	Timestamp *time.Time
}

func (d *Discover) setHandler(id peer.ID, code byte, ackCh chan respMessage, expiration time.Duration) {
	key := fmt.Sprintf("%s_%v", id, code)

	callback := func(payload []byte, timestamp *time.Time) {
		select {
		case ackCh <- respMessage{true, payload, timestamp}:
		default:
		}
	}

	d.respLock.Lock()
	d.handlers[key] = callback
	d.respLock.Unlock()

	d.timer = time.AfterFunc(expiration, func() {
		d.respLock.Lock()
		delete(d.handlers, key)
		d.respLock.Unlock()

		select {
		case ackCh <- respMessage{false, nil, nil}:
		default:
		}
	})
}

func (d *Discover) sendPacket(peer *Peer, code byte, payload interface{}) error {
	data, err := d.encodePacket(code, payload)
	if err != nil {
		return err
	}

	if _, err := d.WriteTo(data, peer.addr()); err != nil {
		return err
	}

	return nil
}

// WriteTo sends b to addrs with an udp packet
func (d *Discover) WriteTo(b []byte, addr string) (time.Time, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return time.Time{}, err
	}
	_, err = d.listener.WriteTo(b, udpAddr)
	return time.Now(), err
}

func (d *Discover) encodePacket(code byte, payload interface{}) ([]byte, error) {
	b := new(bytes.Buffer)
	b.Write(headSpace)
	b.WriteByte(code)

	if err := rlp.Encode(b, payload); err != nil {
		return nil, err
	}

	packet := b.Bytes()
	sig, err := crypto.Sign(crypto.Keccak256(packet[headSize:]), d.ID)
	if err != nil {
		return nil, err
	}
	copy(packet[macSize:], sig)

	hash := crypto.Keccak256(packet[macSize:])
	copy(packet, hash)

	return packet, nil
}
