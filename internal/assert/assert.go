package assert

import (
	"strings"
	"testing"
)

// Equal fails the test if actual and expected are not equal.
func Equal[T comparable](t *testing.T, actual, expected T) {
	// t.Helper() makes failures report the caller's line number, not this one.
	t.Helper()

	if actual != expected {
		t.Errorf("got: %v; want: %v", actual, expected)
	}
}

// StringContains fails the test if actual doesn't contain expectedSubstring.
func StringContains(t *testing.T, actual, expectedSubstring string) {
	t.Helper()

	if !strings.Contains(actual, expectedSubstring) {
		t.Errorf("got: %q; expected to contain: %q", actual, expectedSubstring)
	}
}

// NilError fails the test if actual is a non-nil error.
func NilError(t *testing.T, actual error) {
	t.Helper()

	if actual != nil {
		t.Errorf("got: %v; expected: nil", actual)
	}
}
