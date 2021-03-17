package ibft

import (
	"math/big"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
)

// represetns core/roundstate.go

type state struct {
	round    *big.Int
	sequence *big.Int
}

func (s *state) validateView(view *proto.View) error {
	return nil
}

type messageSet struct {
}
