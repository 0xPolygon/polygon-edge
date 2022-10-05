package polybft

import (
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"testing"

	"github.com/stretchr/testify/require"
)

func createSignature(t *testing.T, accounts []*wallet.Account, hash types.Hash) *Signature {
	var signatures bls.Signatures
	var bmp bitmap.Bitmap
	for i, x := range accounts {
		bmp.Set(uint64(i))
		src, err := x.Bls.Sign(hash[:])
		require.NoError(t, err)
		signatures = append(signatures, src)
	}
	aggs, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)
	return &Signature{AggregatedSignature: aggs, Bitmap: bmp}
}
