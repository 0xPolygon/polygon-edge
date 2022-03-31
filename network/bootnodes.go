package network

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"sync/atomic"
)

type bootnodesWrapper struct {
	// bootnodeArr is the array that contains all the bootnode addresses
	bootnodeArr []*peer.AddrInfo

	// bootnodesMap is a map used for quick bootnode lookup
	bootnodesMap map[peer.ID]*peer.AddrInfo

	// bootnodeConnCount is an atomic value that keeps track
	// of the number of bootnode connections
	bootnodeConnCount int64
}

// isBootnode checks if the node ID belongs to a set bootnode
func (bw *bootnodesWrapper) isBootnode(nodeID peer.ID) bool {
	_, ok := bw.bootnodesMap[nodeID]

	return ok
}

// getBootnodeConnCount loads the bootnode connection count [Thread safe]
func (bw *bootnodesWrapper) getBootnodeConnCount() int64 {
	return atomic.LoadInt64(&bw.bootnodeConnCount)
}

// increaseBootnodeConnCount increases the bootnode connection count by delta [Thread safe]
func (bw *bootnodesWrapper) increaseBootnodeConnCount(delta int64) {
	atomic.AddInt64(&bw.bootnodeConnCount, delta)
}

// getBootnodes gets all the bootnodes
func (bw *bootnodesWrapper) getBootnodes() []*peer.AddrInfo {
	return bw.bootnodeArr
}

// getBootnodeCount returns the number of set bootnodes
func (bw *bootnodesWrapper) getBootnodeCount() int {
	return len(bw.bootnodeArr)
}

// hasBootnodes checks if any bootnodes are set [Thread safe]
func (bw *bootnodesWrapper) hasBootnodes() bool {
	return bw.getBootnodeCount() > 0
}
