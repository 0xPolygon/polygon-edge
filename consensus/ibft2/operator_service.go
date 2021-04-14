package ibft2

import (
	"context"
	"fmt"
	"sync"

	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/empty"
)

type operator struct {
	i *Ibft2

	candidatesLock sync.Mutex
	candidates     []*proto.Candidate

	proto.UnimplementedIbftOperatorServer
}

func (o *operator) Status(ctx context.Context, req *empty.Empty) (*proto.IbftStatusResp, error) {
	resp := &proto.IbftStatusResp{
		Key: o.i.validatorKeyAddr.String(),
	}
	return resp, nil
}

func (o *operator) getNextCandidate(snap *Snapshot) *proto.Candidate {
	// TODO: Test (we do not vote twice and items are removed after being promoted)
	o.candidatesLock.Lock()
	defer o.candidatesLock.Unlock()

	// first, we need to remove any candidates that have already been
	// selected as validators
	for i := 0; i < len(o.candidates); i++ {
		addr := types.StringToAddress(o.candidates[i].Address)
		if snap.Set.Includes(addr) {
			o.candidates = append(o.candidates[:i], o.candidates[i+1:]...)
			i--
		}
	}

	var candidate *proto.Candidate

	// now pick the first candidate that we have not voted yet
	for _, c := range o.candidates {
		addr := types.StringToAddress(c.Address)

		count := snap.Count(func(v *Vote) bool {
			return v.Address == addr && v.Validator == o.i.validatorKeyAddr
		})
		if count == 0 {
			candidate = c
		}
	}
	return candidate
}

func (o *operator) GetSnapshot(ctx context.Context, req *proto.SnapshotReq) (*proto.Snapshot, error) {
	var snap *Snapshot
	var err error

	if req.Latest {
		snap, err = o.i.getLatestSnapshot()
	} else {
		snap, err = o.i.getSnapshot(req.Number)
	}
	if err != nil {
		return nil, err
	}
	resp := snap.ToProto()
	return resp, nil
}

func (o *operator) Propose(ctx context.Context, req *proto.Candidate) (*empty.Empty, error) {
	var addr types.Address
	if err := addr.UnmarshalText([]byte(req.Address)); err != nil {
		return nil, err
	}

	// check if the candidate is already there
	o.candidatesLock.Lock()
	defer o.candidatesLock.Unlock()

	for _, c := range o.candidates {
		if c.Address == req.Address {
			return nil, fmt.Errorf("already a candidate")
		}
	}

	snap, err := o.i.getLatestSnapshot()
	if err != nil {
		return nil, err
	}
	// safe checks
	if req.Auth {
		if snap.Set.Includes(addr) {
			return nil, fmt.Errorf("is already a validator")
		}
	}
	if !req.Auth {
		if !snap.Set.Includes(addr) {
			return nil, fmt.Errorf("cannot remove a validator if not in snapshot")
		}
	}

	// check if we have already vote for this candidate
	count := snap.Count(func(v *Vote) bool {
		return v.Address == addr && v.Validator == o.i.validatorKeyAddr
	})
	if count == 1 {
		return nil, fmt.Errorf("we already voted for this address")
	}

	o.candidates = append(o.candidates, req)
	return &empty.Empty{}, nil
}

func (o *operator) Candidates(ctx context.Context, req *empty.Empty) (*proto.CandidatesResp, error) {
	o.candidatesLock.Lock()
	defer o.candidatesLock.Unlock()

	resp := &proto.CandidatesResp{
		Candidates: []*proto.Candidate{},
	}
	resp.Candidates = append(resp.Candidates, o.candidates...)
	return resp, nil
}
