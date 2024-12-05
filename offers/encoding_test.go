package offers

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

const formatStringTestJson = "format-string-test.json"

type offerFormatTestVector struct {
	Comment string `json:"comment"`
	Valid   bool   `json:"valid"`
	String  string `json:"string"`
}

// TestOfferStringEncoding tests decoding of the test vectors for offer strings.
// Note that the following test cases are missing:
//
// {
// "comment": "A complete string is valid",
// "valid": true,
// "string": "lno1pqps7sjqpgtyzm3qv4uxzmtsd3jjqer9wd3hy6tsw35k7msjzfpy7nz5yqcnygrfdej82um5wf5k2uckyypwa3eyt44h6txtxquqh7lz5djge4afgfjn7k4rgrkuag0jsd5xvxg"
// },
// {
// "comment": "Uppercase is valid",
// "valid": true,
// "string": "LNO1PQPS7SJQPGTYZM3QV4UXZMTSD3JJQER9WD3HY6TSW35K7MSJZFPY7NZ5YQCNYGRFDEJ82UM5WF5K2UCKYYPWA3EYT44H6TXTXQUQH7LZ5DJGE4AFGFJN7K4RGRKUAG0JSD5XVXG"
// },
// {
// "comment": "+ can join anywhere",
// "valid": true,
// "string": "l+no1pqps7sjqpgtyzm3qv4uxzmtsd3jjqer9wd3hy6tsw35k7msjzfpy7nz5yqcnygrfdej82um5wf5k2uckyypwa3eyt44h6txtxquqh7lz5djge4afgfjn7k4rgrkuag0jsd5xvxg"
// },
// {
// "comment": "Multiple + can join",
// "valid": true,
// "string": "lno1pqps7sjqpgt+yzm3qv4uxzmtsd3jjqer9wd3hy6tsw3+5k7msjzfpy7nz5yqcn+ygrfdej82um5wf5k2uckyypwa3eyt44h6txtxquqh7lz5djge4afgfjn7k4rgrkuag0jsd+5xvxg"
// },
// {
// "comment": "+ can be followed by whitespace",
// "valid": true,
// "string": "lno1pqps7sjqpgt+ yzm3qv4uxzmtsd3jjqer9wd3hy6tsw3+  5k7msjzfpy7nz5yqcn+\nygrfdej82um5wf5k2uckyypwa3eyt44h6txtxquqh7lz5djge4afgfjn7k4rgrkuag0jsd+\r\n 5xvxg"
// },
// {
// "comment": "+ can be followed by whitespace, UPPERCASE",
// "valid": true,
// "string": "LNO1PQPS7SJQPGT+ YZM3QV4UXZMTSD3JJQER9WD3HY6TSW3+  5K7MSJZFPY7NZ5YQCN+\nYGRFDEJ82UM5WF5K2UCKYYPWA3EYT44H6TXTXQUQH7LZ5DJGE4AFGFJN7K4RGRKUAG0JSD+\r\n 5XVXG"
// },
func TestOfferStringEncoding(t *testing.T) {
	vectorBytes, err := ioutil.ReadFile(formatStringTestJson)
	require.NoError(t, err, "read file")

	var testCases []*offerFormatTestVector
	require.NoError(t, json.Unmarshal(vectorBytes, &testCases))

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Comment, func(t *testing.T) {
			_, err := DecodeOfferStr(testCase.String)
			require.Equal(t, testCase.Valid, err == nil,
				"error check: %v", err)
		})
	}
}
