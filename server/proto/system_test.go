package proto

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/validate"
	"github.com/stretchr/testify/assert"
)

func TestRequestValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		req      validate.Validator
		valid    bool
		errorMsg string
	}{
		{
			name: "PeersAddRequest: empty id",
			req: &PeersAddRequest{
				Id: "",
			},
			valid:    false,
			errorMsg: "PeersAddRequest.Id: value does not match regex pattern",
		},
		{
			name: "PeersAddRequest: invalid id format",
			req: &PeersAddRequest{
				Id: "11111",
			},
			valid:    false,
			errorMsg: "PeersAddRequest.Id: value does not match regex pattern",
		},
		{
			name: "PeersAddRequest: valid id format",
			req: &PeersAddRequest{
				Id: "/ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAm698AJQJJjZj4ef8PPmE1YDnPAuZnf3wGCp7ZyM3uhcAu",
			},
			valid: true,
		},
		{
			name: "PeersAddRequest: valid id format relay circuit",
			req: &PeersAddRequest{
				Id: "/ip4/198.51.100.0/tcp/4242/p2p/QmRelay/p2p-circuit/p2p/QmRelayedPeer",
			},
			valid: true,
		},
		{
			name: "PeersAddRequest: valid id format dns4",
			req: &PeersAddRequest{
				Id: "/dns4/ams-2.bootstrap.libp2p.io/tcp/443/wss/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
			},
			valid: true,
		},
		{
			name: "PeersStatusRequest: empty id",
			req: &PeersStatusRequest{
				Id: "",
			},
			valid:    false,
			errorMsg: "PeersStatusRequest.Id: value does not match regex pattern",
		},
		{
			name: "PeersStatusRequest: invalid id format",
			req: &PeersStatusRequest{
				Id: "-1",
			},
			valid:    false,
			errorMsg: "PeersStatusRequest.Id: value does not match regex pattern",
		},
		{
			name: "PeersStatusRequest: valid id format",
			req: &PeersStatusRequest{
				Id: "16Uiu2HAkzfvGmMz52QdkNYuCNouEJncdWZeuMq7jEPPrPYhzEuJh",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.req.ValidateAll()
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			}
		})
	}
}
