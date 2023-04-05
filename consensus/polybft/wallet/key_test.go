package wallet

import (
	"testing"

	"github.com/0xPolygon/go-ibft/messages/proto"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_RecoverAddressFromSignature(t *testing.T) {
	t.Parallel()

	for _, account := range []*Account{generateTestAccount(t), generateTestAccount(t), generateTestAccount(t)} {
		key := NewKey(account)
		msgNoSig := &proto.Message{
			From:    key.Address().Bytes(),
			Type:    proto.MessageType_COMMIT,
			Payload: &proto.Message_CommitData{},
		}

		msg, err := key.SignIBFTMessage(msgNoSig)
		require.NoError(t, err)

		payload, err := msgNoSig.PayloadNoSig()
		require.NoError(t, err)

		address, err := RecoverAddressFromSignature(msg.Signature, payload)
		require.NoError(t, err)
		assert.Equal(t, key.Address().Bytes(), address.Bytes())
	}
}

func Test_Sign(t *testing.T) {
	t.Parallel()

	msg := []byte("some message")

	for _, account := range []*Account{generateTestAccount(t), generateTestAccount(t)} {
		key := NewKey(account)
		ser, err := key.SignWithDomain(msg, bls.DomainCheckpointManager)

		require.NoError(t, err)

		sig, err := bls.UnmarshalSignature(ser)
		require.NoError(t, err)

		assert.True(t, sig.Verify(key.raw.Bls.PublicKey(), msg, bls.DomainCheckpointManager))
	}
}

func Test_String(t *testing.T) {
	t.Parallel()

	for _, account := range []*Account{generateTestAccount(t), generateTestAccount(t), generateTestAccount(t)} {
		key := NewKey(account)
		assert.Equal(t, key.Address().String(), key.String())
	}
}
