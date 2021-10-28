package ibft

import (
	"context"

	"github.com/0xPolygon/polygon-sdk/consensus/ibft/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

type operator struct {
	ibft *Ibft

	proto.UnimplementedIbftOperatorServer
}

// Status returns the status of the IBFT client
func (o *operator) Status(ctx context.Context, req *empty.Empty) (*proto.IbftStatusResp, error) {
	resp := &proto.IbftStatusResp{
		Key: o.ibft.validatorKeyAddr.String(),
	}

	return resp, nil
}

// GetSnapshot returns the snapshot, based on the passed in request
func (o *operator) GetSnapshot(ctx context.Context, req *proto.SnapshotReq) (*proto.Snapshot, error) {
	var snap *Snapshot
	var err error

	if req.Latest {
		snap, err = o.ibft.getLatestSnapshot()
	} else {
		snap, err = o.ibft.getSnapshot(req.Number)
	}
	if err != nil {
		return nil, err
	}
	resp := snap.ToProto()

	return resp, nil
}
