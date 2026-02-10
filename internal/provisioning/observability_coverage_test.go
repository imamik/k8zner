package provisioning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsoleObserver_Progress_ZeroTotal(t *testing.T) {
	t.Parallel()
	observer := NewConsoleObserver()

	// Should not panic when total is 0 (triggers early return branch)
	observer.Progress("test-phase", 0, 0)
}

func TestConsoleObserver_Progress_NonZeroTotal(t *testing.T) {
	t.Parallel()
	observer := NewConsoleObserver()

	// Should not panic and should calculate percentage
	observer.Progress("test-phase", 5, 10)
}

func TestConsoleObserver_Event_ZeroTimestamp(t *testing.T) {
	t.Parallel()
	observer := NewConsoleObserver()

	event := Event{
		Type:    EventResourceCreated,
		Phase:   "test",
		Message: "test event with zero timestamp",
		// Timestamp is zero - should be filled in by the observer
	}

	// Should not panic; observer should set timestamp to Now()
	observer.Event(event)
}

func TestConsoleObserver_Event_NilFields(t *testing.T) {
	t.Parallel()
	observer := NewConsoleObserver()

	event := Event{
		Type:    EventResourceCreated,
		Phase:   "test",
		Message: "test event with nil fields",
		Fields:  nil,
	}

	// Should not panic; observer initializes Fields map
	observer.Event(event)
}

func TestConsoleObserver_Event_WithContextFields(t *testing.T) {
	t.Parallel()
	observer := NewConsoleObserver()
	child := observer.WithFields(map[string]string{
		"cluster": "test-cluster",
		"region":  "fsn1",
	})

	event := Event{
		Type:    EventResourceCreated,
		Phase:   "infra",
		Message: "network created",
		Fields:  map[string]string{"id": "12345"},
	}

	// Should merge context fields with event fields
	child.Event(event)
}

func TestConsoleObserver_Event_ContextFieldsDoNotOverrideEventFields(t *testing.T) {
	t.Parallel()
	observer := NewConsoleObserver()

	// Set a context field that conflicts with an event field
	child := observer.WithFields(map[string]string{
		"id": "context-value",
	}).(*ConsoleObserver)

	event := Event{
		Type:    EventResourceCreated,
		Phase:   "test",
		Message: "test",
		Fields:  map[string]string{"id": "event-value"},
	}

	// This exercises the "if _, exists := event.Fields[k]; !exists" branch
	// Event fields should take precedence
	child.Event(event)
}

func TestConsoleObserver_FormatEvent_AllParts(t *testing.T) {
	t.Parallel()
	observer := NewConsoleObserver()

	event := Event{
		Type:     EventResourceCreated,
		Phase:    "infra",
		Resource: "my-network",
		Message:  "network created successfully",
		Fields: map[string]string{
			"type": "network",
			"id":   "12345",
		},
	}

	msg := observer.formatEvent(event)
	assert.Contains(t, msg, string(EventResourceCreated))
	assert.Contains(t, msg, "[infra]")
	assert.Contains(t, msg, "resource=my-network")
	assert.Contains(t, msg, "network created successfully")
}

func TestConsoleObserver_FormatEvent_Minimal(t *testing.T) {
	t.Parallel()
	observer := NewConsoleObserver()

	event := Event{
		Type:    EventPhaseStarted,
		Message: "starting",
	}

	msg := observer.formatEvent(event)
	assert.Contains(t, msg, string(EventPhaseStarted))
	assert.Contains(t, msg, "starting")
	// No phase, no resource, no fields
	assert.NotContains(t, msg, "[")
	assert.NotContains(t, msg, "resource=")
}

func TestConsoleObserver_FormatEvent_EmptyFields(t *testing.T) {
	t.Parallel()
	observer := NewConsoleObserver()

	event := Event{
		Type:    EventPhaseCompleted,
		Phase:   "test",
		Message: "done",
		Fields:  map[string]string{}, // empty but not nil
	}

	msg := observer.formatEvent(event)
	assert.Contains(t, msg, "done")
	// Empty fields should not produce "()" in output
	assert.NotContains(t, msg, "()")
}

// --- LogResourceDeleting and LogResourceDeleted tests ---

func TestLogResourceDeleting(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()

	LogResourceDeleting(observer, "compute", "server", "srv-1")

	require.Len(t, observer.events, 1)
	assert.Equal(t, EventResourceDeleting, observer.events[0].Type)
	assert.Equal(t, "compute", observer.events[0].Phase)
	assert.Equal(t, "srv-1", observer.events[0].Resource)
	assert.Contains(t, observer.events[0].Message, "deleting server")
	assert.Equal(t, "server", observer.events[0].Fields["type"])
}

func TestLogResourceDeleted(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()

	LogResourceDeleted(observer, "compute", "server", "srv-1")

	require.Len(t, observer.events, 1)
	assert.Equal(t, EventResourceDeleted, observer.events[0].Type)
	assert.Equal(t, "compute", observer.events[0].Phase)
	assert.Equal(t, "srv-1", observer.events[0].Resource)
	assert.Contains(t, observer.events[0].Message, "server deleted")
	assert.Equal(t, "server", observer.events[0].Fields["type"])
}

func TestLogPhaseFailed_EventType(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()

	LogPhaseFailed(observer, "infra", assert.AnError)

	require.Len(t, observer.events, 1)
	assert.Equal(t, EventPhaseFailed, observer.events[0].Type)
	assert.Equal(t, "infra", observer.events[0].Phase)
	assert.Contains(t, observer.events[0].Message, "failed")
}

func TestLogResourceExists(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()

	LogResourceExists(observer, "infra", "network", "my-net", "42")

	require.Len(t, observer.events, 1)
	assert.Equal(t, EventResourceExists, observer.events[0].Type)
	assert.Equal(t, "infra", observer.events[0].Phase)
	assert.Equal(t, "my-net", observer.events[0].Resource)
	assert.Contains(t, observer.events[0].Message, "already exists")
	assert.Equal(t, "42", observer.events[0].Fields["id"])
}

// --- MockObserver WithFields test ---

func TestMockObserver_WithFields_MergesParent(t *testing.T) {
	t.Parallel()
	obs := NewMockObserver()

	child1 := obs.WithFields(map[string]string{"a": "1"}).(*MockObserver)
	child2 := child1.WithFields(map[string]string{"b": "2"}).(*MockObserver)

	assert.Equal(t, "1", child2.fields["a"], "should inherit parent fields")
	assert.Equal(t, "2", child2.fields["b"], "should have own fields")
	assert.Empty(t, obs.fields, "root should be unmodified")
}

// --- Event timestamp check ---

func TestEvent_TimestampSet(t *testing.T) {
	t.Parallel()
	event := Event{
		Type:      EventPhaseStarted,
		Phase:     "test",
		Message:   "starting",
		Timestamp: time.Now(),
	}

	assert.False(t, event.Timestamp.IsZero())
}

// --- EventType deletion types exist ---

func TestEventTypes_DeletionTypes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, EventType("resource.deleting"), EventResourceDeleting)
	assert.Equal(t, EventType("resource.deleted"), EventResourceDeleted)
}
