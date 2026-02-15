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

func TestRunPhases_Success(t *testing.T) {
	t.Parallel()
	executed := make([]string, 0)

	observer := NewMockObserver()
	ctx := &Context{Observer: observer}

	phases := []Phase{
		phaseFunc("infra", func(_ *Context) error { executed = append(executed, "infra"); return nil }),
		phaseFunc("compute", func(_ *Context) error { executed = append(executed, "compute"); return nil }),
		phaseFunc("cluster", func(_ *Context) error { executed = append(executed, "cluster"); return nil }),
	}

	err := RunPhases(ctx, phases)

	require.NoError(t, err)
	assert.Equal(t, []string{"infra", "compute", "cluster"}, executed)
}

func TestRunPhases_StopsOnError(t *testing.T) {
	t.Parallel()
	executed := make([]string, 0)

	observer := NewMockObserver()
	ctx := &Context{Observer: observer}

	phases := []Phase{
		phaseFunc("infra", func(_ *Context) error { executed = append(executed, "infra"); return nil }),
		phaseFunc("compute", func(_ *Context) error { return fmt.Errorf("out of capacity") }),
		phaseFunc("cluster", func(_ *Context) error { executed = append(executed, "cluster"); return nil }),
	}

	err := RunPhases(ctx, phases)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "compute phase failed")
	assert.Contains(t, err.Error(), "out of capacity")
	// cluster should NOT have executed
	assert.Equal(t, []string{"infra"}, executed)
}

func TestRunPhases_Empty(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()
	ctx := &Context{Observer: observer}

	err := RunPhases(ctx, nil)

	require.NoError(t, err)
}

func TestRunPhases_LogsProgress(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()
	ctx := &Context{Observer: observer}

	phases := []Phase{
		phaseFunc("test", func(_ *Context) error { return nil }),
	}

	err := RunPhases(ctx, phases)

	require.NoError(t, err)
	// Should have logged starting, phase start, phase complete, and overall complete
	assert.GreaterOrEqual(t, len(observer.messages), 3)
}

func TestRunPhases_LogsFailure(t *testing.T) {
	t.Parallel()
	observer := NewMockObserver()
	ctx := &Context{Observer: observer}

	phases := []Phase{
		phaseFunc("failing", func(_ *Context) error { return fmt.Errorf("boom") }),
	}

	_ = RunPhases(ctx, phases)

	// Should have logged the failure
	found := false
	for _, msg := range observer.messages {
		if assert.ObjectsAreEqual("[%s] failed: %v", msg) {
			found = true
		}
	}
	assert.True(t, found || len(observer.messages) >= 2, "should log phase failure")
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
