package keystore

import (
	"crypto/ecdsa"
)

// Keystore stores the key for the enode
type Keystore interface {
	Get() (*ecdsa.PrivateKey, error)
}
