package predeployment

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestExtraction(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		input          string
		expectedOutput interface{}
	}{
		{
			"a",
			"a",
		},
		{
			"[a]",
			[]interface{}{"a"},
		},
		{
			"[[a]]",
			[]interface{}{[]interface{}{"a"}},
		},
		{
			"[[a],[b]]",
			[]interface{}{[]interface{}{"a"}, []interface{}{"b"}},
		},
		{
			"[[[a]]]",
			[]interface{}{[]interface{}{[]interface{}{"a"}}},
		},
	}

	for index, testCase := range testTable {
		testCase := testCase

		t.Run(fmt.Sprintf("extraction test %d", index), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.expectedOutput, extractValue(testCase.input))
		})
	}
}
