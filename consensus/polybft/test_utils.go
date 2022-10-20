package polybft

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"

	hclog "github.com/hashicorp/go-hclog"

	"github.com/stretchr/testify/require"
)

func createSignature(t *testing.T, accounts []*wallet.Account, hash types.Hash) *Signature {
	t.Helper()

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

func newTestLogger() hclog.Logger {
	return hclog.New(&hclog.LoggerOptions{
		Name: "polygon",
		//Level: config.LogLevel,
	})
}

func generateStateSyncEvents(t *testing.T, eventsCount int, startIdx uint64) []*StateSyncEvent {
	t.Helper()

	stateSyncEvents := make([]*StateSyncEvent, eventsCount)
	for i := 0; i < eventsCount; i++ {
		stateSyncEvents[i] = &StateSyncEvent{
			ID:     startIdx + uint64(i),
			Sender: ethgo.Address(types.StringToAddress(fmt.Sprintf("0x5%d", i))),
			Data:   generateRandomBytes(t),
		}
	}

	return stateSyncEvents
}

// generateRandomBytes generates byte array with random data of 32 bytes length
func generateRandomBytes(t *testing.T) (result []byte) {
	t.Helper()

	result = make([]byte, types.HashLength)
	_, err := rand.Reader.Read(result)
	require.NoError(t, err, "Cannot generate random byte array content.")

	return
}

type testHeadersMap struct {
	headersByNumber map[uint64]*types.Header
}

func (t *testHeadersMap) addHeader(header *types.Header) {
	if t.headersByNumber == nil {
		t.headersByNumber = map[uint64]*types.Header{}
	}

	t.headersByNumber[header.Number] = header
}

func (t *testHeadersMap) getHeader(number uint64) *types.Header {
	return t.headersByNumber[number]
}

func (t *testHeadersMap) getHeaders() []*types.Header {
	headers := make([]*types.Header, 0, len(t.headersByNumber))
	for _, header := range t.headersByNumber {
		headers = append(headers, header)
	}

	return headers
}
