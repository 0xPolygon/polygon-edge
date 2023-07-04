package polybft

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
)

const abiMethodIDLength = 4

func decodeStateTransaction(txData []byte) (contractsapi.StateTransactionInput, error) {
	if len(txData) < abiMethodIDLength {
		return nil, fmt.Errorf("state transactions have input")
	}

	sig := txData[:abiMethodIDLength]

	var (
		commitFn            contractsapi.CommitStateReceiverFn
		commitEpochFn       contractsapi.CommitEpochValidatorSetFn
		distributeRewardsFn contractsapi.DistributeRewardForRewardPoolFn
		obj                 contractsapi.StateTransactionInput
	)

	if bytes.Equal(sig, commitFn.Sig()) {
		// bridge commitment
		obj = &CommitmentMessageSigned{}
	} else if bytes.Equal(sig, commitEpochFn.Sig()) {
		// commit epoch
		obj = &contractsapi.CommitEpochValidatorSetFn{}
	} else if bytes.Equal(sig, distributeRewardsFn.Sig()) {
		// distribute rewards
		obj = &contractsapi.DistributeRewardForRewardPoolFn{}
	} else {
		return nil, fmt.Errorf("unknown state transaction")
	}

	if err := obj.DecodeAbi(txData); err != nil {
		return nil, err
	}

	return obj, nil
}
