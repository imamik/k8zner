package provisioning

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// Observer defines the interface for structured observability during provisioning.
type Observer interface {
	Logger // Embeds Logger for backward compatibility

	// Event emits a structured event
	Event(event Event)

	// Progress reports progress for a phase
	Progress(phase string, current, total int)

	// WithFields returns a new Observer with additional context fields
	WithFields(fields map[string]string) Observer
}

// Event represents a structured provisioning event.
type Event struct {
	Type      EventType         // Type of event
	Phase     string            // Phase name (e.g., "infrastructure", "compute")
	Message   string            // Human-readable message
	Resource  string            // Resource name/ID if applicable
	Timestamp time.Time         // When the event occurred
	Fields    map[string]string // Additional contextual fields
}

// EventType represents the type of provisioning event.
type EventType string

const (
	// Phase events
	EventPhaseStarted   EventType = "phase.started"
	EventPhaseCompleted EventType = "phase.completed"
	EventPhaseFailed    EventType = "phase.failed"

	// Resource events
	EventResourceCreating EventType = "resource.creating"
	EventResourceCreated  EventType = "resource.created"
	EventResourceExists   EventType = "resource.exists"
	EventResourceFailed   EventType = "resource.failed"

	// Validation events
	EventValidationWarning EventType = "validation.warning"
	EventValidationError   EventType = "validation.error"

	// Progress events
	EventProgress EventType = "progress"
)

// ConsoleObserver implements Observer using standard log package.
type ConsoleObserver struct {
	contextFields map[string]string
}

// NewConsoleObserver creates a new console-based observer.
func NewConsoleObserver() *ConsoleObserver {
	return &ConsoleObserver{
		contextFields: make(map[string]string),
	}
}

// Printf implements Logger interface for backward compatibility.
func (o *ConsoleObserver) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}

// Event implements Observer interface.
func (o *ConsoleObserver) Event(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Merge context fields
	if event.Fields == nil {
		event.Fields = make(map[string]string)
	}
	for k, v := range o.contextFields {
		if _, exists := event.Fields[k]; !exists {
			event.Fields[k] = v
		}
	}

	// Format message based on event type
	msg := o.formatEvent(event)
	log.Print(msg)
}

// Progress implements Observer interface.
func (o *ConsoleObserver) Progress(phase string, current, total int) {
	percentage := (current * 100) / total
	log.Printf("[%s] Progress: %d/%d (%d%%)", phase, current, total, percentage)
}

// WithFields implements Observer interface.
func (o *ConsoleObserver) WithFields(fields map[string]string) Observer {
	newFields := make(map[string]string)
	// Copy existing fields
	for k, v := range o.contextFields {
		newFields[k] = v
	}
	// Add new fields
	for k, v := range fields {
		newFields[k] = v
	}

	return &ConsoleObserver{
		contextFields: newFields,
	}
}

// formatEvent formats an event for console output.
func (o *ConsoleObserver) formatEvent(event Event) string {
	var parts []string

	// Event type indicator
	parts = append(parts, string(event.Type))

	// Phase if present
	if event.Phase != "" {
		parts = append(parts, fmt.Sprintf("[%s]", event.Phase))
	}

	// Resource if present
	if event.Resource != "" {
		parts = append(parts, fmt.Sprintf("resource=%s", event.Resource))
	}

	// Message
	parts = append(parts, event.Message)

	// Context fields if any
	if len(event.Fields) > 0 {
		var fieldParts []string
		for k, v := range event.Fields {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%s", k, v))
		}
		parts = append(parts, fmt.Sprintf("(%s)", strings.Join(fieldParts, ", ")))
	}

	return strings.Join(parts, " ")
}

// Helper functions for common events

// LogPhaseStart logs a phase start event.
func LogPhaseStart(observer Observer, phase string) {
	observer.Event(Event{
		Type:    EventPhaseStarted,
		Phase:   phase,
		Message: "starting",
	})
}

// LogPhaseComplete logs a phase completion event.
func LogPhaseComplete(observer Observer, phase string, duration time.Duration) {
	observer.Event(Event{
		Type:    EventPhaseCompleted,
		Phase:   phase,
		Message: fmt.Sprintf("completed in %v", duration.Round(time.Millisecond)),
	})
}

// LogPhaseFailed logs a phase failure event.
func LogPhaseFailed(observer Observer, phase string, err error) {
	observer.Event(Event{
		Type:    EventPhaseFailed,
		Phase:   phase,
		Message: fmt.Sprintf("failed: %v", err),
	})
}

// LogResourceCreating logs a resource creation start event.
func LogResourceCreating(observer Observer, phase, resourceType, resourceName string) {
	observer.Event(Event{
		Type:     EventResourceCreating,
		Phase:    phase,
		Resource: resourceName,
		Message:  fmt.Sprintf("creating %s", resourceType),
		Fields: map[string]string{
			"type": resourceType,
		},
	})
}

// LogResourceCreated logs a successful resource creation event.
func LogResourceCreated(observer Observer, phase, resourceType, resourceName, resourceID string) {
	observer.Event(Event{
		Type:     EventResourceCreated,
		Phase:    phase,
		Resource: resourceName,
		Message:  fmt.Sprintf("%s created", resourceType),
		Fields: map[string]string{
			"type": resourceType,
			"id":   resourceID,
		},
	})
}

// LogResourceExists logs when a resource already exists.
func LogResourceExists(observer Observer, phase, resourceType, resourceName, resourceID string) {
	observer.Event(Event{
		Type:     EventResourceExists,
		Phase:    phase,
		Resource: resourceName,
		Message:  fmt.Sprintf("%s already exists", resourceType),
		Fields: map[string]string{
			"type": resourceType,
			"id":   resourceID,
		},
	})
}
