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
			name: "FindPeersReq: allows empty key",
			req: &FindPeersReq{
				Key:   "",
				Count: 1,
			},
			valid: true,
		},
		{
			name: "FindPeersReq: valid key",
			req: &FindPeersReq{
				Key:   "16Uiu2HAkzfvGmMz52QdkNYuCNouEJncdWZeuMq7jEPPrPYhzEuJh",
				Count: 16,
			},
			valid: true,
		},
		{
			name: "FindPeersReq: invalid key",
			req: &FindPeersReq{
				Key:   "-1",
				Count: 1,
			},
			valid:    false,
			errorMsg: "invalid FindPeersReq.Key: value does not match regex pattern",
		},
		{
			name: "FindPeersReq: count too small",
			req: &FindPeersReq{
				Key:   "",
				Count: 0,
			},
			valid:    false,
			errorMsg: "invalid FindPeersReq.Count: value must be inside range (0, 17)",
		},
		{
			name: "FindPeersReq: count too big",
			req: &FindPeersReq{
				Key:   "",
				Count: 17,
			},
			valid:    false,
			errorMsg: "invalid FindPeersReq.Count: value must be inside range (0, 17)",
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
