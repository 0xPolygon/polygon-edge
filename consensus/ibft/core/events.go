package core

import (
	"github.com/0xPolygon/minimal/consensus/ibft"
)

type backlogEvent struct {
	src ibft.Validator
	msg *message
}

type timeoutEvent struct{}
