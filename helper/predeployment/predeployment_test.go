package predeployment

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestNormalizeArgs(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name           string
		arguments      []string
		expectedResult []interface{}
	}{
		{
			"simple type arguments",
			[]string{"argument"},
			[]interface{}{"argument"},
		},
		{
			"array of simple type arguments",
			[]string{"argument 1", "argument 2"},
			[]interface{}{"argument 1", "argument 2"},
		},
		{
			"structure as argument",
			[]string{"[argument 1]"},
			[]interface{}{[]interface{}{"argument 1"}},
		},
		{
			"structure with regular types",
			[]string{"[argument 1]", "argument 2"},
			[]interface{}{[]interface{}{"argument 1"}, "argument 2"},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.expectedResult,
				normalizeConstructorArguments(testCase.arguments),
			)
		})
	}
}
