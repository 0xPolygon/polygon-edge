package candidates

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	ibftHelper "github.com/0xPolygon/polygon-edge/command/ibft/helper"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
)

type IBFTCandidate struct {
	Address string          `json:"address"`
	Vote    ibftHelper.Vote `json:"vote"`
}

type IBFTCandidatesResult struct {
	Candidates []IBFTCandidate `json:"candidates"`
}

func newIBFTCandidatesResult(resp *ibftOp.CandidatesResp) *IBFTCandidatesResult {
	res := &IBFTCandidatesResult{
		Candidates: make([]IBFTCandidate, len(resp.Candidates)),
	}

	for i, c := range resp.Candidates {
		res.Candidates[i].Address = c.Address
		res.Candidates[i].Vote = ibftHelper.BoolToVote(c.Auth)
	}

	return res
}

func (r *IBFTCandidatesResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[IBFT CANDIDATES]\n")

	if num := len(r.Candidates); num == 0 {
		buffer.WriteString("No candidates found")
	} else {
		buffer.WriteString(fmt.Sprintf("Number of candidates: %d\n\n", num))
		buffer.WriteString(formatCandidates(r.Candidates))
	}

	buffer.WriteString("\n")

	return buffer.String()
}

func formatCandidates(candidates []IBFTCandidate) string {
	generatedCandidates := make([]string, 0, len(candidates)+1)

	generatedCandidates = append(generatedCandidates, "Address|Vote")
	for _, c := range candidates {
		generatedCandidates = append(generatedCandidates, fmt.Sprintf("%s|%s", c.Address, c.Vote))
	}

	return helper.FormatKV(generatedCandidates)
}
