package protocol

import "context"

// Handler is the handler of the msg for the protocol
type Handler interface {
	Init() error
	Close() error
}

// Protocol is a specification of an etheruem protocol
type Protocol struct {
	Name    string
	Version uint
	Length  uint64
}

const (
	// ETH is the name of the ethereum protocol
	ETH = "eth"
	// PAR is the name of the parity protocol
	PAR = "par"
)

// ETH63 is the Fast synchronization protocol
var ETH63 = Protocol{
	Name:    ETH,
	Version: 63,
	Length:  17,
}

// ETH62 is the other
var ETH62 = Protocol{
	Name:    ETH,
	Version: 62,
	Length:  8,
}

// PAR1 is the parity protocol
var PAR1 = Protocol{
	Name:    PAR,
	Version: 1,
	Length:  21,
}

// Factory is the factory method to create the protocol (TODO)
type Factory func(context.Context)
