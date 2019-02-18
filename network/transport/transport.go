package transport

import "net"

// TODO, open needs an input

// MuxSession multiplexes different streams inside
// the same session
type MuxSession interface {
	Open() (net.Conn, error)
}
