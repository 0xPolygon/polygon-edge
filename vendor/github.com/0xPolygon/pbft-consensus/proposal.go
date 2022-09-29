package pbft

import (
	"bytes"
	"time"
)

// Proposal is the default proposal
type Proposal struct {
	// Data is an arbitrary set of data to approve in consensus
	Data []byte

	// Time is the time to create the proposal
	Time time.Time

	// Hash is the digest of the data to seal
	Hash []byte
}

// Equal compares whether two proposals have the same hash
func (p *Proposal) Equal(pp *Proposal) bool {
	return bytes.Equal(p.Hash, pp.Hash)
}

// Copy makes a copy of the Proposal
func (p *Proposal) Copy() *Proposal {
	pp := new(Proposal)
	*pp = *p

	pp.Data = append([]byte{}, p.Data...)
	pp.Hash = append([]byte{}, p.Hash...)

	return pp
}
