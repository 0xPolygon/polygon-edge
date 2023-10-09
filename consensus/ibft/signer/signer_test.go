package signer

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	testHelper "github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/stretchr/testify/assert"
)

var (
	errTest = errors.New("test err")

	testAddr1 = types.StringToAddress("1")
	testAddr2 = types.StringToAddress("1")

	testBLSPubKey1 = newTestBLSKeyBytes()
	testBLSPubKey2 = newTestBLSKeyBytes()

	ecdsaValidator1 = validators.NewECDSAValidator(
		testAddr1,
	)
	ecdsaValidator2 = validators.NewECDSAValidator(
		testAddr2,
	)

	blsValidator1 = validators.NewBLSValidator(testAddr1, testBLSPubKey1)
	blsValidator2 = validators.NewBLSValidator(testAddr2, testBLSPubKey2)

	ecdsaValidators = validators.NewECDSAValidatorSet(
		ecdsaValidator1,
		ecdsaValidator2,
	)
	blsValidators = validators.NewBLSValidatorSet(
		blsValidator1,
		blsValidator2,
	)

	testProposerSeal     = crypto.Keccak256([]byte{0x1})
	testSerializedSeals1 = &SerializedSeal{[]byte{0x1}, []byte{0x2}}
	testSerializedSeals2 = &SerializedSeal{[]byte{0x3}, []byte{0x4}}
	testAggregatedSeals1 = newTestAggregatedSeals([]int{0, 1}, []byte{0x12})
	testAggregatedSeals2 = newTestAggregatedSeals([]int{2, 3}, []byte{0x23})
)

func newTestAggregatedSeals(bitFlags []int, signature []byte) *AggregatedSeal {
	bitMap := new(big.Int)
	for _, idx := range bitFlags {
		bitMap = bitMap.SetBit(bitMap, idx, 1)
	}

	return &AggregatedSeal{
		Bitmap:    bitMap,
		Signature: signature,
	}
}

func newTestBLSKeyBytes() validators.BLSValidatorPublicKey {
	key, err := crypto.GenerateBLSKey()
	if err != nil {
		return nil
	}

	pubKey, err := key.GetPublicKey()
	if err != nil {
		return nil
	}

	buf, err := pubKey.MarshalBinary()
	if err != nil {
		return nil
	}

	return buf
}

func newTestSingleKeyManagerSigner(km KeyManager) *SignerImpl {
	return &SignerImpl{
		keyManager:       km,
		parentKeyManager: km,
	}
}

func getTestExtraBytes(
	validators validators.Validators,
	proposerSeal []byte,
	committedSeals Seals,
	parentCommittedSeals Seals,
	roundNumber *uint64,
) []byte {
	extra := &IstanbulExtra{
		Validators:           validators,
		ProposerSeal:         proposerSeal,
		CommittedSeals:       committedSeals,
		ParentCommittedSeals: parentCommittedSeals,
		RoundNumber:          roundNumber,
	}

	return append(
		make([]byte, IstanbulExtraVanity),
		extra.MarshalRLPTo(nil)...,
	)
}

func TestNewKeyManager(t *testing.T) {
	t.Parallel()

	keyManager := &MockKeyManager{}
	parentKeyManager := &MockKeyManager{}

	signer := NewSigner(keyManager, parentKeyManager)

	assert.Same(
		t,
		keyManager,
		signer.keyManager,
	)

	assert.Same(
		t,
		parentKeyManager,
		signer.parentKeyManager,
	)
}

func TestSignerType(t *testing.T) {
	t.Parallel()

	validatorType := validators.ECDSAValidatorType
	signer := newTestSingleKeyManagerSigner(
		&MockKeyManager{
			TypeFunc: func() validators.ValidatorType {
				return validatorType
			},
		},
	)

	assert.Equal(
		t,
		validatorType,
		signer.Type(),
	)
}

func TestSignerAddress(t *testing.T) {
	t.Parallel()

	addr := testAddr1
	signer := newTestSingleKeyManagerSigner(
		&MockKeyManager{
			AddressFunc: func() types.Address {
				return addr
			},
		},
	)

	assert.Equal(
		t,
		addr,
		signer.Address(),
	)
}

func TestSignerInitIBFTExtra(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		validators           validators.Validators
		committedSeals       Seals
		parentCommittedSeals Seals
	}{
		{
			name:                 "ECDSA Serialized Seals",
			validators:           ecdsaValidators,
			committedSeals:       &SerializedSeal{},
			parentCommittedSeals: testSerializedSeals1,
		},
		{
			name:                 "BLS Aggregated Seals",
			validators:           blsValidators,
			committedSeals:       &AggregatedSeal{},
			parentCommittedSeals: testAggregatedSeals1,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			header := &types.Header{}

			signer := newTestSingleKeyManagerSigner(
				&MockKeyManager{
					NewEmptyCommittedSealsFunc: func() Seals {
						return test.committedSeals
					},
				},
			)

			signer.InitIBFTExtra(
				header,
				test.validators,
				test.parentCommittedSeals,
			)

			expectedExtraBytes := getTestExtraBytes(
				test.validators,
				[]byte{},
				test.committedSeals,
				test.parentCommittedSeals,
				nil,
			)

			assert.Equal(
				t,
				expectedExtraBytes,
				header.ExtraData,
			)
		})
	}
}

func TestSignerGetIBFTExtra(t *testing.T) {
	tests := []struct {
		name          string
		header        *types.Header
		signer        *SignerImpl
		expectedExtra *IstanbulExtra
		expectedErr   error
	}{
		{
			name: "should return error if the size of header.ExtraData is less than IstanbulExtraVanity",
			header: &types.Header{
				ExtraData: []byte{},
			},
			signer:        NewSigner(nil, nil),
			expectedExtra: nil,
			expectedErr: fmt.Errorf(
				"wrong extra size, expected greater than or equal to %d but actual %d",
				IstanbulExtraVanity,
				0,
			),
		},
		{
			name: "should return IstanbulExtra for the header at 1 (ECDSA Serialized Seal)",
			header: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					nil,
					nil,
				),
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return ecdsaValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &SerializedSeal{}
					},
				},
				nil,
			),
			expectedExtra: &IstanbulExtra{
				Validators:           ecdsaValidators,
				ProposerSeal:         testProposerSeal,
				CommittedSeals:       testSerializedSeals1,
				ParentCommittedSeals: nil,
			},
			expectedErr: nil,
		},
		{
			name: "should return IstanbulExtra for the header at 1 (BLS Aggregated Seals)",
			header: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					blsValidators,
					testProposerSeal,
					testAggregatedSeals1,
					nil,
					nil,
				),
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return blsValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &AggregatedSeal{}
					},
				},
				nil,
			),
			expectedExtra: &IstanbulExtra{
				Validators:           blsValidators,
				ProposerSeal:         testProposerSeal,
				CommittedSeals:       testAggregatedSeals1,
				ParentCommittedSeals: nil,
			},
			expectedErr: nil,
		},
		{
			name: "should return IstanbulExtra with ParentCommittedSeals for the header at 2 (ECDSA Serialized Seal)",
			header: &types.Header{
				Number: 2,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					testSerializedSeals2,
					nil,
				),
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return ecdsaValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &SerializedSeal{}
					},
				},
				&MockKeyManager{
					NewEmptyCommittedSealsFunc: func() Seals {
						return &SerializedSeal{}
					},
				},
			),
			expectedExtra: &IstanbulExtra{
				Validators:           ecdsaValidators,
				ProposerSeal:         testProposerSeal,
				CommittedSeals:       testSerializedSeals1,
				ParentCommittedSeals: testSerializedSeals2,
			},
			expectedErr: nil,
		},
		{
			name: "should return IstanbulExtra with ParentCommittedSeals for the header at 2 (BLS Aggregated Seal)",
			header: &types.Header{
				Number: 2,
				ExtraData: getTestExtraBytes(
					blsValidators,
					testProposerSeal,
					testAggregatedSeals1,
					testAggregatedSeals2,
					nil,
				),
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return blsValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &AggregatedSeal{}
					},
				},
				&MockKeyManager{
					NewEmptyCommittedSealsFunc: func() Seals {
						return &AggregatedSeal{}
					},
				},
			),
			expectedExtra: &IstanbulExtra{
				Validators:           blsValidators,
				ProposerSeal:         testProposerSeal,
				CommittedSeals:       testAggregatedSeals1,
				ParentCommittedSeals: testAggregatedSeals2,
			},
			expectedErr: nil,
		},
		{
			name: "should return IstanbulExtra for BLS even if parent committed seals is created by ECDSA",
			header: &types.Header{
				Number: 3,
				ExtraData: getTestExtraBytes(
					blsValidators,
					testProposerSeal,
					testAggregatedSeals1,
					testSerializedSeals1,
					nil,
				),
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return blsValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &AggregatedSeal{}
					},
				},
				&MockKeyManager{
					NewEmptyCommittedSealsFunc: func() Seals {
						return &SerializedSeal{}
					},
				},
			),
			expectedExtra: &IstanbulExtra{
				Validators:           blsValidators,
				ProposerSeal:         testProposerSeal,
				CommittedSeals:       testAggregatedSeals1,
				ParentCommittedSeals: testSerializedSeals1,
			},
			expectedErr: nil,
		},
		{
			name: "should return IstanbulExtra for ECDSA even if parent committed seals is created by BLS",
			header: &types.Header{
				Number: 3,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					testAggregatedSeals1,
					nil,
				),
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return ecdsaValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &SerializedSeal{}
					},
				},
				&MockKeyManager{
					NewEmptyCommittedSealsFunc: func() Seals {
						return &AggregatedSeal{}
					},
				},
			),
			expectedExtra: &IstanbulExtra{
				Validators:           ecdsaValidators,
				ProposerSeal:         testProposerSeal,
				CommittedSeals:       testSerializedSeals1,
				ParentCommittedSeals: testAggregatedSeals1,
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extra, err := test.signer.GetIBFTExtra(test.header)

			assert.Equal(
				t,
				test.expectedExtra,
				extra,
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				err,
			)
		})
	}
}

func TestSignerWriteProposerSeal(t *testing.T) {
	tests := []struct {
		name           string
		header         *types.Header
		signer         *SignerImpl
		expectedHeader *types.Header
		expectedErr    error
	}{
		{
			name: "should return error if GetIBFTExtra fails",
			header: &types.Header{
				ExtraData: []byte{},
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return ecdsaValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &SerializedSeal{}
					},
				},
				nil,
			),
			expectedHeader: nil,
			expectedErr: fmt.Errorf(
				"wrong extra size, expected greater than or equal to %d but actual %d",
				IstanbulExtraVanity,
				0,
			),
		},
		{
			name: "should return error if SignProposerSeal returns error",
			header: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					nil,
					nil,
				),
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return ecdsaValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &SerializedSeal{}
					},
					SignProposerSealFunc: func(b []byte) ([]byte, error) {
						return nil, errTest
					},
				},
				nil,
			),
			expectedHeader: nil,
			expectedErr:    errTest,
		},
		{
			name: "should set ProposerSeal into Header",
			header: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					nil,
					testSerializedSeals1,
					nil,
					nil,
				),
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return ecdsaValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &SerializedSeal{}
					},
					SignProposerSealFunc: func(b []byte) ([]byte, error) {
						return testProposerSeal, nil
					},
				},
				nil,
			),
			expectedHeader: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					nil,
					nil,
				),
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			UseIstanbulHeaderHashInTest(t, test.signer)

			header, err := test.signer.WriteProposerSeal(test.header)

			assert.Equal(
				t,
				test.expectedHeader,
				header,
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				err,
			)
		})
	}
}

func TestSignerEcrecoverFromHeader(t *testing.T) {
	tests := []struct {
		name         string
		header       *types.Header
		signer       *SignerImpl
		expectedAddr types.Address
		expectedErr  error
	}{
		{
			name: "should return error if GetIBFTExtra fails",
			header: &types.Header{
				Number:    0,
				ExtraData: []byte{},
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return ecdsaValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &SerializedSeal{}
					},
				},
				nil,
			),
			expectedAddr: types.ZeroAddress,
			expectedErr: fmt.Errorf(
				"wrong extra size, expected greater than or equal to %d but actual %d",
				IstanbulExtraVanity,
				0,
			),
		},
		{
			name: "should return address",
			header: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					nil,
					nil,
				),
			},
			signer: NewSigner(
				&MockKeyManager{
					NewEmptyValidatorsFunc: func() validators.Validators {
						return ecdsaValidators
					},
					NewEmptyCommittedSealsFunc: func() Seals {
						return &SerializedSeal{}
					},
					EcrecoverFunc: func(b1, b2 []byte) (types.Address, error) {
						return ecdsaValidator1.Address, nil
					},
				},
				nil,
			),
			expectedAddr: ecdsaValidator1.Address,
			expectedErr:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			UseIstanbulHeaderHashInTest(t, test.signer)

			addr, err := test.signer.EcrecoverFromHeader(test.header)

			assert.Equal(
				t,
				test.expectedAddr,
				addr,
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				err,
			)
		})
	}
}

func TestSignerCreateCommittedSeal(t *testing.T) {
	t.Parallel()

	hash := crypto.Keccak256([]byte{0x1})
	sig := crypto.Keccak256([]byte{0x2})

	signer := newTestSingleKeyManagerSigner(
		&MockKeyManager{
			SignCommittedSealFunc: func(b []byte) ([]byte, error) {
				assert.Equal(
					t,
					crypto.Keccak256(wrapCommitHash(hash)),
					b,
				)

				return sig, nil
			},
		},
	)

	res, err := signer.CreateCommittedSeal(hash)

	assert.Equal(t, sig, res)
	assert.NoError(t, err)
}

func TestVerifyCommittedSeal(t *testing.T) {
	t.Parallel()

	hash := crypto.Keccak256([]byte{0x1})
	sig := crypto.Keccak256([]byte{0x2})

	signer := newTestSingleKeyManagerSigner(
		&MockKeyManager{
			VerifyCommittedSealFunc: func(vals validators.Validators, author types.Address, s, h []byte) error {
				assert.Equal(t, ecdsaValidators, vals)
				assert.Equal(t, testAddr1, author)
				assert.Equal(t, sig, s)
				assert.Equal(t, crypto.Keccak256(
					wrapCommitHash(hash[:]),
				), h)

				return errTest
			},
		},
	)

	assert.Equal(
		t,
		errTest,
		signer.VerifyCommittedSeal(
			ecdsaValidators,
			testAddr1,
			sig,
			hash,
		),
	)
}

func TestSignerWriteCommittedSeals(t *testing.T) {
	var round0 uint64 = 0

	tests := []struct {
		name           string
		header         *types.Header
		roundNumber    uint64
		sealMap        map[types.Address][]byte
		keyManager     *MockKeyManager
		expectedHeader *types.Header
		expectedErr    error
	}{
		{
			name:           "should return ErrEmptyCommittedSeals if sealMap is empty",
			header:         &types.Header{},
			roundNumber:    0,
			sealMap:        map[types.Address][]byte{},
			keyManager:     nil,
			expectedHeader: nil,
			expectedErr:    ErrEmptyCommittedSeals,
		},
		{
			name:        "should return error if GetValidators fails",
			header:      &types.Header{},
			roundNumber: 0,
			sealMap: map[types.Address][]byte{
				testAddr1: []byte("test"),
			},
			keyManager:     nil,
			expectedHeader: nil,
			expectedErr: fmt.Errorf(
				"wrong extra size, expected greater than or equal to %d but actual %d",
				IstanbulExtraVanity,
				0,
			),
		},
		{
			name: "should return error if GenerateCommittedSeals fails",
			header: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					&SerializedSeal{},
					nil,
					nil,
				),
			},
			roundNumber: 0,
			sealMap: map[types.Address][]byte{
				testAddr1: []byte("test"),
			},
			keyManager: &MockKeyManager{
				NewEmptyValidatorsFunc: func() validators.Validators {
					return ecdsaValidators
				},
				NewEmptyCommittedSealsFunc: func() Seals {
					return &SerializedSeal{}
				},
				GenerateCommittedSealsFunc: func(m map[types.Address][]byte, v validators.Validators) (Seals, error) {
					return nil, errTest
				},
			},
			expectedHeader: nil,
			expectedErr:    errTest,
		},
		{
			name: "should set CommittedSeals into IBFTExtra",
			header: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					&SerializedSeal{},
					nil,
					nil,
				),
			},
			roundNumber: 0,
			sealMap: map[types.Address][]byte{
				testAddr1: []byte("test"),
			},
			keyManager: &MockKeyManager{
				NewEmptyValidatorsFunc: func() validators.Validators {
					return ecdsaValidators
				},
				NewEmptyCommittedSealsFunc: func() Seals {
					return &SerializedSeal{}
				},
				GenerateCommittedSealsFunc: func(m map[types.Address][]byte, v validators.Validators) (Seals, error) {
					return testSerializedSeals1, nil
				},
			},
			expectedHeader: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					nil,
					&round0,
				),
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			signer := newTestSingleKeyManagerSigner(test.keyManager)

			header, err := signer.WriteCommittedSeals(test.header, test.roundNumber, test.sealMap)

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				err,
			)
			assert.Equal(
				t,
				test.expectedHeader,
				header,
			)
		})
	}
}

func TestSignerVerifyCommittedSeals(t *testing.T) {
	committedSeals := &SerializedSeal{}

	tests := []struct {
		name                    string
		header                  *types.Header
		validators              validators.Validators
		quorumSize              int
		verifyCommittedSealsRes int
		verifyCommittedSealsErr error
		expectedErr             error
	}{
		{
			name: "should return error if VerifyCommittedSeals fails",
			header: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					nil,
					nil,
				),
			},
			validators:              ecdsaValidators,
			quorumSize:              0,
			verifyCommittedSealsRes: 0,
			verifyCommittedSealsErr: errTest,
			expectedErr:             errTest,
		},
		{
			name: "should return error if number of signers is less than quorumSize",
			header: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					nil,
					nil,
				),
			},
			validators:              ecdsaValidators,
			quorumSize:              5,
			verifyCommittedSealsRes: 3,
			verifyCommittedSealsErr: nil,
			expectedErr:             ErrNotEnoughCommittedSeals,
		},
		{
			name: "should succeed",
			header: &types.Header{
				Number: 1,
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					nil,
					nil,
				),
			},
			validators:              ecdsaValidators,
			quorumSize:              5,
			verifyCommittedSealsRes: 6,
			verifyCommittedSealsErr: nil,
			expectedErr:             nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var expectedSig []byte

			signer := newTestSingleKeyManagerSigner(&MockKeyManager{
				NewEmptyValidatorsFunc: func() validators.Validators {
					return test.validators
				},
				NewEmptyCommittedSealsFunc: func() Seals {
					return committedSeals
				},
				VerifyCommittedSealsFunc: func(s Seals, b []byte, v validators.Validators) (int, error) {
					assert.Equal(t, testSerializedSeals1, s)
					assert.Equal(t, ecdsaValidators, v)
					assert.Equal(t, expectedSig, b)

					return test.verifyCommittedSealsRes, test.verifyCommittedSealsErr
				},
			})

			UseIstanbulHeaderHashInTest(t, signer)

			expectedSig = crypto.Keccak256(
				wrapCommitHash(
					test.header.ComputeHash().Hash.Bytes(),
				),
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				signer.VerifyCommittedSeals(
					test.header.Hash,
					committedSeals,
					test.validators,
					test.quorumSize,
				),
			)
		})
	}
}

func TestSignerVerifyParentCommittedSeals(t *testing.T) {
	t.Parallel()

	parentHeaderHash := crypto.Keccak256(types.ZeroAddress.Bytes())

	tests := []struct {
		name                    string
		parentHeader            *types.Header
		header                  *types.Header
		parentValidators        validators.Validators
		quorumSize              int
		mustExist               bool
		verifyCommittedSealsRes int
		verifyCommittedSealsErr error
		expectedErr             error
	}{
		{
			name: "should return error if GetIBFTExtra fails",
			parentHeader: &types.Header{
				Hash: types.BytesToHash(parentHeaderHash),
			},
			header: &types.Header{
				ExtraData: []byte{},
			},
			parentValidators:        ecdsaValidators,
			quorumSize:              0,
			mustExist:               true,
			verifyCommittedSealsRes: 0,
			verifyCommittedSealsErr: nil,
			expectedErr: fmt.Errorf(
				"wrong extra size, expected greater than or equal to %d but actual %d",
				IstanbulExtraVanity,
				0,
			),
		},
		{
			name: "should return error if header doesn't have ParentCommittedSeals and must exist is true",
			parentHeader: &types.Header{
				Hash: types.BytesToHash(parentHeaderHash),
			},
			header: &types.Header{
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					nil,
					nil,
				),
			},
			parentValidators:        ecdsaValidators,
			quorumSize:              0,
			mustExist:               true,
			verifyCommittedSealsRes: 0,
			verifyCommittedSealsErr: nil,
			expectedErr:             ErrEmptyParentCommittedSeals,
		},
		{
			name: "should succeed if header doesn't have ParentCommittedSeals and must exist is false",
			parentHeader: &types.Header{
				Hash: types.BytesToHash(parentHeaderHash),
			},
			header: &types.Header{
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					nil,
					nil,
				),
			},
			parentValidators:        ecdsaValidators,
			quorumSize:              0,
			mustExist:               false,
			verifyCommittedSealsRes: 0,
			verifyCommittedSealsErr: nil,
			expectedErr:             nil,
		},
		{
			name: "should return error if VerifyCommittedSeals fails",
			parentHeader: &types.Header{
				Hash: types.BytesToHash(parentHeaderHash),
			},
			header: &types.Header{
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					testSerializedSeals2,
					nil,
				),
			},
			parentValidators:        ecdsaValidators,
			quorumSize:              0,
			mustExist:               false,
			verifyCommittedSealsRes: 0,
			verifyCommittedSealsErr: errTest,
			expectedErr:             errTest,
		},
		{
			name: "should return ErrNotEnoughCommittedSeals if the number of signers is less than quorum",
			parentHeader: &types.Header{
				Hash: types.BytesToHash(parentHeaderHash),
			},
			header: &types.Header{
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					testSerializedSeals2,
					nil,
				),
			},
			parentValidators:        ecdsaValidators,
			quorumSize:              5,
			mustExist:               false,
			verifyCommittedSealsRes: 2,
			verifyCommittedSealsErr: nil,
			expectedErr:             ErrNotEnoughCommittedSeals,
		},
		{
			name: "should succeed",
			parentHeader: &types.Header{
				Hash: types.BytesToHash(parentHeaderHash),
			},
			header: &types.Header{
				ExtraData: getTestExtraBytes(
					ecdsaValidators,
					testProposerSeal,
					testSerializedSeals1,
					testSerializedSeals2,
					nil,
				),
			},
			parentValidators:        ecdsaValidators,
			quorumSize:              5,
			mustExist:               false,
			verifyCommittedSealsRes: 6,
			verifyCommittedSealsErr: nil,
			expectedErr:             nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			expectedSig := crypto.Keccak256(
				wrapCommitHash(
					test.parentHeader.Hash.Bytes(),
				),
			)

			signer := newTestSingleKeyManagerSigner(&MockKeyManager{
				NewEmptyValidatorsFunc: func() validators.Validators {
					return ecdsaValidators
				},
				NewEmptyCommittedSealsFunc: func() Seals {
					return &SerializedSeal{}
				},
				VerifyCommittedSealsFunc: func(s Seals, b []byte, v validators.Validators) (int, error) {
					assert.Equal(t, testSerializedSeals2, s)
					assert.Equal(t, ecdsaValidators, v)
					assert.Equal(t, expectedSig, b)

					return test.verifyCommittedSealsRes, test.verifyCommittedSealsErr
				},
			})

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				signer.VerifyParentCommittedSeals(
					test.parentHeader.Hash,
					test.header,
					test.parentValidators,
					test.quorumSize,
					test.mustExist,
				),
			)
		})
	}
}

func TestSignerSignIBFTMessage(t *testing.T) {
	t.Parallel()

	msg := []byte("test")
	sig := []byte("signature")

	signer := &SignerImpl{
		keyManager: &MockKeyManager{
			SignIBFTMessageFunc: func(data []byte) ([]byte, error) {
				assert.Equal(t, crypto.Keccak256(msg), data)

				return sig, errTest
			},
		},
	}

	res, err := signer.SignIBFTMessage(msg)

	assert.Equal(
		t,
		sig,
		res,
	)

	assert.Equal(
		t,
		errTest,
		err,
	)
}

func TestEcrecoverFromIBFTMessage(t *testing.T) {
	t.Parallel()

	msg := []byte("test")
	sig := []byte("signature")

	signer := &SignerImpl{
		keyManager: &MockKeyManager{
			EcrecoverFunc: func(b1, b2 []byte) (types.Address, error) {
				assert.Equal(t, sig, b1)
				assert.Equal(t, crypto.Keccak256(msg), b2)

				return testAddr1, errTest
			},
		},
	}

	res, err := signer.EcrecoverFromIBFTMessage(sig, msg)

	assert.Equal(
		t,
		testAddr1,
		res,
	)

	assert.Equal(
		t,
		errTest,
		err,
	)
}

func TestSignerSignIBFTMessageAndEcrecoverFromIBFTMessage(t *testing.T) {
	t.Parallel()

	msg := []byte("message")

	ecdsaKeyManager, _ := newTestECDSAKeyManager(t)
	blsKeyManager, _, _ := newTestBLSKeyManager(t)

	tests := []struct {
		name       string
		keyManager KeyManager
	}{
		{
			name:       "ECDSA Signer",
			keyManager: ecdsaKeyManager,
		},
		{
			name:       "BLS Signer",
			keyManager: blsKeyManager,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			signer := newTestSingleKeyManagerSigner(test.keyManager)

			sig, err := signer.SignIBFTMessage(msg)
			assert.NoError(t, err)

			recovered, err := signer.EcrecoverFromIBFTMessage(sig, msg)
			assert.NoError(t, err)

			assert.Equal(
				t,
				signer.Address(),
				recovered,
			)
		})
	}
}
