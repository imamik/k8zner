package provisioning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockObserver is a test implementation of Observer that records events.
type MockObserver struct {
	events   []Event
	messages []string
	fields   map[string]string
}

func NewMockObserver() *MockObserver {
	return &MockObserver{
		events:   make([]Event, 0),
		messages: make([]string, 0),
		fields:   make(map[string]string),
	}
}

func (m *MockObserver) Printf(format string, v ...interface{}) {
	// Record raw log messages
	m.messages = append(m.messages, format)
}

func (m *MockObserver) Event(event Event) {
	m.events = append(m.events, event)
}

func (m *MockObserver) Progress(phase string, current, total int) {
	m.Event(Event{
		Type:    EventProgress,
		Phase:   phase,
		Message: "progress",
		Fields: map[string]string{
			"current": string(rune(current)),
			"total":   string(rune(total)),
		},
	})
}

func (m *MockObserver) WithFields(fields map[string]string) Observer {
	newObserver := NewMockObserver()
	newObserver.fields = make(map[string]string)
	for k, v := range m.fields {
		newObserver.fields[k] = v
	}
	for k, v := range fields {
		newObserver.fields[k] = v
	}
	return newObserver
}

func TestConsoleObserver_Printf(t *testing.T) {
	observer := NewConsoleObserver()

	// Should not panic
	observer.Printf("test message: %s", "value")
}

func TestConsoleObserver_Event(t *testing.T) {
	observer := NewConsoleObserver()

	event := Event{
		Type:     EventResourceCreated,
		Phase:    "test",
		Resource: "test-resource",
		Message:  "resource created successfully",
		Fields: map[string]string{
			"type": "network",
			"id":   "12345",
		},
	}

	// Should not panic
	observer.Event(event)
}

func TestConsoleObserver_Progress(t *testing.T) {
	observer := NewConsoleObserver()

	// Should not panic
	observer.Progress("test-phase", 5, 10)
}

func TestConsoleObserver_WithFields(t *testing.T) {
	observer := NewConsoleObserver()

	contextualObserver := observer.WithFields(map[string]string{
		"cluster": "test-cluster",
		"region":  "us-east",
	})

	assert.NotNil(t, contextualObserver)
}

func TestMockObserver_Events(t *testing.T) {
	observer := NewMockObserver()

	// Log some events
	LogPhaseStart(observer, "test-phase")
	LogResourceCreating(observer, "infra", "network", "test-net")
	LogResourceCreated(observer, "infra", "network", "test-net", "12345")
	LogPhaseComplete(observer, "test-phase", 2*time.Second)

	// Verify events were recorded
	assert.Len(t, observer.events, 4)

	assert.Equal(t, EventPhaseStarted, observer.events[0].Type)
	assert.Equal(t, "test-phase", observer.events[0].Phase)

	assert.Equal(t, EventResourceCreating, observer.events[1].Type)
	assert.Equal(t, "test-net", observer.events[1].Resource)

	assert.Equal(t, EventResourceCreated, observer.events[2].Type)
	assert.Equal(t, "12345", observer.events[2].Fields["id"])

	assert.Equal(t, EventPhaseCompleted, observer.events[3].Type)
}

func TestEventTypes(t *testing.T) {
	// Verify all event types are defined
	eventTypes := []EventType{
		EventPhaseStarted,
		EventPhaseCompleted,
		EventPhaseFailed,
		EventResourceCreating,
		EventResourceCreated,
		EventResourceExists,
		EventResourceFailed,
		EventValidationWarning,
		EventValidationError,
		EventProgress,
	}

	for _, et := range eventTypes {
		assert.NotEmpty(t, et)
	}
}

func TestObserver_ImplementsLogger(t *testing.T) {
	var logger Logger
	var observer Observer = NewConsoleObserver()

	// Observer should be assignable to Logger (implements interface)
	logger = observer
	assert.NotNil(t, logger)
}

func TestLogHelpers(t *testing.T) {
	observer := NewMockObserver()

	// Test all log helpers
	LogPhaseStart(observer, "phase1")
	LogPhaseComplete(observer, "phase1", time.Second)
	LogPhaseFailed(observer, "phase2", assert.AnError)
	LogResourceCreating(observer, "compute", "server", "srv-1")
	LogResourceCreated(observer, "compute", "server", "srv-1", "id-123")
	LogResourceExists(observer, "compute", "server", "srv-1", "id-123")

	assert.Len(t, observer.events, 6)
}
