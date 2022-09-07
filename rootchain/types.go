package rootchain

import "math"

type PayloadType uint8

const (
	ValidatorSetPayloadType PayloadType = iota
)

const (
	LatestRootchainBlockNumber uint64 = math.MaxUint64 // Special value, meaning the latest block on the rootchain
)

type Payload interface {
	Get() (PayloadType, []byte)
}

type VerifiedSAM []SAM

func (v VerifiedSAM) Signatures() (signatures [][]byte) {
	signatures = make([][]byte, len(v))

	for index, m := range v {
		signatures[index] = m.Signature
	}

	return
}
