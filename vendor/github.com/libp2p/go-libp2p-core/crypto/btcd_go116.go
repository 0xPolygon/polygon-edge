//go:build !go1.17
// +build !go1.17

package crypto

// This import allows us to force a minimum version of github.com/btcsuite/btcd,
// which otherwise causes problems when `go mod tidy` tries to please Go 1.16
// (which we don't care about any more at this point).
//
// See https://github.com/libp2p/go-libp2p-core/issues/252 for details.
import (
	_ "github.com/btcsuite/btcd/chaincfg"
)
