package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportedServiceManager(t *testing.T) {
	testTable := []struct {
		name        string
		serviceName SecretsManagerType
		supported   bool
	}{
		{
			"Valid local secrets manager",
			Local,
			true,
		},
		{
			"Valid Hashicorp Vault secrets manager",
			HashicorpVault,
			true,
		},
		{
			"Valid AWS SSM secrets manager",
			AWSSSM,
			true,
		},
		{
			"Valid GCP secrets manager",
			GCPSSM,
			true,
		},
		{
			"Invalid secrets manager",
			"MarsSecretsManager",
			false,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(
				t,
				testCase.supported,
				SupportedServiceManager(testCase.serviceName),
			)
		})
	}
}
