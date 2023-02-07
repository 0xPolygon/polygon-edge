package snapshot

import (
	"context"
	"math"

	"github.com/0xPolygon/polygon-edge/command/helper"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
)

const (
	numberFlag = "number"
)

var (
	params = &snapshotParams{}
)

type snapshotParams struct {
	blockNumber uint64

	snapshot *ibftOp.Snapshot
}

func (p *snapshotParams) initSnapshot(grpcAddress string) error {
	ibftClient, err := helper.GetIBFTOperatorClientConnection(grpcAddress)
	if err != nil {
		return err
	}

	snapshot, err := ibftClient.GetSnapshot(
		context.Background(),
		p.getSnapshotRequest(),
	)
	if err != nil {
		return err
	}

	p.snapshot = snapshot

	return nil
}

func (p *snapshotParams) getSnapshotRequest() *ibftOp.SnapshotReq {
	req := &ibftOp.SnapshotReq{
		Latest: true,
	}

	if p.blockNumber != math.MaxUint64 {
		req.Latest = false
		req.Number = p.blockNumber
	}

	return req
}
