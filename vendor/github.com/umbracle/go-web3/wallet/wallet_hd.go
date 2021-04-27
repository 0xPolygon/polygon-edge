package wallet

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/tyler-smith/go-bip39"
)

type DerivationPath []uint32

// 0x800000
var decVal = big.NewInt(2147483648)

// DefaultDerivationPath is the default derivation path for Ethereum addresses
var DefaultDerivationPath = DerivationPath{0x80000000 + 44, 0x80000000 + 60, 0x80000000 + 0, 0, 0}

func (d *DerivationPath) Derive(master *hdkeychain.ExtendedKey) (*ecdsa.PrivateKey, error) {
	var err error
	key := master
	for _, n := range *d {
		key, err = key.Child(n)
		if err != nil {
			return nil, err
		}
	}
	priv, err := key.ECPrivKey()
	if err != nil {
		return nil, err
	}
	return priv.ToECDSA(), nil
}

func parseDerivationPath(path string) (*DerivationPath, error) {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return nil, fmt.Errorf("no derivation path")
	}

	// clean all the parts of any trim spaces
	for indx := range parts {
		parts[indx] = strings.TrimSpace(parts[indx])
	}

	// first part has to be an 'm'
	if parts[0] != "m" {
		return nil, fmt.Errorf("first has to be m")
	}

	result := DerivationPath{}
	for _, p := range parts[1:] {
		val := new(big.Int)
		if strings.HasSuffix(p, "'") {
			p = strings.TrimSuffix(p, "'")
			val.Add(val, decVal)
		}

		bigVal, ok := new(big.Int).SetString(p, 0)
		if !ok {
			return nil, fmt.Errorf("invalid path")
		}
		val.Add(val, bigVal)

		// TODO, limit to uint32
		if !val.IsUint64() {
			return nil, fmt.Errorf("bad")
		}
		result = append(result, uint32(val.Uint64()))
	}

	return &result, nil
}

func NewWalletFromMnemonic(mnemonic string) (*Key, error) {
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, "")
	if err != nil {
		return nil, err
	}
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}
	priv, err := DefaultDerivationPath.Derive(masterKey)
	if err != nil {
		return nil, err
	}
	return newKey(priv), nil
}
