package wallet

import (
	"crypto/ecdsa"

	"github.com/btcsuite/btcd/btcec"
)

func ParsePrivateKey(buf []byte) (*ecdsa.PrivateKey, error) {
	prv, _ := btcec.PrivKeyFromBytes(S256, buf)
	return prv.ToECDSA(), nil
}

func NewWalletFromPrivKey(p []byte) (*Key, error) {
	priv, err := ParsePrivateKey(p)
	if err != nil {
		return nil, err
	}
	return newKey(priv), nil
}
