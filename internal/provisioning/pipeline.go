package provisioning

import (
	"fmt"
	"time"
)

// RunPhases executes all provisioning phases sequentially.
func RunPhases(ctx *Context, phases []Phase) error {
	start := time.Now()
	ctx.Observer.Printf("Starting provisioning with %d phases...", len(phases))

	for i, phase := range phases {
		phaseStart := time.Now()
		name := fmt.Sprintf("%s (%d/%d)", phase.Name(), i+1, len(phases))

		ctx.Observer.Printf("[%s] starting", name)

		if err := phase.Provision(ctx); err != nil {
			ctx.Observer.Printf("[%s] failed: %v", name, err)
			return fmt.Errorf("%s phase failed: %w", phase.Name(), err)
		}

		ctx.Observer.Printf("[%s] completed in %v", name, time.Since(phaseStart).Round(time.Millisecond))
	}

	ctx.Observer.Printf("Provisioning completed in %v", time.Since(start).Round(time.Millisecond))
	return nil
}
