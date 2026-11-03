package updater_test

import (
	"testing"

	"resonamesh/internal/updater"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"0.2.0", "0.1.0", 1},
		{"0.1.0", "0.2.0", -1},
		{"0.1.0", "0.1.0", 0},
		{"v0.2.0", "0.1.0", 1},
		{"1.0.0", "0.9.9", 1},
		{"0.1.1", "0.1.0", 1},
		{"0.1.0", "0.1.0-alpha", 1}, // release > alpha
		{"0.1.0-beta", "0.1.0-alpha", 1},
		{"0.1.0-alpha", "0.1.0-beta", -1},
		{"0.2.0-alpha", "0.1.0", 1},
	}

	for _, tc := range tests {
		actual := updater.CompareVersions(tc.v1, tc.v2)
		if actual != tc.expected {
			t.Errorf("CompareVersions(%q, %q) = %d; want %d", tc.v1, tc.v2, actual, tc.expected)
		}
	}
}
