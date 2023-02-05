package wallet

import (
	"testing"

	"github.com/0xPolygon/go-ibft/messages/proto"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_RecoverAddressFromSignature(t *testing.T) {
	for _, account := range []*Account{GenerateAccount(), GenerateAccount(), GenerateAccount()} {
		key := NewKey(account)
		msgNoSig := &proto.Message{
			From:    key.Address().Bytes(),
			Type:    proto.MessageType_COMMIT,
			Payload: &proto.Message_CommitData{},
		}

		msg, err := key.SignEcdsaMessage(msgNoSig)
		require.NoError(t, err)

		payload, err := msgNoSig.PayloadNoSig()
		require.NoError(t, err)

		address, err := RecoverAddressFromSignature(msg.Signature, payload)
		require.NoError(t, err)
		assert.Equal(t, key.Address().Bytes(), address.Bytes())
	}
}

func Test_Sign(t *testing.T) {
	msg := []byte("some message")

	for _, account := range []*Account{GenerateAccount(), GenerateAccount()} {
		key := NewKey(account)
		ser, err := key.Sign(msg)

		require.NoError(t, err)

		sig, err := bls.UnmarshalSignature(ser)
		require.NoError(t, err)

		assert.True(t, sig.Verify(key.raw.Bls.PublicKey(), msg))
	}
}

func Test_String(t *testing.T) {
	for _, account := range []*Account{GenerateAccount(), GenerateAccount(), GenerateAccount()} {
		key := NewKey(account)
		assert.Equal(t, key.Address().String(), key.String())
	}
}
