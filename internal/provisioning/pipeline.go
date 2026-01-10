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
	ctx.Logger.Printf("Starting provisioning pipeline with %d phases...", len(p.Phases))

	for i, phase := range p.Phases {
		startPhase := time.Now()
		// We could use reflection to get the phase name, but for now just use index/type
		ctx.Logger.Printf("--- Phase %d/%d: Starting ---", i+1, len(p.Phases))

		if err := phase.Provision(ctx); err != nil {
			return fmt.Errorf("phase %d failed: %w", i+1, err)
		}

		ctx.Logger.Printf("--- Phase %d/%d: Completed in %v ---", i+1, len(p.Phases), time.Since(startPhase).Round(time.Millisecond))
	}

	ctx.Logger.Printf("Provisioning pipeline completed successfully in %v", time.Since(startTotal).Round(time.Millisecond))
	return nil
}
