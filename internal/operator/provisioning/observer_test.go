package provisioning

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/imamik/k8zner/internal/provisioning"
)

func TestNewOperatorObserver(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	obs := NewOperatorObserver(ctx)

	require.NotNil(t, obs)
}

func TestOperatorObserver_ImplementsObserver(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	obs := NewOperatorObserver(ctx)

	// Verify it implements the provisioning.Observer interface
	var _ provisioning.Observer = obs
}

func TestOperatorObserver_Printf_DoesNotPanic(t *testing.T) {
	t.Parallel()
	ctx := log.IntoContext(context.Background(), logr.Discard())
	obs := NewOperatorObserver(ctx)

	// Should not panic with discard logger
	obs.Printf("test %s %d", "hello", 42)
}

func TestOperatorObserver_Printf_Messages(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{}
	logger := logr.New(sink)
	ctx := log.IntoContext(context.Background(), logger)
	obs := NewOperatorObserver(ctx)

	obs.Printf("hello %s", "world")

	assert.Len(t, sink.messages, 1)
	assert.Equal(t, "hello world", sink.messages[0])
}

// fakeSink is a minimal logr.LogSink for testing.
type fakeSink struct {
	messages []string
}

func (f *fakeSink) Init(logr.RuntimeInfo)                    {}
func (f *fakeSink) Enabled(int) bool                         { return true }
func (f *fakeSink) WithValues(...interface{}) logr.LogSink   { return f }
func (f *fakeSink) WithName(string) logr.LogSink             { return f }
func (f *fakeSink) Error(error, string, ...interface{})      {}
func (f *fakeSink) Info(_ int, msg string, _ ...interface{}) { f.messages = append(f.messages, msg) }
