package keystore

import (
	"crypto/ecdsa"
)

// Keystore stores the key for the enode
type Keystore interface {
	Put(*ecdsa.PrivateKey) error
	Get() (*ecdsa.PrivateKey, bool, error)
}
