package ibft

import (
	"context"
	"fmt"
	"sync"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/empty"
)

type operator struct {
	ibft *Ibft

	candidatesLock sync.Mutex
	candidates     []*proto.Candidate

	proto.UnimplementedIbftOperatorServer
}

func (o *operator) Status(ctx context.Context, req *empty.Empty) (*proto.IbftStatusResp, error) {
	resp := &proto.IbftStatusResp{
		Key: o.ibft.validatorKeyAddr.String(),
	}
	return resp, nil
}

func (o *operator) getNextCandidate(snap *Snapshot) *proto.Candidate {
	o.candidatesLock.Lock()
	defer o.candidatesLock.Unlock()

	// first, we need to remove any candidates that have already been
	// selected as validators
	for i := 0; i < len(o.candidates); i++ {
		addr := types.StringToAddress(o.candidates[i].Address)

		delete := func() {
			o.candidates = append(o.candidates[:i], o.candidates[i+1:]...)
			i--
		}

		if o.candidates[i].Auth && snap.Set.Includes(addr) {
			// if add and its already in, we can remove it
			delete()
			continue
		}
		if !o.candidates[i].Auth && !snap.Set.Includes(addr) {
			// if remove and already out of the validators we can remove it
			delete()
		}
	}

	var candidate *proto.Candidate

	// now pick the first candidate that has not received a vote yet
	for _, c := range o.candidates {
		addr := types.StringToAddress(c.Address)

		count := snap.Count(func(v *Vote) bool {
			return v.Address == addr && v.Validator == o.ibft.validatorKeyAddr
		})
		if count == 0 {
			candidate = c
			break
		}
	}
	return candidate
}

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

	snap, err := o.ibft.getLatestSnapshot()
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
		return v.Address == addr && v.Validator == o.ibft.validatorKeyAddr
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
