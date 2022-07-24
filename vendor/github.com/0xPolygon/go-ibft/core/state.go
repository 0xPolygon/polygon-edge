package core

import (
	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
	"sync"
)

type stateType uint8

const (
	newRound stateType = iota
	prepare
	commit
	fin
)

func (s stateType) String() (str string) {
	switch s {
	case newRound:
		str = "new round"
	case prepare:
		str = "prepare"
	case commit:
		str = "commit"
	case fin:
		str = "fin"
	}

	return
}

type state struct {
	sync.RWMutex

	//	current view (sequence, round)
	view *proto.View

	// latestPC is the latest prepared certificate
	latestPC *proto.PreparedCertificate

	// latestPreparedProposedBlock is the block
	// for which Q(N)-1 PREPARE messages were received
	latestPreparedProposedBlock []byte

	//	accepted block proposal for current round
	proposalMessage *proto.Message

	//	validated commit seals
	seals [][]byte

	//	flags for different states
	roundStarted bool

	name stateType
}

func (s *state) getView() *proto.View {
	s.RLock()
	defer s.RUnlock()

	return &proto.View{
		Height: s.view.Height,
		Round:  s.view.Round,
	}
}

func (s *state) clear(height uint64) {
	s.Lock()
	defer s.Unlock()

	s.seals = nil
	s.roundStarted = false
	s.name = newRound
	s.proposalMessage = nil
	s.latestPC = nil
	s.latestPreparedProposedBlock = nil

	s.view = &proto.View{
		Height: height,
		Round:  0,
	}
}

func (s *state) getLatestPC() *proto.PreparedCertificate {
	s.RLock()
	defer s.RUnlock()

	return s.latestPC
}

func (s *state) setLatestPC(certificate *proto.PreparedCertificate) {
	s.Lock()
	defer s.Unlock()

	s.latestPC = certificate
}

func (s *state) getLatestPreparedProposedBlock() []byte {
	s.RLock()
	defer s.RUnlock()

	return s.latestPreparedProposedBlock
}

func (s *state) setLatestPPB(block []byte) {
	s.Lock()
	defer s.Unlock()

	s.latestPreparedProposedBlock = block
}

func (s *state) getProposalMessage() *proto.Message {
	s.RLock()
	defer s.RUnlock()

	return s.proposalMessage
}

func (s *state) getProposalHash() []byte {
	s.RLock()
	defer s.RUnlock()

	return messages.ExtractProposalHash(s.proposalMessage)
}

func (s *state) setProposalMessage(proposalMessage *proto.Message) {
	s.Lock()
	defer s.Unlock()

	s.proposalMessage = proposalMessage
}

func (s *state) getRound() uint64 {
	s.RLock()
	defer s.RUnlock()

	return s.view.Round
}

func (s *state) isRoundStarted() bool {
	s.RLock()
	defer s.RUnlock()

	return s.roundStarted
}

func (s *state) getHeight() uint64 {
	s.RLock()
	defer s.RUnlock()

	return s.view.Height
}

func (s *state) getProposal() []byte {
	s.RLock()
	defer s.RUnlock()

	if s.proposalMessage != nil {
		return messages.ExtractProposal(s.proposalMessage)
	}

	return nil
}

func (s *state) getCommittedSeals() [][]byte {
	s.RLock()
	defer s.RUnlock()

	return s.seals
}

func (s *state) getStateName() stateType {
	s.RLock()
	defer s.RUnlock()

	return s.name
}

func (s *state) changeState(name stateType) {
	s.Lock()
	defer s.Unlock()

	s.name = name
}

func (s *state) setRoundStarted(started bool) {
	s.Lock()
	defer s.Unlock()

	s.roundStarted = started
}

func (s *state) setView(view *proto.View) {
	s.Lock()
	defer s.Unlock()

	s.view = view
}

func (s *state) setCommittedSeals(seals [][]byte) {
	s.Lock()
	defer s.Unlock()

	s.seals = seals
}
