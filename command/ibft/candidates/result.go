package candidates

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	ibftHelper "github.com/0xPolygon/polygon-edge/command/ibft/helper"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

type IBFTCandidate struct {
	Address      string          `json:"address"`
	BLSPublicKey *string         `json:"bls_pubkey"`
	Vote         ibftHelper.Vote `json:"vote"`
}

type IBFTCandidatesResult struct {
	Candidates []IBFTCandidate `json:"candidates"`
}

func newIBFTCandidatesResult(resp *ibftOp.CandidatesResp) (*IBFTCandidatesResult, error) {
	res := &IBFTCandidatesResult{
		Candidates: make([]IBFTCandidate, len(resp.Candidates)),
	}

	for i, c := range resp.Candidates {
		var validatorType validators.ValidatorType
		if err := validatorType.FromString(c.Type); err != nil {
			return nil, err
		}

		validator := validators.NewValidatorFromType(validatorType)
		if err := validator.SetFromBytes(c.Data); err != nil {
			return nil, err
		}

		res.Candidates[i].Address = validator.Addr().String()
		res.Candidates[i].Vote = ibftHelper.BoolToVote(c.Auth)

		if blsValidator, ok := validator.(*validators.BLSValidator); ok {
			res.Candidates[i].BLSPublicKey = types.EncodeBytes(blsValidator.BLSPublicKey)
		}
	}

	return res, nil
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
