package contracts

import (
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
)

type StateSyncProof struct {
	Proof     []types.Hash
	StateSync *contractsapi.StateSyncedEvent
}

// EncodeAbi contains logic for encoding given ABI data
func (ssp *StateSyncProof) EncodeAbi() ([]byte, error) {
	execute := contractsapi.Execute{
		Proof: ssp.Proof,
		Obj:   (*contractsapi.StateSync)(ssp.StateSync),
	}

	return execute.EncodeAbi()
}

// DecodeAbi contains logic for decoding given ABI data
func (ssp *StateSyncProof) DecodeAbi(txData []byte) error {
	execute := &contractsapi.Execute{}

	if err := execute.DecodeAbi(txData); err != nil {
		return err
	}

	*ssp = StateSyncProof{
		Proof:     execute.Proof,
		StateSync: (*contractsapi.StateSyncedEvent)(execute.Obj),
	}

	return nil
}

type ExitProof struct {
	Proof     []types.Hash
	LeafIndex uint64
}
