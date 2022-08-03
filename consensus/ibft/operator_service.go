package ibft

import (
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/validators/valset"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

type operator struct {
	ibft *Ibft
	proto.UnimplementedIbftOperatorServer
}

// Status returns the status of the IBFT client
func (o *operator) Status(ctx context.Context, req *empty.Empty) (*proto.IbftStatusResp, error) {
	var signerAddr string
	if o.ibft.currentSigner != nil {
		signerAddr = o.ibft.currentSigner.Address().String()
	}

	return &proto.IbftStatusResp{
		Key: signerAddr,
	}, nil
}

// GetSnapshot returns the snapshot, based on the passed in request
func (o *operator) GetSnapshot(ctx context.Context, req *proto.SnapshotReq) (*proto.Snapshot, error) {
	height := req.Number
	if req.Latest {
		height = o.ibft.blockchain.Header().Number
	}

	header, ok := o.ibft.blockchain.GetHeaderByNumber(height)
	if !ok {
		return nil, fmt.Errorf("header not found")
	}

	resp := &proto.Snapshot{
		Number: height,
		Hash:   header.Hash.String(),
	}

	valSet, err := o.ibft.forkManager.GetValidatorSet(height)
	if err != nil {
		return nil, err
	}

	vals, err := valSet.GetValidators(height)
	if err != nil {
		return nil, err
	}

	resp.Validators = make([]*proto.Snapshot_Validator, vals.Len())
	for idx := 0; idx < vals.Len(); idx++ {
		val := vals.At(uint64(idx))

		resp.Validators[idx] = &proto.Snapshot_Validator{
			Type: string(vals.Type()),
			Data: val.Bytes(),
		}
	}

	votableValSet, ok := valSet.(valset.Votable)
	if !ok {
		return resp, nil
	}

	votes, err := votableValSet.Votes(height)
	if err != nil {
		return nil, err
	}

	resp.Votes = make([]*proto.Snapshot_Vote, len(votes))
	for idx := range votes {
		resp.Votes[idx] = &proto.Snapshot_Vote{
			Validator: votes[idx].Validator.String(),
			Proposed:  votes[idx].Candidate.String(),
			Auth:      votes[idx].Authorize,
		}
	}

	return resp, nil
}

// Propose proposes a new candidate to be added / removed from the validator set
func (o *operator) Propose(ctx context.Context, req *proto.Candidate) (*empty.Empty, error) {
	valSet, err := o.ibft.forkManager.GetValidatorSet(o.ibft.blockchain.Header().Number)
	if err != nil {
		return nil, err
	}

	votableValSet, ok := valSet.(valset.Votable)
	if !ok {
		return nil, fmt.Errorf("voting is not supported")
	}

	if err := votableValSet.Propose(req.Data, req.Auth, o.ibft.currentSigner.Address()); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// Candidates returns the validator candidates list
func (o *operator) Candidates(ctx context.Context, req *empty.Empty) (*proto.CandidatesResp, error) {
	valSet, err := o.ibft.forkManager.GetValidatorSet(o.ibft.blockchain.Header().Number)
	if err != nil {
		return nil, err
	}

	votableValSet, ok := valSet.(valset.Votable)
	if !ok {
		return nil, fmt.Errorf("voting is not supported")
	}

	candidates, err := votableValSet.Candidates()
	if err != nil {
		return nil, err
	}

	resp := &proto.CandidatesResp{
		Candidates: make([]*proto.Candidate, len(candidates)),
	}

	for idx, candidate := range candidates {
		resp.Candidates[idx] = &proto.Candidate{
			Type: string(candidate.Validator.Type()),
			Data: candidate.Validator.Bytes(),
			Auth: candidate.Authorize,
		}
	}

	return resp, nil
}
