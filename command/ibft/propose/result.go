package propose

import (
	"bytes"
	"fmt"
)

type IBFTProposeResult struct {
	Address string `json:"-"`
	Vote    string `json:"-"`
}

func (r *IBFTProposeResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[IBFT PROPOSE]\n")
	buffer.WriteString(r.Message())
	buffer.WriteString("\n")

	return buffer.String()
}

func (r *IBFTProposeResult) Message() string {
	if r.Vote == authVote {
		return fmt.Sprintf(
			"Successfully voted for the addition of address [%s] to the validator set",
			r.Address,
		)
	}

	return fmt.Sprintf(
		"Successfully voted for the removal of validator at address [%s] from the validator set",
		r.Address,
	)
}

func (r *IBFTProposeResult) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"message": "%s"}`, r.Message())), nil
}
