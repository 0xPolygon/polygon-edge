package network

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

type peerEntry struct {
	Status Status
}

// PeerStore stores the peers locally in json format
type PeerStore struct {
	path  string
	peers map[string]*peerEntry
}

func NewPeerStore(path string) *PeerStore {
	return &PeerStore{
		path:  path,
		peers: map[string]*peerEntry{},
	}
}

func (p *PeerStore) Update(addr string, status Status) {
	if pp, ok := p.peers[addr]; ok {
		pp.Status = status
	} else {
		p.peers[addr] = &peerEntry{status}
	}
}

func (p *PeerStore) Load() []string {
	if _, err := os.Stat(peersFile); os.IsNotExist(err) {
		return []string{}
	}

	data, err := ioutil.ReadFile(p.path)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(data, &p.peers); err != nil {
		panic(err)
	}

	addrs := []string{}
	for addr := range p.peers {
		addrs = append(addrs, addr)
	}
	return addrs
}

func (p *PeerStore) Save() error {
	data, err := json.MarshalIndent(p.peers, "", "    ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(p.path, data, 0644); err != nil {
		return err
	}
	return nil
}
