package ibft2

import (
	"context"
	"sync"

	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

type operator struct {
	i *Ibft2

	candidatesLock sync.Mutex
	candidates     []*proto.Candidate

	proto.UnimplementedOperatorServer
}

func (o *operator) GetSnapshot(ctx context.Context, req *proto.SnapshotReq) (*proto.Snapshot, error) {
	snap, err := o.i.getSnapshot(req.Number)
	if err != nil {
		return nil, err
	}
	resp := snap.ToProto()
	return resp, nil
}

func (o *operator) Propose(ctx context.Context, req *proto.Candidate) (*empty.Empty, error) {
	return nil, nil
}

func (o *operator) Candidates(ctx context.Context, req *empty.Empty) (*proto.CandidatesResp, error) {
	o.candidatesLock.Lock()
	defer o.candidatesLock.Unlock()

	resp := &proto.CandidatesResp{}
	return resp, nil
}
