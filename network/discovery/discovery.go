package discovery

import (
	"context"
	"crypto/ecdsa"
	"log"

	"github.com/umbracle/minimal/helper/enode"
)

// TODO, Is is necessary the Schedule call or we can start once the factory
// method is called?

// TODO, Add another field in the config to pass some information about
// the server (i.e. rlpx address, port, capabilities). Maybe just the *info*
// field in RLPX would be enough

// Backend interface must be implemented for a discovery protocol
type Backend interface {
	// Close closes the backend
	Close() error

	// Deliver returns discovered elements
	Deliver() chan string

	// Schedule starts the discovery
	Schedule()
}

// BackendConfig contains configuration parameters
type BackendConfig struct {
	// Logger to be used by the backend
	Logger *log.Logger

	// Enode is the identification of the node
	Enode *enode.Enode

	// Private key of the node to encrypt/decrypt messages
	Key *ecdsa.PrivateKey

	// Specific configuration parameters for the backend
	Config map[string]interface{}
}

// Factory is the factory function to create a discovery backend
type Factory func(context.Context, *BackendConfig) (Backend, error)
