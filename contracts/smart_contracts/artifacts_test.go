package smartcontracts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArtifacts(t *testing.T) {
	artifact, err := ReadArtifact("sidechain", "Validator")
	assert.NoError(t, err)
	assert.NotNil(t, artifact)
}
