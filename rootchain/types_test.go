package rootchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifiedSAM_Signatures(t *testing.T) {
	signature := []byte("signature")
	verifiedSAM := VerifiedSAM{
		SAM{
			Signature: signature,
		},
	}

	signatures := verifiedSAM.Signatures()
	if len(signatures) < 1 {
		t.Fatalf("invalid signatures size")
	}

	assert.Len(t, signatures, 1)
	assert.Equal(t, signatures[0], signature)
}
