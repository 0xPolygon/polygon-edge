package validators

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type ValidatorSet interface {
	Len() int
	Equal(ValidatorSet) bool
	Copy() ValidatorSet
	GetAddress(int) types.Address
	Index(types.Address) int
	Includes(types.Address) bool
	CalcProposer(round uint64, lastProposer types.Address) types.Address
	Add(addr types.Address)
	Del(target types.Address)
	MaxFaultyNodes() int
	QuorumSize() int
	MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error
}
