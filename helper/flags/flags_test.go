package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMultiAddrFromDns(t *testing.T) {

	tests := []struct {
		name       string
		dnsAddress string
		port       int
		err        bool
		outcome    string
	}{
		{
			name:       "Invalid DNS Version",
			dnsAddress: "/dns8/example.io/",
			port:       12345,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Invalid DNS String",
			dnsAddress: "dns4rahul.io",
			port:       12345,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Valid DNS Address with `/` ",
			dnsAddress: "/dns4/rahul.io/",
			port:       12345,
			err:        false,
			outcome:    "/dns4/rahul.io/tcp/12345",
		},
		{
			name:       "Valid DNS Address without `/`",
			dnsAddress: "dns6/example.io",
			port:       12345,
			err:        false,
			outcome:    "/dns6/example.io/tcp/12345",
		},
		{
			name:       "Invalid Port Number",
			dnsAddress: "dns6/example.io",
			port:       100000,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Invalid Host name starting with `-` ",
			dnsAddress: "dns6/-example.io",
			port:       12345,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Invalid Host name starting with `/` ",
			dnsAddress: "dns6//example.io",
			port:       12345,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Invalid Host name  with `/` ",
			dnsAddress: "dns6/example/.io",
			port:       12345,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Invalid Host name  with `-` ",
			dnsAddress: "dns6/example-.io",
			port:       12345,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Missing DNS version",
			dnsAddress: "example.io",
			port:       12345,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Invalid DNS version",
			dnsAddress: "/dns8/example.io",
			port:       12345,
			err:        true,
			outcome:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			multiAddr, err := MultiAddrFromDns(tt.dnsAddress, tt.port)
			if !tt.err {
				assert.NotNil(t, multiAddr, "Multi Address should not be nil")
				assert.Equal(t, multiAddr.String(), tt.outcome)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
