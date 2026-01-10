package provisioning

import (
	"fmt"
	"time"
)

// Pipeline manages the execution of provisioning phases.
type Pipeline struct {
	Phases []Phase
}

// NewPipeline creates a new provisioning pipeline.
func NewPipeline(phases ...Phase) *Pipeline {
	return &Pipeline{
		Phases: phases,
	}
}

// Run executes all phases in the pipeline sequentially.
func (p *Pipeline) Run(ctx *Context) error {
	startTotal := time.Now()
	ctx.Observer.Printf("Starting provisioning pipeline with %d phases...", len(p.Phases))

	for i, phase := range p.Phases {
		startPhase := time.Now()
		phaseName := fmt.Sprintf("Phase %d/%d", i+1, len(p.Phases))

		LogPhaseStart(ctx.Observer, phaseName)

		if err := phase.Provision(ctx); err != nil {
			LogPhaseFailed(ctx.Observer, phaseName, err)
			return fmt.Errorf("phase %d failed: %w", i+1, err)
		}

		LogPhaseComplete(ctx.Observer, phaseName, time.Since(startPhase))
	}

	ctx.Observer.Printf("Provisioning pipeline completed successfully in %v", time.Since(startTotal).Round(time.Millisecond))
	return nil
}
