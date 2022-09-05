package sampool

import "github.com/0xPolygon/polygon-edge/rootchain"

type SAMPool struct {
}

func (s *SAMPool) AddMessage(msg rootchain.SAM) error {
	return nil
}

func (s *SAMPool) Prune(index uint64) {

}

func (s *SAMPool) Peek() rootchain.VerifiedSAM {
	return nil
}

func (s *SAMPool) Pop() rootchain.VerifiedSAM {
	return nil
}
