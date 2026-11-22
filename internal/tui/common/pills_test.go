package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrapPills(t *testing.T) {
	tests := []struct {
		name     string
		pills    []string
		width    int
		expected string
	}{
		{name: "no pills", pills: nil, width: 20, expected: ""},
		{name: "empty pills are skipped", pills: []string{"", "aa", ""}, width: 20, expected: "aa"},
		{
			name:     "all fit on one row",
			pills:    []string{"aa", "bb", "cc"},
			width:    8,
			expected: "aa bb cc",
		},
		{
			name:     "wraps when the next pill won't fit",
			pills:    []string{"aa", "bb", "cc"},
			width:    7,
			expected: "aa bb\n\ncc",
		},
		{
			name:     "a pill wider than width gets its own row",
			pills:    []string{"aa", "bbbbbb", "cc"},
			width:    4,
			expected: "aa\n\nbbbbbb\n\ncc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, WrapPills(tt.pills, tt.width))
		})
	}
}
