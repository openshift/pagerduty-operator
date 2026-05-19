package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFedrampEnv(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		envSet    bool
		expected  bool
		expectErr bool
	}{
		{
			name:     "unset env returns false",
			envSet:   false,
			expected: false,
		},
		{
			name:     "true returns true",
			envValue: "true",
			envSet:   true,
			expected: true,
		},
		{
			name:     "false returns false",
			envValue: "false",
			envSet:   true,
			expected: false,
		},
		{
			name:     "TRUE returns true",
			envValue: "TRUE",
			envSet:   true,
			expected: true,
		},
		{
			name:     "1 returns true",
			envValue: "1",
			envSet:   true,
			expected: true,
		},
		{
			name:     "0 returns false",
			envValue: "0",
			envSet:   true,
			expected: false,
		},
		{
			name:      "invalid value returns error",
			envValue:  "invalid",
			envSet:    true,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envSet {
				t.Setenv("FEDRAMP", tt.envValue)
			}

			result, err := ParseFedrampEnv()

			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
