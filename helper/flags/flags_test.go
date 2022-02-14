package flags

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMultiAddrFromDns(t *testing.T) {
	port := 12345

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
			port:       port,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Invalid DNS String",
			dnsAddress: "dns4rahul.io",
			port:       port,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Valid DNS Address with `/` ",
			dnsAddress: "/dns4/rahul.io/",
			port:       port,
			err:        false,
			outcome:    fmt.Sprintf("/dns4/rahul.io/tcp/%d", port),
		},
		{
			name:       "Valid DNS Address without `/`",
			dnsAddress: "dns6/example.io",
			port:       port,
			err:        false,
			outcome:    fmt.Sprintf("/dns6/example.io/tcp/%d", port),
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
			port:       port,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Invalid Host name starting with `/` ",
			dnsAddress: "dns6//example.io",
			port:       port,
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
			port:       port,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Missing DNS version",
			dnsAddress: "example.io",
			port:       port,
			err:        true,
			outcome:    "",
		},
		{
			name:       "Invalid DNS version",
			dnsAddress: "/dns8/example.io",
			port:       port,
			err:        true,
			outcome:    "",
		},
		{
			name:       "valid long domain suffix",
			dnsAddress: "dns/validator-1.foo.technology",
			port:       port,
			err:        false,
			outcome:    fmt.Sprintf("/dns/validator-1.foo.technology/tcp/%d", port),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			multiAddr, err := MultiAddrFromDNS(tt.dnsAddress, tt.port)
			if !tt.err {
				assert.NotNil(t, multiAddr, "Multi Address should not be nil")
				assert.Equal(t, multiAddr.String(), tt.outcome)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
