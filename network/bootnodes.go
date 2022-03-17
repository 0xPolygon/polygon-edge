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
	bootnodeConnCount int32
}

// isBootnode checks if the node ID belongs to a set bootnode
func (bw *bootnodesWrapper) isBootnode(nodeID peer.ID) bool {
	_, ok := bw.bootnodesMap[nodeID]

	return ok
}

// getBootnodeConnCount loads the bootnode connection count [Thread safe]
func (bw *bootnodesWrapper) getBootnodeConnCount() int32 {
	return atomic.LoadInt32(&bw.bootnodeConnCount)
}

// increaseBootnodeConnCount increases the bootnode connection count by delta [Thread safe]
func (bw *bootnodesWrapper) increaseBootnodeConnCount(delta int32) {
	atomic.AddInt32(&bw.bootnodeConnCount, delta)
}

// getBootnodes gets all the bootnodes
func (bw *bootnodesWrapper) getBootnodes() []*peer.AddrInfo {
	return bw.bootnodeArr
}

// getBootnodeCount returns the number of set bootnodes
func (bw *bootnodesWrapper) getBootnodeCount() int {
	return len(bw.bootnodeArr)
}
