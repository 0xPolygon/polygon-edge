package ibft

import "google.golang.org/grpc"

// Backend2 is the IBFT consensus implementation
type Backend2 struct {
	grpc  grpc.Server
	state *state
}

func (b *Backend2) run() {

	// start proposals queue, this is where we send local sealed blocks
	// start the queue to read grpc messages from other sealers.

}
