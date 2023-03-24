package config

import "testing"

// Test Test_SanitizeRPCEndpoint
func Test_SanitizeRPCEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			"url with port",
			"http://localhost:10001",
			"http://localhost:10001",
		},
		{
			"all interfaces with port without schema",
			"0.0.0.0:10001",
			"http://127.0.0.1:10001",
		},
		{
			"url without port",
			"http://127.0.0.1",
			"http://127.0.0.1",
		},
		{
			"empty endpoint",
			"",
			"http://127.0.0.1:8545",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := SanitizeRPCEndpoint(tt.endpoint); got != tt.want {
				t.Errorf("sanitizeRPCEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}
