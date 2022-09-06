package rootchain

type PayloadType uint8

const (
	ValidatorSetPayloadType PayloadType = iota
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
