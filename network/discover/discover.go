package discover

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/rlp"

	kb "github.com/libp2p/go-libp2p-kbucket"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
)

const (
	nodeIDBytes               = 512 / 8
	defaultBucketSize         = 16
	defaultBondExpiration     = 24 * time.Hour
	defaultRespTimeout        = 10 * time.Second
	defaultRevalidateInterval = 10 * time.Second
	maxNeighbors              = 6
	alpha                     = 3
)

// Config is the discover configuration
type Config struct {
	BucketSize         int
	BondExpiration     time.Duration
	RespTimeout        time.Duration
	RevalidateInterval time.Duration
	Bootnodes          []string
}

// DefaultConfig is the default config
func DefaultConfig() *Config {
	c := &Config{
		BucketSize:         defaultBucketSize,
		BondExpiration:     defaultBondExpiration,
		RespTimeout:        defaultRespTimeout,
		RevalidateInterval: defaultRevalidateInterval,
		Bootnodes:          []string{},
	}
	return c
}

// Peer is the discovery peer
type Peer struct {
	ID      peer.ID
	Bytes   []byte
	UDPAddr *net.UDPAddr
	Last    *time.Time // last time pinged
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
	r.TCP = 0
	return r
}

func newPeer(id string, addr *net.UDPAddr) (*Peer, error) {
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

	return &Peer{peer.ID(id), bytes, addr, nil}, nil
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
	return newPeer(hexutil.Encode(r.ID), &net.UDPAddr{IP: r.IP, Port: int(r.UDP)})
}

type rpcEndpoint struct {
	IP  net.IP // len 4 for IPv4 or 16 for IPv6
	UDP uint16 // for discovery protocol
	TCP uint16 // for RLPx protocol
}

// Packet is used to provide some metadata about incoming packets from peers
// over a packet connection, as well as the packet payload.
type Packet struct {
	Buf       []byte
	From      net.Addr
	Timestamp time.Time
}

// Transport is the UDP transport to send packets
type Transport interface {
	WriteTo([]byte, string) (time.Time, error)
	PacketCh() <-chan *Packet
}

// RoutingTable is the kademlia table
type RoutingTable struct {
	ID         *ecdsa.PrivateKey
	config     *Config
	transport  Transport
	timer      *time.Timer
	handlers   map[string]func(payload []byte, timestamp *time.Time)
	respLock   sync.Mutex
	validLock  sync.Mutex
	table      *kb.RoutingTable
	nodes      map[peer.ID]*Peer
	local      *Peer
	shutdownCh chan bool
}

// NewRoutingTable creates a new routing table
func NewRoutingTable(key *ecdsa.PrivateKey, transport Transport, addr *net.UDPAddr, config *Config) (*RoutingTable, error) {

	pub := &key.PublicKey
	id := hexutil.Encode(elliptic.Marshal(pub.Curve, pub.X, pub.Y)[1:])

	localPeer, err := newPeer(id, addr)
	if err != nil {
		return nil, err
	}

	r := &RoutingTable{
		ID:         key,
		config:     config,
		transport:  transport,
		handlers:   map[string]func(payload []byte, timestamp *time.Time){},
		respLock:   sync.Mutex{},
		validLock:  sync.Mutex{},
		nodes:      map[peer.ID]*Peer{},
		local:      localPeer,
		table:      kb.NewRoutingTable(config.BucketSize, kb.ConvertPeerID(localPeer.ID), 1000*time.Second, peerstore.NewMetrics()),
		shutdownCh: make(chan bool),
	}

	return r, nil
}

// Schedule starts the discovery protocol
func (r *RoutingTable) Schedule() {
	go r.schedule()
}

// Close closes the discover
func (r *RoutingTable) Close() {
	close(r.shutdownCh)
}

func (r *RoutingTable) schedule() {
	for {
		select {
		case packet := <-r.transport.PacketCh():
			go r.handlePacket(packet)

		case <-time.After(r.config.RevalidateInterval):
			go r.revalidatePeer()

		case <-r.shutdownCh:
			return
		}
	}
}

func (r *RoutingTable) handlePacket(packet *Packet) {
	if err := r.HandlePacket(packet); err != nil {
		fmt.Printf("ERR: %v\n", err)
	}
}

func (r *RoutingTable) revalidatePeer() {
	var id peer.ID
	for _, i := range rand.Perm(len(r.table.Buckets)) {
		if bucket := r.table.Buckets[i]; bucket.Len() != 0 {
			id = bucket.Peers()[bucket.Len()-1]
			break
		}
	}

	peer, ok := r.getPeer(id)
	if !ok {
		// log, failed to get peer, weird
	} else {
		r.probeNode(peer)
	}
}

func (r *RoutingTable) nearestPeers(target []byte) ([]*Peer, error) {
	key := peer.ID(hexutil.Encode(target))

	peers := []*Peer{}
	for _, p := range r.table.NearestPeers(kb.ConvertPeerID(key), r.config.BucketSize) {
		peer, ok := r.getPeer(p)
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

// Lookup does a kademlia lookup with the local key as target
func (r *RoutingTable) Lookup() ([]*Peer, error) {
	return r.LookupTarget(r.local.Bytes)
}

// LookupTarget does a kademlia lookup around target
func (r *RoutingTable) LookupTarget(target []byte) ([]*Peer, error) {
	fmt.Println("@@@@@@@@@@@@@@@@@@@@@@@ LOOKUP @@@@@@@@@@@@@@@@@@@@@@@@")

	visited := map[peer.ID]bool{}

	// initialize the queue
	queue, err := r.nearestPeers(r.local.Bytes)
	if err != nil {
		return nil, err
	}

	reply := make(chan []*Peer)

	var peer *Peer

	pending := 0

	findNodes := func(p *Peer) {
		fmt.Println(len(queue))

		pending++
		go func() {
			nodes, err := r.findNodes(p, target)
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
		nodes := <-reply
		pending--

		v := 0
		for _, p := range nodes {
			if _, ok := visited[p.ID]; !ok {
				visited[p.ID] = true
				queue = append(queue, p)
				v++
			}
		}

		fmt.Printf("Returned nodes (%d, %d): %v\n", len(nodes), v, nodes)

		for pending < alpha && len(queue) != 0 {
			peer, queue = queue[0], queue[1:]
			findNodes(peer)
		}
	}

	discovered := []*Peer{}
	for id := range visited {
		p, ok := r.getPeer(id)
		if !ok {
			return nil, fmt.Errorf("")
		}

		discovered = append(discovered, p)
	}

	return discovered, nil
}

// GetPeers return the peers
func (r *RoutingTable) GetPeers() []*Peer {
	peers := []*Peer{}
	for _, peer := range r.nodes {
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

// TODO. what to do with this, i'd like to remove it
func decodePeerFromPacket(packet *Packet) (*Peer, error) {
	_, _, pubkey, err := decodePacket(packet.Buf)
	if err != nil {
		return nil, err
	}

	addr, ok := packet.From.(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("expected udp addr")
	}

	peer, err := newPeer(hexutil.Encode(pubkey[1:]), addr)
	if err != nil {
		return nil, err
	}
	peer.Last = &packet.Timestamp
	return peer, nil
}

// HandlePacket handles an incoming udp packet
func (r *RoutingTable) HandlePacket(packet *Packet) error {
	mac, sigdata, pubkey, err := decodePacket(packet.Buf)
	if err != nil {
		return err
	}

	addr, ok := packet.From.(*net.UDPAddr)
	if !ok {
		return fmt.Errorf("expected udp addr")
	}

	peer, err := newPeer(hexutil.Encode(pubkey[1:]), addr)
	if err != nil {
		return err
	}
	peer.Last = &packet.Timestamp

	msgcode, payload := sigdata[0], sigdata[1:]

	if callback, ok := r.getCallback(peer.ID, msgcode); ok {
		callback(payload, &packet.Timestamp)

		// We can also create callbacks for pingPackets that work as notifications
		// but we still have to send the pong
		if msgcode != pingPacket {
			return nil
		}
	}

	switch msgcode {
	case pingPacket:
		return r.handlePingPacket(payload, mac, peer)
	case findnodePacket:
		return r.handleFindNodePacket(payload, peer)
	case neighborsPacket:
		return fmt.Errorf("neighborsPacket not expected")
	case pongPacket:
		return fmt.Errorf("pongPacket not expected %20s %20s %s", peer.addr(), peer.ID, hexutil.Encode(peer.Bytes))
	default:
		return fmt.Errorf("message code %d not found", msgcode)
	}
}

func (r *RoutingTable) getCallback(id peer.ID, code byte) (func([]byte, *time.Time), bool) {
	key := fmt.Sprintf("%s_%v", id, code)
	r.respLock.Lock()
	c, ok := r.handlers[key]
	r.respLock.Unlock()
	return c, ok
}

func (r *RoutingTable) handlePingPacket(payload []byte, mac []byte, peer *Peer) error {
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

	r.sendPacket(peer, pongPacket, reply)

	// received a ping, probe back it it has expired
	if r.hasExpired(peer) {
		go r.probeNode(peer)
	} else {
		r.updatePeer(peer)
	}

	return nil
}

func (r *RoutingTable) hasExpired(p *Peer) bool {
	r.validLock.Lock()
	defer r.validLock.Unlock()

	// If we check hasExpired if the node has not been probed yet
	// i.e. first findNodes query
	if p.Last == nil {
		return true
	}

	receivedTime := p.Last
	p, ok := r.nodes[p.ID]
	if !ok {
		return true
	}

	return p.hasExpired(*receivedTime, r.config.BondExpiration)
}

// TODO, build nearestPeers wrapper that returns peer objects

// TODO. if we probe and findnode all at the same time we might end up
// not working becasues the other is not synced yet

func (r *RoutingTable) handleFindNodePacket(payload []byte, peer *Peer) error {
	if r.hasExpired(peer) {
		return nil
	}

	var req findNodeRequest
	if err := rlp.DecodeBytes(payload, &req); err != nil {
		return err
	}

	peers, err := r.nearestPeers(req.Target)
	if err != nil {
		return err
	}

	// NOTE: number of peers on each requests is fixed to 'maxNeighbors'
	for i := 0; i < len(peers); i += maxNeighbors {
		nodes := []rpcNode{}
		for _, p := range peers[i:min(i+maxNeighbors, len(peers))] {
			nodes = append(nodes, p.toRPCNode())
		}

		err := r.sendPacket(peer, neighborsPacket, &neighborsResponse{
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

// AddNode adds a new node to the discover process
func (r *RoutingTable) AddNode(nodeStr string) error {
	node, err := ParseNode(nodeStr)
	if err != nil {
		return err
	}

	id := make([]byte, 64)
	for i := 0; i < len(id); i++ {
		id[i] = node.ID[i]
	}

	peer, err := newPeer(hexutil.Encode(id), &net.UDPAddr{IP: node.IP, Port: int(node.UDP)})
	if err != nil {
		return err
	}

	go r.probeNode(peer)
	return nil
}

func (r *RoutingTable) probeNode(peer *Peer) bool {
	// fmt.Printf("Probe peer %25s %20s %s\n", peer.Addr(), peer.ID, hexutil.Encode(peer.Bytes))

	// Send ping packet
	r.sendPacket(peer, pingPacket, pingRequest{
		Version:    4,
		From:       r.local.toRPCEndpoint(),
		To:         peer.toRPCEndpoint(),
		Expiration: uint64(time.Now().Add(10 * time.Second).Unix()),
	})

	ack := make(chan respMessage)
	r.setHandler(peer.ID, pongPacket, ack, r.config.RespTimeout)

	// fmt.Printf("-- handler set for %s --\n", peer.Addr())

	resp := <-ack

	// TODO. add time from the packet here
	if resp.Complete {

		peer.Last = resp.Timestamp
		r.updatePeer(peer)

		// fmt.Printf("Probe worked %s\n", peer.Addr())
		return true
	} else {
		// fmt.Printf("probe failed %s\n", peer.Addr())
	}

	return false
}

func (r *RoutingTable) getPeer(id peer.ID) (*Peer, bool) {
	r.validLock.Lock()
	defer r.validLock.Unlock()

	p, ok := r.nodes[id]
	return p, ok
}

func (r *RoutingTable) updatePeer(peer *Peer) {
	r.validLock.Lock()
	defer r.validLock.Unlock()

	r.table.Update(peer.ID)

	if p, ok := r.nodes[peer.ID]; ok {
		// if already in, update the timestamp
		// only update if there is a timestamp
		if peer.Last != nil {
			p.Last = peer.Last
		}
	} else {
		r.nodes[peer.ID] = peer
	}
}

func (r *RoutingTable) findNodes(peer *Peer, target []byte) ([]*Peer, error) {
	if r.hasExpired(peer) {
		// The connect has expired with the node, lets probe again.
		if !r.probeNode(peer) {
			return nil, fmt.Errorf("failed to probe node")
		}

		fmt.Printf("- here %s -\n", peer.ID)

		// wait for a ping from the peer to ensure we are alive on his side
		ack := make(chan respMessage)
		r.setHandler(peer.ID, pingPacket, ack, r.config.RespTimeout)

		if resp := <-ack; !resp.Complete {
			fmt.Printf("- ended bad %s -\n", peer.ID)
			// return nil, fmt.Errorf("We have not received probe back from other peer")
		} else {
			fmt.Printf("- done %s -\n", peer.ID)
		}

		// sleep a couple of milliseconds to send the other package first
		time.Sleep(100 * time.Millisecond)
	}

	r.sendPacket(peer, findnodePacket, &findNodeRequest{
		Target:     r.local.Bytes,
		Expiration: uint64(time.Now().Add(20 * time.Second).Unix()),
	})

	ack := make(chan respMessage)
	r.setHandler(peer.ID, neighborsPacket, ack, r.config.RespTimeout)

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

			if len(peers) == r.config.BucketSize {
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
		r.updatePeer(p)
	}

	return peers, nil
}

type respMessage struct {
	Complete  bool
	Payload   []byte
	Timestamp *time.Time
}

func (r *RoutingTable) setHandler(id peer.ID, code byte, ackCh chan respMessage, expiration time.Duration) {
	key := fmt.Sprintf("%s_%v", id, code)

	callback := func(payload []byte, timestamp *time.Time) {
		select {
		case ackCh <- respMessage{true, payload, timestamp}:
		default:
		}
	}

	r.respLock.Lock()
	r.handlers[key] = callback
	r.respLock.Unlock()

	r.timer = time.AfterFunc(expiration, func() {
		r.respLock.Lock()
		delete(r.handlers, key)
		r.respLock.Unlock()

		select {
		case ackCh <- respMessage{false, nil, nil}:
		default:
		}
	})
}

func (r *RoutingTable) sendPacket(peer *Peer, code byte, payload interface{}) error {
	data, err := r.encodePacket(code, payload)
	if err != nil {
		return err
	}

	if _, err := r.transport.WriteTo(data, peer.addr()); err != nil {
		return err
	}

	return nil
}

func (r *RoutingTable) encodePacket(code byte, payload interface{}) ([]byte, error) {
	b := new(bytes.Buffer)
	b.Write(headSpace)
	b.WriteByte(code)

	if err := rlp.Encode(b, payload); err != nil {
		return nil, err
	}

	packet := b.Bytes()
	sig, err := crypto.Sign(crypto.Keccak256(packet[headSize:]), r.ID)
	if err != nil {
		return nil, err
	}
	copy(packet[macSize:], sig)

	hash := crypto.Keccak256(packet[macSize:])
	copy(packet, hash)

	return packet, nil
}
