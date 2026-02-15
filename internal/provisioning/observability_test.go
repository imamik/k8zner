package provisioning

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockObserver is a test implementation of Observer that records messages.
type MockObserver struct {
	messages []string
}

func NewMockObserver() *MockObserver {
	return &MockObserver{
		messages: make([]string, 0),
	}
}

func (m *MockObserver) Printf(format string, _ ...interface{}) {
	m.messages = append(m.messages, format)
}

func TestConsoleObserver_Printf_Basic(_ *testing.T) {
	observer := NewConsoleObserver()
	// Should not panic
	observer.Printf("test %s %d", "hello", 42)
}

func TestMockObserver_Printf(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()

	observer.Printf("msg1 %s", "a")
	observer.Printf("msg2 %d", 1)

	assert.Len(t, observer.messages, 2)
}
