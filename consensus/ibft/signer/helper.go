package signer

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/btcsuite/btcd/btcec"
	"github.com/coinbase/kryptology/pkg/signatures/bls/bls_sig"
	"github.com/umbracle/fastrlp"
)

func obtainOrCreateECDSAKey(manager secrets.SecretsManager) (*ecdsa.PrivateKey, error) {
	if manager.HasSecret(secrets.ValidatorKey) {
		keyBytes, err := manager.GetSecret(secrets.ValidatorKey)
		if err != nil {
			return nil, err
		}

		return crypto.BytesToPrivateKey(keyBytes)
	}

	// create new key
	key, keyBytes, err := crypto.GenerateAndEncodePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("unable to generate validator key for Secrets Manager, %w", err)
	}

	err = manager.SetSecret(secrets.ValidatorKey, keyBytes)
	if err != nil {
		return nil, fmt.Errorf("unable to save validator key to Secrets Manager, %w", err)
	}

	return key, nil
}

func ecdsaToBLS(key *ecdsa.PrivateKey) (*bls_sig.SecretKey, error) {
	blsPop := bls_sig.NewSigPop()
	keyBytes := (*btcec.PrivateKey)(key).Serialize()

	_, sk, err := blsPop.KeygenWithSeed(keyBytes)
	if err != nil {
		return nil, err
	}

	return sk, nil
}

func calculateHeaderHash(h *types.Header) types.Hash {
	arena := fastrlp.DefaultArenaPool.Get()
	defer fastrlp.DefaultArenaPool.Put(arena)

	vv := arena.NewArray()
	vv.Set(arena.NewBytes(h.ParentHash.Bytes()))
	vv.Set(arena.NewBytes(h.Sha3Uncles.Bytes()))
	vv.Set(arena.NewBytes(h.Miner.Bytes()))
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

func commitMsg(b []byte) []byte {
	// message that the nodes need to sign to commit to a block
	// hash with COMMIT_MSG_CODE which is the same value used in quorum
	return crypto.Keccak256(b, []byte{byte(proto.MessageReq_Commit)})
}

func signMsg(key *ecdsa.PrivateKey, msg *proto.MessageReq) error {
	signMsg, err := msg.PayloadNoSig()
	if err != nil {
		return err
	}

	sig, err := crypto.Sign(key, crypto.Keccak256(signMsg))
	if err != nil {
		return err
	}

	msg.Signature = hex.EncodeToHex(sig)

	return nil
}

func ValidateMsg(msg *proto.MessageReq) error {
	signMsg, err := msg.PayloadNoSig()
	if err != nil {
		return err
	}

	buf, err := hex.DecodeHex(msg.Signature)
	if err != nil {
		return err
	}

	addr, err := ecrecoverImpl(buf, signMsg)
	if err != nil {
		return err
	}

	msg.From = addr.String()

	return nil
}

// TODO: move to IBFT?
func ecrecoverImpl(sig, msg []byte) (types.Address, error) {
	pub, err := crypto.RecoverPubkey(sig, crypto.Keccak256(msg))
	if err != nil {
		return types.Address{}, err
	}

	return crypto.PubKeyToAddress(pub), nil
}
