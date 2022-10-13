package polybftcontracts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArtifacts(t *testing.T) {
	artifact, err := ReadArtifact("sidechain", "Validator")
	assert.NoError(t, err)
	assert.NotNil(t, artifact)
}

func TestArtifactsError(t *testing.T) {
	_, err := ReadArtifact("sidechaine", "Validator")
	assert.Error(t, err)
}
