package provisioning

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/imamik/k8zner/internal/provisioning"
)

// OperatorObserver implements provisioning.Observer for operator context.
type OperatorObserver struct {
	ctx    context.Context
	fields map[string]string
}

// NewOperatorObserver creates a new operator observer.
func NewOperatorObserver(ctx context.Context) *OperatorObserver {
	return &OperatorObserver{
		ctx:    ctx,
		fields: make(map[string]string),
	}
}

// Printf implements the Logger interface.
func (o *OperatorObserver) Printf(format string, v ...interface{}) {
	logger := log.FromContext(o.ctx)
	logger.Info(fmt.Sprintf(format, v...))
}

// Event implements provisioning.Observer.
func (o *OperatorObserver) Event(event provisioning.Event) {
	logger := log.FromContext(o.ctx)

	// Merge context fields with event fields
	fields := make(map[string]string)
	for k, v := range o.fields {
		fields[k] = v
	}
	for k, v := range event.Fields {
		fields[k] = v
	}

	// Convert to key-value pairs for structured logging
	keysAndValues := make([]interface{}, 0, len(fields)*2+4)
	keysAndValues = append(keysAndValues, "eventType", string(event.Type))
	if event.Phase != "" {
		keysAndValues = append(keysAndValues, "phase", event.Phase)
	}
	if event.Resource != "" {
		keysAndValues = append(keysAndValues, "resource", event.Resource)
	}
	for k, v := range fields {
		keysAndValues = append(keysAndValues, k, v)
	}

	switch event.Type {
	case provisioning.EventPhaseFailed, provisioning.EventResourceFailed, provisioning.EventValidationError:
		logger.Error(nil, event.Message, keysAndValues...)
	default:
		logger.Info(event.Message, keysAndValues...)
	}
}

// Progress implements provisioning.Observer.
func (o *OperatorObserver) Progress(phase string, current, total int) {
	logger := log.FromContext(o.ctx)
	logger.V(1).Info("progress", "phase", phase, "current", current, "total", total)
}

// WithFields implements provisioning.Observer.
func (o *OperatorObserver) WithFields(fields map[string]string) provisioning.Observer {
	newFields := make(map[string]string)
	for k, v := range o.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}
	return &OperatorObserver{
		ctx:    o.ctx,
		fields: newFields,
	}
}
