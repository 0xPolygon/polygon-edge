package ibft

import (
	"context"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

var (
	ErrVotingNotSupported = errors.New("voting is not supported")
)

type operator struct {
	proto.UnimplementedIbftOperatorServer

	ibft *backendIBFT
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

	valSet, err := o.ibft.forkManager.GetValidatorStore(height)
	if err != nil {
		return nil, err
	}

	vals, err := o.ibft.forkManager.GetValidators(height)
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

	votableValSet, ok := valSet.(store.Votable)
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
	votableSet, err := o.getVotableValidatorStore()
	if err != nil {
		return &empty.Empty{}, err
	}

	candidate, err := o.parseCandidate(req)
	if err != nil {
		return &empty.Empty{}, err
	}

	if err := votableSet.Propose(candidate, req.Auth, o.ibft.currentSigner.Address()); err != nil {
		return &empty.Empty{}, err
	}

	return &empty.Empty{}, nil
}

// Candidates returns the validator candidates list
func (o *operator) Candidates(ctx context.Context, req *empty.Empty) (*proto.CandidatesResp, error) {
	valSet, err := o.ibft.forkManager.GetValidatorStore(o.ibft.blockchain.Header().Number)
	if err != nil {
		return nil, err
	}

	votableValSet, ok := valSet.(store.Votable)
	if !ok {
		return nil, fmt.Errorf("voting is not supported")
	}

	candidates := votableValSet.Candidates()

	resp := &proto.CandidatesResp{
		Candidates: make([]*proto.Candidate, len(candidates)),
	}

	for idx, candidate := range candidates {
		resp.Candidates[idx] = &proto.Candidate{
			Address:   candidate.Validator.Addr().String(),
			Auth:      candidate.Authorize,
			BlsPubkey: []byte{},
		}

		if blsVal, ok := candidate.Validator.(*validators.BLSValidator); ok {
			resp.Candidates[idx].BlsPubkey = blsVal.BLSPublicKey
		}
	}

	return resp, nil
}

// parseCandidate parses proto.Candidate and maps to validator
func (o *operator) parseCandidate(req *proto.Candidate) (validators.Validator, error) {
	signer, err := o.ibft.forkManager.GetSigner(o.ibft.blockchain.Header().Number)
	if err != nil {
		return nil, err
	}

	switch signer.Type() {
	case validators.ECDSAValidatorType:
		return &validators.ECDSAValidator{
			Address: types.StringToAddress(req.Address),
		}, nil

	case validators.BLSValidatorType:
		// safe check
		// user doesn't give BLS Public Key in case of removal
		if req.Auth {
			if _, err := crypto.UnmarshalBLSPublicKey(req.BlsPubkey); err != nil {
				return nil, err
			}
		}

		return &validators.BLSValidator{
			Address:      types.StringToAddress(req.Address),
			BLSPublicKey: req.BlsPubkey,
		}, nil
	}

	return nil, fmt.Errorf("invalid validator type: %s", signer.Type())
}

// getVotableValidatorStore gets current validator set and convert its type to Votable
func (o *operator) getVotableValidatorStore() (store.Votable, error) {
	valSet, err := o.ibft.forkManager.GetValidatorStore(o.ibft.blockchain.Header().Number)
	if err != nil {
		return nil, err
	}

	votableValSet, ok := valSet.(store.Votable)
	if !ok {
		return nil, ErrVotingNotSupported
	}

	return votableValSet, nil
}
