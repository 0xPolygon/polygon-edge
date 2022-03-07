package ibft

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
)

var (
	// Magic nonce number to vote on adding a new validator
	nonceAuthVote = types.Nonce{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	// Magic nonce number to vote on removing a validator.
	nonceDropVote = types.Nonce{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
)

var (
	ErrInvalidNonce = errors.New("invalid nonce specified")
)

// PoAMechanism defines specific hooks for the Proof of Authority IBFT mechanism
type PoAMechanism struct {
	BaseConsensusMechanism
}

// PoAFactory initializes the required data
// for the Proof of Authority mechanism
func PoAFactory(ibft *Ibft, params *IBFTFork) (ConsensusMechanism, error) {
	poa := &PoAMechanism{
		BaseConsensusMechanism: BaseConsensusMechanism{
			mechanismType: PoA,
			ibft:          ibft,
		},
	}

	if err := poa.initializeParams(params); err != nil {
		return nil, err
	}

	poa.initializeHookMap()

	return poa, nil
}

// IsAvailable returns indicates if mechanism should be called at given height
func (poa *PoAMechanism) IsAvailable(hookType HookType, height uint64) bool {
	switch hookType {
	case AcceptStateLogHook, VerifyHeadersHook, ProcessHeadersHook, CandidateVoteHook, CalculateProposerHook:
		return poa.IsInRange(height)
	default:
		return false
	}
}

// acceptStateLogHook logs the current snapshot with the number of votes
func (poa *PoAMechanism) acceptStateLogHook(snapParam interface{}) error {
	// Cast the param to a *Snapshot
	snap, ok := snapParam.(*Snapshot)
	if !ok {
		return ErrInvalidHookParam
	}

	// Log the info message
	poa.ibft.logger.Info(
		"current snapshot",
		"validators",
		len(snap.Set),
		"votes",
		len(snap.Votes),
	)

	return nil
}

// verifyHeadersHook verifies that the header nonce conforms to the IBFT PoA proposal format
func (poa *PoAMechanism) verifyHeadersHook(nonceParam interface{}) error {
	// Cast the param to the nonce
	nonce, ok := nonceParam.(types.Nonce)
	if !ok {
		return ErrInvalidHookParam
	}

	// Check the nonce format.
	// The nonce field must have either an AUTH or DROP vote value.
	// Block nonce values are not taken into account when the Miner field is set to zeroes, indicating
	// no vote casting is taking place within a block
	if nonce != nonceDropVote && nonce != nonceAuthVote {
		return ErrInvalidNonce
	}

	return nil
}

// processHeadersHookParams are the params passed into the processHeadersHook
type processHeadersHookParams struct {
	header     *types.Header
	snap       *Snapshot
	parentSnap *Snapshot
	proposer   types.Address
	saveSnap   func(h *types.Header)
}

// processHeadersHook does the required logic for PoA header processing
func (poa *PoAMechanism) processHeadersHook(hookParam interface{}) error {
	// Cast the params to processHeadersHookParams
	params, ok := hookParam.(*processHeadersHookParams)
	if !ok {
		return ErrInvalidHookParam
	}

	number := params.header.Number
	if number%poa.ibft.epochSize == 0 {
		// during a checkpoint block, we reset the votes
		// and there cannot be any proposals
		params.snap.Votes = nil
		params.saveSnap(params.header)

		// remove in-memory snapshots from two epochs before this one
		epoch := int(number/poa.ibft.epochSize) - 2
		if epoch > 0 {
			purgeBlock := uint64(epoch) * poa.ibft.epochSize
			poa.ibft.store.deleteLower(purgeBlock)
		}

		return nil
	}

	// if we have a miner address, this might be a vote
	if params.header.Miner == types.ZeroAddress {
		return nil
	}

	// the nonce selects the action
	var authorize bool

	switch params.header.Nonce {
	case nonceAuthVote:
		authorize = true
	case nonceDropVote:
		authorize = false
	default:
		return fmt.Errorf("incorrect vote nonce")
	}

	// validate the vote
	if authorize {
		// we can only authorize if they are not on the validators list
		if params.snap.Set.Includes(params.header.Miner) {
			return nil
		}
	} else {
		// we can only remove if they are part of the validators list
		if !params.snap.Set.Includes(params.header.Miner) {
			return nil
		}
	}

	voteCount := params.snap.Count(func(v *Vote) bool {
		return v.Validator == params.proposer && v.Address == params.header.Miner
	})

	if voteCount > 1 {
		// there can only be one vote per validator per address
		return fmt.Errorf("more than one proposal per validator per address found")
	}

	if voteCount == 0 {
		// cast the new vote since there is no one yet
		params.snap.Votes = append(params.snap.Votes, &Vote{
			Validator: params.proposer,
			Address:   params.header.Miner,
			Authorize: authorize,
		})
	}

	// check the tally for the proposed validator
	tally := params.snap.Count(func(v *Vote) bool {
		return v.Address == params.header.Miner
	})

	// If more than a half of all validators voted
	if tally > params.snap.Set.Len()/2 {
		if authorize {
			// add the candidate to the validators list
			params.snap.Set.Add(params.header.Miner)
		} else {
			// remove the candidate from the validators list
			params.snap.Set.Del(params.header.Miner)

			// remove any votes casted by the removed validator
			params.snap.RemoveVotes(func(v *Vote) bool {
				return v.Validator == params.header.Miner
			})
		}

		// remove all the votes that promoted this validator
		params.snap.RemoveVotes(func(v *Vote) bool {
			return v.Address == params.header.Miner
		})
	}

	return nil
}

// candidateVoteHookParams are the params passed into the candidateVoteHook
type candidateVoteHookParams struct {
	header *types.Header
	snap   *Snapshot
}

// candidateVoteHook checks if any candidate is up for voting by the operator
// and casts a vote in the Nonce field of the block being built
func (poa *PoAMechanism) candidateVoteHook(hookParams interface{}) error {
	// Cast the params to candidateVoteHookParams
	params, ok := hookParams.(*candidateVoteHookParams)
	if !ok {
		return ErrInvalidHookParam
	}

	// try to pick a candidate
	if candidate := poa.ibft.operator.getNextCandidate(params.snap); candidate != nil {
		params.header.Miner = types.StringToAddress(candidate.Address)
		if candidate.Auth {
			params.header.Nonce = nonceAuthVote
		} else {
			params.header.Nonce = nonceDropVote
		}
	}

	return nil
}

// calculateProposerHook calculates the next proposer based on the last
func (poa *PoAMechanism) calculateProposerHook(lastProposerParam interface{}) error {
	lastProposer, ok := lastProposerParam.(types.Address)
	if !ok {
		return ErrInvalidHookParam
	}

	poa.ibft.state.CalcProposer(lastProposer)

	return nil
}

// initializeHookMap registers the hooks that the PoA mechanism
// should have
func (poa *PoAMechanism) initializeHookMap() {
	// Create the hook map
	poa.hookMap = make(map[HookType]func(interface{}) error)

	// Register the AcceptStateLogHook
	poa.hookMap[AcceptStateLogHook] = poa.acceptStateLogHook

	// Register the VerifyHeadersHook
	poa.hookMap[VerifyHeadersHook] = poa.verifyHeadersHook

	// Register the ProcessHeadersHook
	poa.hookMap[ProcessHeadersHook] = poa.processHeadersHook

	// Register the CandidateVoteHook
	poa.hookMap[CandidateVoteHook] = poa.candidateVoteHook

	// Register the CalculateProposerHook
	poa.hookMap[CalculateProposerHook] = poa.calculateProposerHook
}

// ShouldWriteTransactions indicates if transactions should be written to a block
func (poa *PoAMechanism) ShouldWriteTransactions(blockNumber uint64) bool {
	// The PoA mechanism doesn't have special cases where transactions
	// shouldn't be written to a block
	return poa.IsInRange(blockNumber)
}
