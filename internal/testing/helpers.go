package testing

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestContext returns a context with a reasonable timeout for tests.
func TestContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// AssertNoError fails the test if err is not nil.
func AssertNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		if len(msgAndArgs) > 0 {
			t.Errorf("%v: unexpected error: %v", msgAndArgs[0], err)
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

// AssertError fails the test if err is nil.
func AssertError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		if len(msgAndArgs) > 0 {
			t.Errorf("%v: expected error but got nil", msgAndArgs[0])
		} else {
			t.Errorf("expected error but got nil")
		}
	}
}

// AssertContains fails the test if s does not contain substr.
func AssertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}
