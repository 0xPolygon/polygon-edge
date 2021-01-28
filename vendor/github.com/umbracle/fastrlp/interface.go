package fastrlp

type Marshaler interface {
	MarshalRLP() []byte
	MarshalRLPTo(dst []byte) []byte
	MarshalRLPWith(ar *Arena) *Value
}

type Unmarshaler interface {
	UnmarshalRLP(buf []byte) error
	UnmarshalRLPFrom(p *Parser, v *Value) error
}

type Object interface {
	Marshaler
	Unmarshaler
}
