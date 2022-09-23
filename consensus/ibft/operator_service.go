package ibft

import (
	"context"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

var (
	ErrVotingNotSupported = errors.New("voting is not supported")
	ErrHeaderNotFound     = errors.New("header not found")
)

type operator struct {
	proto.UnimplementedIbftOperatorServer

	ibft *backendIBFT
}

// Votable is an interface of the ValidatorStore with vote function
type Votable interface {
	Votes(uint64) ([]*store.Vote, error)
	Candidates() []*store.Candidate
	Propose(validators.Validator, bool, types.Address) error
}

// Status returns the status of the IBFT client
func (o *operator) Status(ctx context.Context, req *empty.Empty) (*proto.IbftStatusResp, error) {
	signer, err := o.getLatestSigner()
	if err != nil {
		return nil, err
	}

	return &proto.IbftStatusResp{
		Key: signer.Address().String(),
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
		return nil, ErrHeaderNotFound
	}

	validatorsStore, err := o.ibft.forkManager.GetValidatorStore(height)
	if err != nil {
		return nil, err
	}

	validators, err := o.ibft.forkManager.GetValidators(height)
	if err != nil {
		return nil, err
	}

	resp := &proto.Snapshot{
		Number:     height,
		Hash:       header.Hash.String(),
		Validators: validatorsToProtoValidators(validators),
	}

	votes, err := getVotes(validatorsStore, height)
	if err != nil {
		return nil, err
	}

	if votes == nil {
		// current ValidatorStore doesn't have voting function
		return resp, nil
	}

	resp.Votes = votesToProtoVotes(votes)

	return resp, nil
}

// Propose proposes a new candidate to be added / removed from the validator set
func (o *operator) Propose(ctx context.Context, req *proto.Candidate) (*empty.Empty, error) {
	votableSet, err := o.getVotableValidatorStore()
	if err != nil {
		return nil, err
	}

	candidate, err := o.parseCandidate(req)
	if err != nil {
		return nil, err
	}

	if err := votableSet.Propose(candidate, req.Auth, o.ibft.currentSigner.Address()); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// Candidates returns the validator candidates list
func (o *operator) Candidates(ctx context.Context, req *empty.Empty) (*proto.CandidatesResp, error) {
	votableValSet, err := o.getVotableValidatorStore()
	if err != nil {
		return nil, err
	}

	candidates := votableValSet.Candidates()

	return &proto.CandidatesResp{
		Candidates: candidatesToProtoCandidates(candidates),
	}, nil
}

// parseCandidate parses proto.Candidate and maps to validator
func (o *operator) parseCandidate(req *proto.Candidate) (validators.Validator, error) {
	signer, err := o.getLatestSigner()
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
		if req.Auth {
			// BLS public key is necessary but the command is not required
			if req.BlsPubkey == nil {
				return nil, errors.New("BLS public key required")
			}

			if _, err := crypto.UnmarshalBLSPublicKey(req.BlsPubkey); err != nil {
				return nil, err
			}
		}

		// BLS Public Key doesn't have to be given in case of removal
		return &validators.BLSValidator{
			Address:      types.StringToAddress(req.Address),
			BLSPublicKey: req.BlsPubkey,
		}, nil
	}

	return nil, fmt.Errorf("invalid validator type: %s", signer.Type())
}

// getVotableValidatorStore gets current validator set and convert its type to Votable
func (o *operator) getVotableValidatorStore() (Votable, error) {
	valSet, err := o.ibft.forkManager.GetValidatorStore(o.ibft.blockchain.Header().Number)
	if err != nil {
		return nil, err
	}

	votableValSet, ok := valSet.(Votable)
	if !ok {
		return nil, ErrVotingNotSupported
	}

	return votableValSet, nil
}

// getLatestSigner gets the latest signer IBFT uses
func (o *operator) getLatestSigner() (signer.Signer, error) {
	if o.ibft.currentSigner != nil {
		return o.ibft.currentSigner, nil
	}

	return o.ibft.forkManager.GetSigner(o.ibft.blockchain.Header().Number)
}

// validatorsToProtoValidators converts validators to response of validators
func validatorsToProtoValidators(validators validators.Validators) []*proto.Snapshot_Validator {
	protoValidators := make([]*proto.Snapshot_Validator, validators.Len())

	for idx := 0; idx < validators.Len(); idx++ {
		validator := validators.At(uint64(idx))

		protoValidators[idx] = &proto.Snapshot_Validator{
			Type:    string(validator.Type()),
			Address: validator.Addr().String(),
			Data:    validator.Bytes(),
		}
	}

	return protoValidators
}

// votesToProtoVotes converts votes to response of votes
func votesToProtoVotes(votes []*store.Vote) []*proto.Snapshot_Vote {
	protoVotes := make([]*proto.Snapshot_Vote, len(votes))

	for idx := range votes {
		protoVotes[idx] = &proto.Snapshot_Vote{
			Validator: votes[idx].Validator.String(),
			Proposed:  votes[idx].Candidate.String(),
			Auth:      votes[idx].Authorize,
		}
	}

	return protoVotes
}

func candidatesToProtoCandidates(candidates []*store.Candidate) []*proto.Candidate {
	protoCandidates := make([]*proto.Candidate, len(candidates))

	for idx, candidate := range candidates {
		protoCandidates[idx] = &proto.Candidate{
			Address: candidate.Validator.Addr().String(),
			Auth:    candidate.Authorize,
		}

		if blsVal, ok := candidate.Validator.(*validators.BLSValidator); ok {
			protoCandidates[idx].BlsPubkey = blsVal.BLSPublicKey
		}
	}

	return protoCandidates
}

// getVotes gets votes from validator store only if store supports voting
func getVotes(validatorStore store.ValidatorStore, height uint64) ([]*store.Vote, error) {
	votableStore, ok := validatorStore.(Votable)
	if !ok {
		return nil, nil
	}

	return votableStore.Votes(height)
}
