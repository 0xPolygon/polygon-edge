package dial

import "github.com/libp2p/go-libp2p-core/peer"

type DialTask struct {
	index int

	// info of the task
	addr *peer.AddrInfo

	// priority of the task (the higher the better)
	priority uint64
}

// GetTaskInfo returns the peer information associated with the dial
func (dt *DialTask) GetTaskInfo() *peer.AddrInfo {
	return dt.addr
}
