package network

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/boltdb/bolt"
)

// PeerStore stores peers id
type PeerStore interface {
	Load() ([]string, error)
	Update(addr string, status Status) error
	Close() error
}

// NoopPeerStore is a peerstore that does not store peers
type NoopPeerStore struct {
}

// Load implements the PeerStore interface
func (i *NoopPeerStore) Load() ([]string, error) {
	return nil, nil
}

// Update implements the PeerStore interface
func (i *NoopPeerStore) Update(addr string, status Status) error {
	return nil
}

// Close implements the PeerStore interface
func (i *NoopPeerStore) Close() error {
	return nil
}

type peerEntry struct {
	Status Status
}

// JSONPeerStore stores the peers locally in json format
type JSONPeerStore struct {
	path  string
	peers map[string]*peerEntry
}

var _ PeerStore = (*JSONPeerStore)(nil)

// NewJSONPeerStore creates a json peerstore
func NewJSONPeerStore(path string) *JSONPeerStore {
	return &JSONPeerStore{
		path:  filepath.Join(path, "peers.json"),
		peers: map[string]*peerEntry{},
	}
}

// Update implements the PeerStore interface
func (p *JSONPeerStore) Update(addr string, status Status) error {
	if pp, ok := p.peers[addr]; ok {
		pp.Status = status
	} else {
		p.peers[addr] = &peerEntry{status}
	}
	return nil
}

// Load implements the PeerStore interface
func (p *JSONPeerStore) Load() ([]string, error) {
	if _, err := os.Stat(p.path); os.IsNotExist(err) {
		return []string{}, nil
	}

	data, err := ioutil.ReadFile(p.path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &p.peers); err != nil {
		return nil, err
	}

	addrs := []string{}
	for addr := range p.peers {
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

// Close implements the PeerStore interface
func (p *JSONPeerStore) Close() error {
	data, err := json.MarshalIndent(p.peers, "", "    ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(p.path, data, 0644); err != nil {
		return err
	}
	return nil
}

// BoltDBPeerStore stores the peers locally in boltdb
type BoltDBPeerStore struct {
	db *bolt.DB
}

var _ PeerStore = (*BoltDBPeerStore)(nil)

var (
	boltDBpeersBucket = []byte("peers")
)

// NewBoltDBPeerStore creates a boltdb peerstore
func NewBoltDBPeerStore(dir string) (*BoltDBPeerStore, error) {
	db, err := bolt.Open(filepath.Join(dir, "peers.db"), 0600, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(txn *bolt.Tx) error {
		_, err := txn.CreateBucketIfNotExists(boltDBpeersBucket)
		return err
	})
	if err != nil {
		return nil, err
	}

	p := &BoltDBPeerStore{db}
	return p, nil
}

// Load implements the PeerStore interface
func (p *BoltDBPeerStore) Load() ([]string, error) {
	peers := []string{}

	err := p.db.View(func(txn *bolt.Tx) error {
		return txn.Bucket(boltDBpeersBucket).ForEach(func(k, v []byte) error {
			peers = append(peers, string(k))
			return nil
		})
	})
	return peers, err
}

// Update implements the PeerStore interface
func (p *BoltDBPeerStore) Update(peer string, status Status) error {
	return p.db.Update(func(txn *bolt.Tx) error {
		return txn.Bucket(boltDBpeersBucket).Put([]byte(peer), nil)
	})
}

// Close implements the PeerStore interface
func (p *BoltDBPeerStore) Close() error {
	return p.db.Close()
}
