package provisioning

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPhase implements the Phase interface for testing.
type mockPhase struct {
	name string
	err  error
}

func (m *mockPhase) Name() string               { return m.name }
func (m *mockPhase) Provision(_ *Context) error { return m.err }

func TestNewPipeline(t *testing.T) {
	t.Parallel()
	p1 := &mockPhase{name: "phase-1"}
	p2 := &mockPhase{name: "phase-2"}

	pipeline := NewPipeline(p1, p2)

	require.NotNil(t, pipeline)
	assert.Len(t, pipeline.Phases, 2)
	assert.Equal(t, "phase-1", pipeline.Phases[0].Name())
	assert.Equal(t, "phase-2", pipeline.Phases[1].Name())
}

func TestNewPipeline_Empty(t *testing.T) {
	t.Parallel()
	pipeline := NewPipeline()

	require.NotNil(t, pipeline)
	assert.Empty(t, pipeline.Phases)
}

func TestPipeline_Run_Success(t *testing.T) {
	t.Parallel()
	executed := make([]string, 0)

	p1 := &mockPhase{name: "infra"}
	p2 := &mockPhase{name: "compute"}
	p3 := &mockPhase{name: "cluster"}

	pipeline := NewPipeline(p1, p2, p3)

	observer := NewMockObserver()
	ctx := &Context{
		Observer: observer,
		Logger:   observer,
	}

	// Override phases to track execution order
	pipeline.Phases = []Phase{
		phaseFunc("infra", func(_ *Context) error { executed = append(executed, "infra"); return nil }),
		phaseFunc("compute", func(_ *Context) error { executed = append(executed, "compute"); return nil }),
		phaseFunc("cluster", func(_ *Context) error { executed = append(executed, "cluster"); return nil }),
	}

	err := pipeline.Run(ctx)

	require.NoError(t, err)
	assert.Equal(t, []string{"infra", "compute", "cluster"}, executed)
}

func TestPipeline_Run_StopsOnError(t *testing.T) {
	t.Parallel()
	executed := make([]string, 0)

	observer := NewMockObserver()
	ctx := &Context{
		Observer: observer,
		Logger:   observer,
	}

	pipeline := NewPipeline(
		phaseFunc("infra", func(_ *Context) error { executed = append(executed, "infra"); return nil }),
		phaseFunc("compute", func(_ *Context) error { return fmt.Errorf("out of capacity") }),
		phaseFunc("cluster", func(_ *Context) error { executed = append(executed, "cluster"); return nil }),
	)

	err := pipeline.Run(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "compute phase failed")
	assert.Contains(t, err.Error(), "out of capacity")
	// cluster should NOT have executed
	assert.Equal(t, []string{"infra"}, executed)
}

func TestPipeline_Run_EmptyPipeline(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()
	ctx := &Context{
		Observer: observer,
		Logger:   observer,
	}

	pipeline := NewPipeline()
	err := pipeline.Run(ctx)

	require.NoError(t, err)
}

func TestPipeline_Run_LogsPhaseEvents(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()
	ctx := &Context{
		Observer: observer,
		Logger:   observer,
	}

	pipeline := NewPipeline(
		phaseFunc("test", func(_ *Context) error { return nil }),
	)

	err := pipeline.Run(ctx)

	require.NoError(t, err)

	// Should have phase start and phase complete events
	var hasStart, hasComplete bool
	for _, event := range observer.events {
		if event.Type == EventPhaseStarted {
			hasStart = true
		}
		if event.Type == EventPhaseCompleted {
			hasComplete = true
		}
	}
	assert.True(t, hasStart, "should log phase start event")
	assert.True(t, hasComplete, "should log phase complete event")
}

func TestPipeline_Run_LogsFailure(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()
	ctx := &Context{
		Observer: observer,
		Logger:   observer,
	}

	pipeline := NewPipeline(
		phaseFunc("failing", func(_ *Context) error { return fmt.Errorf("boom") }),
	)

	_ = pipeline.Run(ctx)

	var hasFailed bool
	for _, event := range observer.events {
		if event.Type == EventPhaseFailed {
			hasFailed = true
		}
	}
	assert.True(t, hasFailed, "should log phase failed event")
}

// phaseFunc creates a Phase from a function for testing.
type phaseFuncImpl struct {
	name string
	fn   func(*Context) error
}

func phaseFunc(name string, fn func(*Context) error) Phase {
	return &phaseFuncImpl{name: name, fn: fn}
}

func (p *phaseFuncImpl) Name() string                 { return p.name }
func (p *phaseFuncImpl) Provision(ctx *Context) error { return p.fn(ctx) }
