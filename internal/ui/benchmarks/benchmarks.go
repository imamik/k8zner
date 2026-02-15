// Package benchmarks provides timing estimates for cluster provisioning phases.
package benchmarks

import (
	"time"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

// defaultTimings are median durations from E2E test runs (seconds).
var defaultTimings = map[string]int{
	"Infrastructure": 30,
	"Image":          120,
	"Compute":        60,
	"Bootstrap":      180,
	"CNI":            120,
	"Addons":         300,
	// Individual addons
	"addon:hcloud-ccm":     15,
	"addon:hcloud-csi":     20,
	"addon:metrics-server": 15,
	"addon:cert-manager":   30,
	"addon:traefik":        30,
	"addon:external-dns":   15,
	"addon:argocd":         45,
	"addon:monitoring":     90,
	"addon:talos-backup":   15,
}

// phaseOrder defines the sequence of provisioning phases for ETA calculation.
var phaseOrder = []string{
	"Infrastructure",
	"Image",
	"Compute",
	"Bootstrap",
	"CNI",
	"Addons",
}

// estimateRemaining calculates the estimated time remaining based on
// current phase, elapsed time, and historical phase records.
func estimateRemaining(currentPhase string, phaseElapsed time.Duration, history []k8znerv1alpha1.PhaseRecord) time.Duration {
	return EstimateRemainingWithScale(currentPhase, phaseElapsed, history, PerformanceScale(currentPhase, phaseElapsed, history))
}

// EstimateRemainingWithScale calculates ETA while applying a performance scale factor.
func EstimateRemainingWithScale(
	currentPhase string,
	phaseElapsed time.Duration,
	history []k8znerv1alpha1.PhaseRecord,
	scale float64,
) time.Duration {
	var remaining time.Duration

	// Find the index of the current phase
	currentIdx := -1
	for i, p := range phaseOrder {
		if p == currentPhase {
			currentIdx = i
			break
		}
	}
	if currentIdx < 0 {
		return 0
	}

	// For the current phase: max(0, expected - elapsed)
	if expected, ok := defaultTimings[currentPhase]; ok {
		expectedDur := time.Duration(expected) * time.Second
		expectedDur = time.Duration(float64(expectedDur) * scale)
		if expectedDur > phaseElapsed {
			remaining += expectedDur - phaseElapsed
		}
	}

	// For future phases: use defaultTimings (or actual durations from history if available)
	completedPhases := make(map[string]bool)
	for _, rec := range history {
		if rec.EndedAt != nil {
			completedPhases[string(rec.Phase)] = true
		}
	}

	for i := currentIdx + 1; i < len(phaseOrder); i++ {
		phase := phaseOrder[i]
		if completedPhases[phase] {
			continue
		}
		if expected, ok := defaultTimings[phase]; ok {
			expectedDur := time.Duration(expected) * time.Second
			remaining += time.Duration(float64(expectedDur) * scale)
		}
	}

	return remaining
}

// PerformanceScale derives a speed multiplier from observed-vs-expected durations.
// Example: expected 3m, observed 4m30s => scale=1.5 (future ETAs are stretched by 50%).
func PerformanceScale(currentPhase string, phaseElapsed time.Duration, history []k8znerv1alpha1.PhaseRecord) float64 {
	var expectedTotal time.Duration
	var actualTotal time.Duration

	for _, rec := range history {
		expectedSecs, ok := defaultTimings[string(rec.Phase)]
		if !ok || rec.EndedAt == nil {
			continue
		}
		expectedTotal += time.Duration(expectedSecs) * time.Second
		actualTotal += rec.EndedAt.Sub(rec.StartedAt.Time)
	}

	// If current phase is overrunning, fold it in immediately so ETA adapts quickly.
	if expectedSecs, ok := defaultTimings[currentPhase]; ok && phaseElapsed > 0 {
		expectedCurrent := time.Duration(expectedSecs) * time.Second
		if phaseElapsed > expectedCurrent {
			expectedTotal += expectedCurrent
			actualTotal += phaseElapsed
		}
	}

	if expectedTotal == 0 || actualTotal == 0 {
		return 1.0
	}

	scale := float64(actualTotal) / float64(expectedTotal)
	if scale < 0.6 {
		return 0.6
	}
	if scale > 3.0 {
		return 3.0
	}
	return scale
}

// AddonExpectedDuration returns the benchmark duration for an addon.
func AddonExpectedDuration(addon string) (time.Duration, bool) {
	secs, ok := defaultTimings["addon:"+addon]
	if !ok {
		return 0, false
	}
	return time.Duration(secs) * time.Second, true
}

// PhaseDuration returns the benchmark duration for a provisioning phase.
func PhaseDuration(phase string) (time.Duration, bool) {
	secs, ok := defaultTimings[phase]
	if !ok {
		return 0, false
	}
	return time.Duration(secs) * time.Second, true
}

// totalEstimate returns the total estimated provisioning time.
func totalEstimate() time.Duration {
	var total time.Duration
	for _, phase := range phaseOrder {
		if secs, ok := defaultTimings[phase]; ok {
			total += time.Duration(secs) * time.Second
		}
	}
	return total
}
