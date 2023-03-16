package timeutils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimeNow(t *testing.T) {
	t.Parallel()

	timeNow := Now()
	require.Equal(t, time.UTC.String(), timeNow.Location().String())
}
