package ibft

import (
	"crypto/ecdsa"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
)

// useIstanbulHeaderHash is a helper function so that test use istanbulHeaderHash during the test
func useIstanbulHeaderHash(t *testing.T) {
	t.Helper()

	originalHashCalc := types.HeaderHash
	types.HeaderHash = istanbulHeaderHash

	t.Cleanup(func() {
		types.HeaderHash = originalHashCalc
	})
}

// func TestExtraEncoding(t *testing.T) {
// 	seal1 := types.StringToHash("1").Bytes()
// 	seal2 := types.StringToHash("2").Bytes()

// 	cases := []struct {
// 		data *IstanbulExtra
// 	}{
// 		{
// 			data: &IstanbulExtra{
// 				Validators: []types.Address{
// 					types.StringToAddress("1"),
// 				},
// 				Seal: seal1,
// 				CommittedSeal: &SerializedSeal{
// 					seal1,
// 				},
// 				ParentCommittedSeal: &SerializedSeal{
// 					seal2,
// 				},
// 			},
// 		},
// 	}

// 	for _, c := range cases {
// 		data := c.data.MarshalRLPTo(nil)

// 		ii := &IstanbulExtra{}
// 		if err := ii.UnmarshalRLP(data); err != nil {
// 			t.Fatal(err)
// 		}

// 		if !reflect.DeepEqual(c.data, ii) {
// 			t.Fatal("bad")
// 		}
// 	}
// }

func generateKeysAndAddresses(t *testing.T, num int) ([]*ecdsa.PrivateKey, []types.Address) {
	t.Helper()

	keys := make([]*ecdsa.PrivateKey, num)
	addrs := make([]types.Address, num)

	for i := range keys {
		pk, addr := tests.GenerateKeyAndAddr(t)
		keys[i] = pk
		addrs[i] = addr
	}

	return keys, addrs
}

// func createIBFTHeader(
// 	t *testing.T,
// 	num uint64,
// 	parentHash types.Hash,
// 	validators []types.Address,
// 	lastCommittedSeal [][]byte,
// ) *types.Header {
// 	t.Helper()

// 	header := &types.Header{
// 		Number:     num,
// 		ParentHash: parentHash,
// 	}

// 	sSeal := SerializedSeal(lastCommittedSeal)

// 	initIbftExtra(header, validators, &sSeal, false)

// 	return header
// }

// Test Scenario
// 1. 4 IBFT Validators create headers
// 2. A faulty node scans the past headers and appends new committed seal
// 3. Check if each hash of the headers is wrong
// func TestAppendCommittedSeal(t *testing.T) {
// 	useIstanbulHeaderHash(t)

// 	var (
// 		numHeaders          = 5
// 		numNormalValidators = 4
// 		numFaultyValidators = 1

// 		headers                      = make([]*types.Header, 0, numHeaders)
// 		faultyHeaders                = make([]*types.Header, 0, numHeaders)
// 		parentHash                   = types.StringToHash("genesis")
// 		parentCommittedSeal [][]byte = nil

// 		keys, addresses     = generateKeysAndAddresses(t, numNormalValidators+numFaultyValidators)
// 		normalValidatorKeys = keys[:numNormalValidators]
// 		faultyValidatorKey  = keys[numNormalValidators]

// 		err error
// 	)

// 	// create headers by normal validators
// 	for i := 0; i < numHeaders; i++ {
// 		header := createIBFTHeader(t, uint64(i+1), parentHash, addresses, parentCommittedSeal)

// 		// write seal
// 		header, err = writeSeal(keys[0], header)
// 		assert.NoError(t, err)

// 		// write committed seal
// 		committedSeal := make([][]byte, len(normalValidatorKeys))
// 		for i, key := range normalValidatorKeys {
// 			committedSeal[i], err = createCommittedSeal(key, header)
// 			assert.NoError(t, err)
// 		}

// 		sSeal := SerializedSeal(committedSeal)
// 		assert.NoError(t, packCommittedSealIntoIbftExtra(header, &sSeal))

// 		header = header.ComputeHash()

// 		headers = append(headers, header)

// 		parentHash = header.Hash
// 		parentCommittedSeal = committedSeal
// 	}

// 	// faulty node scans the past headers and try to inject new committed seal
// 	for i, h := range headers {
// 		header := h.Copy()

// 		// update parent hash & committed seal
// 		if i > 0 {
// 			parentHeader := faultyHeaders[i-1]

// 			// update parent hash
// 			header.ParentHash = parentHeader.Hash

// 			// get parent committed seal
// 			x, err := unpackCommittedSealFromIbftExtra(parentHeader)
// 			assert.NoError(t, err)

// 			// update ParentCommittedSeal forcibly
// 			err = packFieldIntoIbftExtra(header, func(extra *IstanbulExtra) {
// 				extra.ParentCommittedSeal = x
// 			})
// 			assert.NoError(t, err)
// 		}

// 		// create new committed seal
// 		fx, err := createCommittedSeal(faultyValidatorKey, header)
// 		assert.NoError(t, err)

// 		// append new committed seal
// 		err = packFieldIntoIbftExtra(header, func(extra *IstanbulExtra) {
// 			sseal := extra.CommittedSeal.(*SerializedSeal)
// 			ssealSlice := [][]byte(*sseal)

// 			ssealSlice = append(ssealSlice, fx)

// 			nsseal := SerializedSeal(ssealSlice)

// 			extra.CommittedSeal = &nsseal
// 		})
// 		assert.NoError(t, err)

// 		header = header.ComputeHash()
// 		faultyHeaders = append(faultyHeaders, header)
// 	}

// 	// Check hashes are different
// 	for i := range headers {
// 		header, faultyHeader := headers[i], faultyHeaders[i]

// 		if i == 0 {
// 			// hashes should be same because first header doesn't have parent committed seal
// 			assert.Equal(t, header.Hash, faultyHeader.Hash)
// 		} else {
// 			assert.NotEqual(t, header.Hash, faultyHeader.Hash)
// 		}
// 	}
// }
