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
	assert.NotNil(t, obs.fields)
	assert.Empty(t, obs.fields)
}

func TestOperatorObserver_WithFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	obs := NewOperatorObserver(ctx)

	child := obs.WithFields(map[string]string{
		"cluster": "test-cluster",
		"phase":   "infra",
	})

	require.NotNil(t, child)
	childObs, ok := child.(*OperatorObserver)
	require.True(t, ok)
	assert.Equal(t, "test-cluster", childObs.fields["cluster"])
	assert.Equal(t, "infra", childObs.fields["phase"])

	// Parent should be unmodified
	assert.Empty(t, obs.fields)
}

func TestOperatorObserver_WithFields_MergesParent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	obs := NewOperatorObserver(ctx)

	child1 := obs.WithFields(map[string]string{"a": "1"}).(*OperatorObserver)
	child2 := child1.WithFields(map[string]string{"b": "2"}).(*OperatorObserver)

	assert.Equal(t, "1", child2.fields["a"], "should inherit parent fields")
	assert.Equal(t, "2", child2.fields["b"], "should have own fields")
	assert.Empty(t, obs.fields, "root should be unmodified")
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

func TestOperatorObserver_Event_DoesNotPanic(t *testing.T) {
	t.Parallel()
	ctx := log.IntoContext(context.Background(), logr.Discard())
	obs := NewOperatorObserver(ctx)

	// Test various event types
	events := []provisioning.Event{
		{Type: provisioning.EventPhaseStarted, Phase: "infra", Message: "starting"},
		{Type: provisioning.EventPhaseCompleted, Phase: "infra", Message: "done"},
		{Type: provisioning.EventPhaseFailed, Phase: "infra", Message: "error"},
		{Type: provisioning.EventResourceCreated, Resource: "network", Message: "created", Fields: map[string]string{"id": "1"}},
		{Type: provisioning.EventResourceFailed, Resource: "network", Message: "failed"},
		{Type: provisioning.EventValidationError, Message: "invalid"},
	}

	for _, e := range events {
		obs.Event(e)
	}
}

func TestOperatorObserver_Event_MergesContextFields(t *testing.T) {
	t.Parallel()
	ctx := log.IntoContext(context.Background(), logr.Discard())
	obs := NewOperatorObserver(ctx)
	childObs := obs.WithFields(map[string]string{"cluster": "test"}).(*OperatorObserver)

	// Event with its own fields
	event := provisioning.Event{
		Type:    provisioning.EventResourceCreated,
		Message: "created",
		Fields:  map[string]string{"id": "42"},
	}

	// Should not panic; fields from both context and event are merged
	childObs.Event(event)
}

func TestOperatorObserver_Progress_DoesNotPanic(t *testing.T) {
	t.Parallel()
	ctx := log.IntoContext(context.Background(), logr.Discard())
	obs := NewOperatorObserver(ctx)

	obs.Progress("compute", 3, 5)
}
