package error_manager

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// dummy error type for test
type TestError struct {
	test string
}

func (t TestError) Error() string {
	return t.test
}

func TestPathParse(t *testing.T) {
	tests := []struct {
		name     string
		status   error
		expected string
	}{
		{
			name:     "nil entry",
			status:   nil,
			expected: statusOk.status,
		},
		{
			name:     "empty",
			status:   Status{status: ""},
			expected: StatusInternalError.status,
		},
		{
			name:     "statusOk",
			status:   statusOk,
			expected: statusOk.status,
		},
		{
			name:     "StatusInternalError",
			status:   StatusInternalError,
			expected: StatusInternalError.status,
		},
		{
			name:     "StatusKubecallError",
			status:   StatusKubecallError,
			expected: StatusKubecallError.status,
		},
		{
			name:     "StatusPagerdutycallError",
			status:   StatusPagerdutycallError,
			expected: StatusPagerdutycallError.status,
		},
		{
			name:     "unknown string entry",
			status:   Status{status: "test"},
			expected: StatusInternalError.status,
		},
		{
			name:     "Other error",
			status:   TestError{test: "test"},
			expected: StatusInternalError.status,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ToString(test.status)
			assert.Equal(t, test.expected, result)
		})
	}

}
