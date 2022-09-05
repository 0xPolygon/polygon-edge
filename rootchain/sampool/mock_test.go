package sampool

import "github.com/0xPolygon/polygon-edge/rootchain"

type mockVerifier struct {
	verifyHash      func(rootchain.SAM) error
	verifySignature func(rootchain.SAM) error
	quorumFunc      func(uint64) bool
}

func (m mockVerifier) VerifyHash(msg rootchain.SAM) error {
	if m.verifyHash == nil {
		panic("callback not implemented: VerifyHash")
	}

	return m.verifyHash(msg)
}

func (m mockVerifier) VerifySignature(msg rootchain.SAM) error {
	if m.verifySignature == nil {
		panic("callback not implemented: VerifySignature")
	}

	return m.verifySignature(msg)
}

func (m mockVerifier) Quorum(n uint64) bool {
	if m.quorumFunc == nil {
		panic("callback not implemented: Quorum")
	}

	return m.quorumFunc(n)
}
