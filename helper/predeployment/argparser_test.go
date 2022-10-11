package predeployment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseArguments_validInput(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name           string
		arguments      []string
		expectedResult []interface{}
	}{
		{
			"simple type arguments",
			[]string{
				"argument",
			},
			[]interface{}{
				"argument",
			},
		},
		{
			"array of simple type arguments",
			[]string{
				"argument 1",
				"argument 2",
			},
			[]interface{}{
				"argument 1",
				"argument 2",
			},
		},
		{
			"structure as argument with double quotes",
			[]string{
				`["argument 1"]`,
			},
			[]interface{}{
				[]interface{}{
					"argument 1",
				},
			},
		},
		{
			"structure with regular types",
			[]string{
				`["argument 1"]`,
				"argument 2",
			},
			[]interface{}{
				[]interface{}{
					"argument 1",
				},
				"argument 2",
			},
		},
		{
			"structure with arrays with nested object",
			[]string{
				`[["struct1-1", "struct1-2"], ["struct2-1", "struct2-2"]]`, // array of struct
				`["string1", "string2"]`,                                   // array of string
			},
			[]interface{}{
				[]interface{}{
					[]interface{}{
						"struct1-1",
						"struct1-2",
					},
					[]interface{}{
						"struct2-1",
						"struct2-2",
					},
				},
				[]interface{}{
					"string1",
					"string2",
				},
			},
		},
		{
			"structure with the values containing special characters",
			[]string{
				// outermost bracket is for array
				`["opening square bracket is \["]`,
				`["closing square bracket is \]"]`,
				`["comma is \,"]`,
				`["double quote is \""]`,
				`["multiple spaces are  "]`,
				`["backslash is \\"]`,
			},
			[]interface{}{
				[]interface{}{
					`opening square bracket is [`,
				},
				[]interface{}{
					`closing square bracket is ]`,
				},
				[]interface{}{
					`comma is ,`,
				},
				[]interface{}{
					"double quote is \"",
				},
				[]interface{}{
					`multiple spaces are  `,
				},
				[]interface{}{
					`backslash is \`,
				},
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			res, err := ParseArguments(testCase.arguments)

			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedResult, res)
		})
	}
}
