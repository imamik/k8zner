package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTransientError(t *testing.T) {
	t.Parallel()

	t.Run("recognizes EOF", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("unexpected EOF"))
	})

	t.Run("recognizes connection refused", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("dial tcp 10.0.0.1:6443: connection refused"))
	})

	t.Run("recognizes connection reset", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("read: connection reset by peer"))
	})

	t.Run("recognizes i/o timeout", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("i/o timeout"))
	})

	t.Run("recognizes no such host", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("dial tcp: lookup foo.bar: no such host"))
	})

	t.Run("recognizes TLS handshake timeout", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("net/http: TLS handshake timeout"))
	})

	t.Run("recognizes context deadline exceeded", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("context deadline exceeded"))
	})

	t.Run("rejects unknown errors", func(t *testing.T) {
		t.Parallel()
		assert.False(t, isTransientError("permission denied"))
		assert.False(t, isTransientError("resource not found"))
		assert.False(t, isTransientError("invalid configuration"))
		assert.False(t, isTransientError(""))
	})
}
