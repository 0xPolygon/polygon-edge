package signer

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/coinbase/kryptology/pkg/signatures/bls/bls_sig"
	"github.com/umbracle/fastrlp"
)

const (
	// legacyCommitCode is the value that is contained in
	// legacy committed seals, so it needs to be preserved in order
	// for new clients to read old committed seals
	legacyCommitCode = 2
)

func wrapCommitHash(b []byte) []byte {
	return crypto.Keccak256(b, []byte{byte(legacyCommitCode)})
}

func getOrCreateECDSAKey(manager secrets.SecretsManager) (*ecdsa.PrivateKey, error) {
	if !manager.HasSecret(secrets.ValidatorKey) {
		if _, err := helper.InitECDSAValidatorKey(manager); err != nil {
			return nil, err
		}
	}

	keyBytes, err := manager.GetSecret(secrets.ValidatorKey)
	if err != nil {
		return nil, err
	}

	return crypto.BytesToECDSAPrivateKey(keyBytes)
}

func getOrCreateBLSKey(manager secrets.SecretsManager) (*bls_sig.SecretKey, error) {
	if !manager.HasSecret(secrets.ValidatorBLSKey) {
		if _, err := helper.InitBLSValidatorKey(manager); err != nil {
			return nil, err
		}
	}

	keyBytes, err := manager.GetSecret(secrets.ValidatorBLSKey)
	if err != nil {
		return nil, err
	}

	return crypto.BytesToBLSSecretKey(keyBytes)
}

func calculateHeaderHash(h *types.Header) types.Hash {
	arena := fastrlp.DefaultArenaPool.Get()
	defer fastrlp.DefaultArenaPool.Put(arena)

	vv := arena.NewArray()
	vv.Set(arena.NewBytes(h.ParentHash.Bytes()))
	vv.Set(arena.NewBytes(h.Sha3Uncles.Bytes()))
	vv.Set(arena.NewBytes(h.Miner[:]))
	vv.Set(arena.NewBytes(h.StateRoot.Bytes()))
	vv.Set(arena.NewBytes(h.TxRoot.Bytes()))
	vv.Set(arena.NewBytes(h.ReceiptsRoot.Bytes()))
	vv.Set(arena.NewBytes(h.LogsBloom[:]))
	vv.Set(arena.NewUint(h.Difficulty))
	vv.Set(arena.NewUint(h.Number))
	vv.Set(arena.NewUint(h.GasLimit))
	vv.Set(arena.NewUint(h.GasUsed))
	vv.Set(arena.NewUint(h.Timestamp))
	vv.Set(arena.NewCopyBytes(h.ExtraData))

	buf := keccak.Keccak256Rlp(nil, vv)

	return types.BytesToHash(buf)
}

func ecrecoverImpl(sig, msg []byte) (types.Address, error) {
	pub, err := crypto.RecoverPubkey(sig, crypto.Keccak256(msg))
	if err != nil {
		return types.Address{}, err
	}

	return crypto.PubKeyToAddress(pub), nil
}

func newKeyManagerFromType(
	secretManager secrets.SecretsManager,
	validatorType validators.ValidatorType,
) (KeyManager, error) {
	switch validatorType {
	case validators.ECDSAValidatorType:
		return NewECDSAKeyManager(secretManager)
	case validators.BLSValidatorType:
		return NewBLSKeyManager(secretManager)
	default:
		return nil, fmt.Errorf("unsupported validator type: %s", validatorType)
	}
}

func NewSignerFromType(
	secretManager secrets.SecretsManager,
	validatorType validators.ValidatorType,
) (Signer, error) {
	km, err := newKeyManagerFromType(secretManager, validatorType)
	if err != nil {
		return nil, err
	}

	return NewSigner(km), nil
}
