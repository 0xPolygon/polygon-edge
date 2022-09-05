package sampool

import "github.com/0xPolygon/polygon-edge/rootchain"

//	Verifies hash and signature of a SAM
type Verifier interface {
	VerifyHash(rootchain.SAM) error
	VerifySignature(rootchain.SAM) error

	Quorum(uint64) bool
}

type SAMPool struct {
	verifier Verifier
}

func New(verifier Verifier) *SAMPool {
	return &SAMPool{
		verifier: verifier,
	}
}

func (s *SAMPool) AddMessage(msg rootchain.SAM) error {
	//	verify message hash
	if err := s.verifier.VerifyHash(msg); err != nil {
		return err
	}

	//	verify message signature
	if err := s.verifier.VerifySignature(msg); err != nil {
		return err
	}

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
