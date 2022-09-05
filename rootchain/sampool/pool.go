package sampool

import "github.com/0xPolygon/polygon-edge/rootchain"

//	Callback used for determining if there is a valid
//	number of SAM messages
type QuorumFunc func(uint64) bool

//	Verifies hash and signature of a SAM
type Verifier interface {
	VerifyHash(msg rootchain.SAM) error
	VerifySignature(msg rootchain.SAM) error
}

type SAMPool struct {
}

func (s *SAMPool) AddMessage(msg rootchain.SAM) error {
	//	verify message hash

	//	verify message signature

	//	add message

	return nil
}

func (s *SAMPool) Prune(index uint64) {

}

//	TODO: Peek or Pop might be redundant

func (s *SAMPool) Peek() rootchain.VerifiedSAM {
	return nil
}

func (s *SAMPool) Pop() rootchain.VerifiedSAM {
	return nil
}
