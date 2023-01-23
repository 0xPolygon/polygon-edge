package contractsapi

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

type method interface {
	EncodeAbi() ([]byte, error)
	DecodeAbi(buf []byte) error
}

func TestEncoding_Method(t *testing.T) {
	t.Parallel()

	cases := []method{
		// empty commit
		&Commit{
			Commitment: &Commitment{
				StartID: big.NewInt(1),
				EndID:   big.NewInt(1),
				Root:    types.EmptyRootHash,
			},
			Signature: []byte{},
			Bitmap:    []byte{},
		},
		// empty commit epoch
		&CommitEpoch{
			ID: big.NewInt(1),
			Epoch: &Epoch{
				StartBlock: big.NewInt(1),
				EndBlock:   big.NewInt(1),
			},
			Uptime: &Uptime{
				EpochID: big.NewInt(1),
				UptimeData: []*UptimeData{
					{
						Validator:    types.Address{0x1},
						SignedBlocks: big.NewInt(1),
					},
				},
				TotalBlocks: big.NewInt(1),
			},
		},
	}

	for _, c := range cases {
		res, err := c.EncodeAbi()
		require.NoError(t, err)

		// use reflection to create another type and decode
		val := reflect.New(reflect.TypeOf(c).Elem()).Interface()
		obj, ok := val.(method)
		require.True(t, ok)

		err = obj.DecodeAbi(res)
		require.NoError(t, err)
		require.Equal(t, obj, c)
	}
}

func TestEncoding_Struct(t *testing.T) {
	t.Parallel()

	commitment := &Commitment{
		StartID: big.NewInt(1),
		EndID:   big.NewInt(10),
		Root:    types.StringToHash("hash"),
	}

	encoding, err := commitment.EncodeAbi()
	require.NoError(t, err)

	commitmentDecoded := &Commitment{}

	require.NoError(t, commitmentDecoded.DecodeAbi(encoding))
	require.Equal(t, commitment.StartID, commitmentDecoded.StartID)
	require.Equal(t, commitment.EndID, commitmentDecoded.EndID)
	require.Equal(t, commitment.Root, commitmentDecoded.Root)
}

func TestEncoding_Contract(t *testing.T) {
	t.Parallel()

	StateReceiverContract.Commit.Commitment = &Commitment{StartID: big.NewInt(1), EndID: big.NewInt(10), Root: types.StringToHash("hash")}
	StateReceiverContract.Commit.Signature = []byte{11, 12}
	StateReceiverContract.Commit.Bitmap = []byte{0, 1}

	data, err := StateReceiverContract.Commit.EncodeAbi()
	require.NoError(t, err)

	StateReceiverContract.Commit = Commit{}

	require.NoError(t, StateReceiverContract.Commit.DecodeAbi(data))
	require.Equal(t, big.NewInt(1), StateReceiverContract.Commit.Commitment.StartID)
	require.Equal(t, big.NewInt(10), StateReceiverContract.Commit.Commitment.EndID)
	require.Equal(t, []byte{11, 12}, StateReceiverContract.Commit.Signature)
	require.Equal(t, []byte{0, 1}, StateReceiverContract.Commit.Bitmap)
}
