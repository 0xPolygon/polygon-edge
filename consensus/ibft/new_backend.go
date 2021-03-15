package ibft

import "google.golang.org/grpc"

// Backend2 is the IBFT consensus implementation
type Backend2 struct {
	grpc  grpc.Server
	state *state
}
