package polybft

import (
	"github.com/0xPolygon/polygon-edge/types"
)

// TaskHook is a lifecycle hook into the life cycle of a block.
type TaskHook interface {
	Name() string
}

type PostBlockRequest struct {
	Header *types.Header
}

type TaskPostBlockHook interface {
	TaskHook

	// PostBlock is called after a block is included
	// in the chain
	PostBlock(*PostBlockRequest) error
}

type PostEpochRequest struct {
	Header *types.Header
}

type TaskPostEpochHook interface {
	TaskHook

	// PostEpoch is called after the end of epoch block
	// is included in the chain
	PostEpoch(*PostEpochRequest) error
}

// hooks is the list of available hooks in polybft
type hooks struct {
	txpool *txpoolHook

	// list is an ordered list of the hooks which represents
	// its execution priority
	list []TaskHook
}
