package ibft

import (
	"context"
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/types"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

type operator struct {
	ibft *Ibft

	candidatesLock sync.Mutex
	candidates     []*proto.Candidate

	proto.UnimplementedIbftOperatorServer
}

// Status returns the status of the IBFT client
func (o *operator) Status(ctx context.Context, req *empty.Empty) (*proto.IbftStatusResp, error) {
	resp := &proto.IbftStatusResp{
		Key: o.ibft.validatorKeyAddr.String(),
	}

	return resp, nil
}

// getNextCandidate returns a candidate from the snapshot
func (o *operator) getNextCandidate(snap *Snapshot) *proto.Candidate {
	o.candidatesLock.Lock()
	defer o.candidatesLock.Unlock()

	// first, we need to remove any candidates that have already been
	// selected as validators
	for i := 0; i < len(o.candidates); i++ {
		addr := types.StringToAddress(o.candidates[i].Address)

		// Define the delete callback method
		deleteFn := func() {
			o.candidates = append(o.candidates[:i], o.candidates[i+1:]...)
			i--
		}

		// Check if the candidate is already in the validator set, and wants to be added
		if o.candidates[i].Auth && snap.Set.Includes(addr) {
			deleteFn()

			continue
		}

		// Check if the candidate is not in the validator set, and wants to be removed
		if !o.candidates[i].Auth && !snap.Set.Includes(addr) {
			deleteFn()
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
			// Candidate found
			candidate = c

			break
		}
	}

	return candidate
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

// Propose proposes a new candidate to be added / removed from the validator set
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
			return nil, fmt.Errorf("the candidate is already a validator")
		}
	}

	if !req.Auth {
		if !snap.Set.Includes(addr) {
			return nil, fmt.Errorf("cannot remove a validator if they're not in the snapshot")
		}
	}

	// check if we have already voted for this candidate
	count := snap.Count(func(v *Vote) bool {
		return v.Address == addr && v.Validator == o.ibft.validatorKeyAddr
	})
	if count == 1 {
		return nil, fmt.Errorf("already voted for this address")
	}

	o.candidates = append(o.candidates, req)

	return &empty.Empty{}, nil
}

// Candidates returns the validator candidates list
func (o *operator) Candidates(ctx context.Context, req *empty.Empty) (*proto.CandidatesResp, error) {
	o.candidatesLock.Lock()
	defer o.candidatesLock.Unlock()

	resp := &proto.CandidatesResp{
		Candidates: []*proto.Candidate{},
	}

	resp.Candidates = append(resp.Candidates, o.candidates...)

	return resp, nil
}
