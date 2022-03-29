package snapshot

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
	ibftHelper "github.com/0xPolygon/polygon-edge/command/ibft/helper"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
)

type IBFTSnapshotVote struct {
	Proposer string          `json:"proposer"`
	Address  string          `json:"address"`
	Vote     ibftHelper.Vote `json:"vote"`
}

type IBFTSnapshotResult struct {
	Number     uint64             `json:"number"`
	Hash       string             `json:"hash"`
	Votes      []IBFTSnapshotVote `json:"votes"`
	Validators []string           `json:"validators"`
}

func newIBFTSnapshotResult(resp *ibftOp.Snapshot) *IBFTSnapshotResult {
	res := &IBFTSnapshotResult{
		Number:     resp.Number,
		Hash:       resp.Hash,
		Votes:      make([]IBFTSnapshotVote, len(resp.Votes)),
		Validators: make([]string, len(resp.Validators)),
	}

	for i, v := range resp.Votes {
		res.Votes[i].Proposer = v.Validator
		res.Votes[i].Address = v.Proposed
		res.Votes[i].Vote = ibftHelper.BoolToVote(v.Auth)
	}

	for i, v := range resp.Validators {
		res.Validators[i] = v.Address
	}

	return res
}

func (r *IBFTSnapshotResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[IBFT SNAPSHOT]\n")
	r.writeBlockData(&buffer)
	r.writeVoteData(&buffer)
	r.writeValidatorData(&buffer)

	return buffer.String()
}

func (r *IBFTSnapshotResult) writeBlockData(buffer *bytes.Buffer) {
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Block|%d", r.Number),
		fmt.Sprintf("Hash|%s", r.Hash),
	}))
	buffer.WriteString("\n")
}

func (r *IBFTSnapshotResult) writeVoteData(buffer *bytes.Buffer) {
	numVotes := len(r.Votes)
	votes := make([]string, numVotes+1)

	votes[0] = "No votes found"

	if numVotes > 0 {
		votes[0] = "PROPOSER|ADDRESS|VOTE TO ADD"

		for i, d := range r.Votes {
			votes[i+1] = fmt.Sprintf(
				"%s|%s|%s",
				d.Proposer,
				d.Address,
				ibftHelper.VoteToString(d.Vote),
			)
		}
	}

	buffer.WriteString("\n[VOTES]\n")
	buffer.WriteString(helper.FormatList(votes))
	buffer.WriteString("\n")
}

func (r *IBFTSnapshotResult) writeValidatorData(buffer *bytes.Buffer) {
	numValidators := len(r.Validators)
	validators := make([]string, numValidators+1)
	validators[0] = "No validators found"

	if numValidators > 0 {
		validators[0] = "ADDRESS"
		for i, d := range r.Validators {
			validators[i+1] = d
		}
	}

	buffer.WriteString("\n[VALIDATORS]\n")
	buffer.WriteString(helper.FormatList(validators))
	buffer.WriteString("\n")
}
