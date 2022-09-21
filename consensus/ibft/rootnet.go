package ibft

import (
	"fmt"
)

/*
	backend implementation
	for rootnet monitoring
*/

func (i *backendIBFT) Sign(data []byte) ([]byte, uint64, error) {
	latestBlockNumber := i.blockchain.Header().Number

	signer, _, _, err := getModulesFromForkManager(i.forkManager, latestBlockNumber)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to get signer for block=%d: %w", latestBlockNumber, err)
	}

	signature, err := signer.SignIBFTMessage(data)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to sign data: %w", err)
	}

	return signature, latestBlockNumber, nil
}

func (i *backendIBFT) VerifySignature(signature, data []byte, blockNumber uint64) error {
	signer, validatorSet, _, err := getModulesFromForkManager(i.forkManager, blockNumber)
	if err != nil {
		return fmt.Errorf("unable to get signer for block=%d: %w", blockNumber, err)
	}

	addr, err := signer.EcrecoverFromIBFTMessage(signature, data)
	if err != nil {
		return fmt.Errorf("unable to get recover address from data: %w", err)
	}

	if !validatorSet.Includes(addr) {
		return fmt.Errorf(
			"signature generated from a non-validator: block=%d addr=%s", blockNumber, addr.String())
	}

	return nil
}
