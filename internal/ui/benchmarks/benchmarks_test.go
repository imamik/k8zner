package benchmarks

import (
	"testing"
	"time"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEstimateRemaining_NoHistory(t *testing.T) {
	// At Infrastructure phase, 10s elapsed, no history
	remaining := EstimateRemaining("Infrastructure", 10*time.Second, nil)

	// Should be: (30-10) + 120 + 60 + 180 + 120 + 300 = 800s
	expected := 800 * time.Second
	if remaining != expected {
		t.Errorf("expected %v, got %v", expected, remaining)
	}
}

func TestEstimateRemaining_MidwayPhase(t *testing.T) {
	// At CNI phase, 60s elapsed, with completed history for earlier phases
	now := metav1.Now()
	past := metav1.NewTime(now.Add(-5 * time.Minute))
	history := []k8znerv1alpha1.PhaseRecord{
		{Phase: "Infrastructure", StartedAt: past, EndedAt: &now},
		{Phase: "Image", StartedAt: past, EndedAt: &now},
		{Phase: "Compute", StartedAt: past, EndedAt: &now},
		{Phase: "Bootstrap", StartedAt: past, EndedAt: &now},
		{Phase: "CNI", StartedAt: now},
	}

	remaining := EstimateRemaining("CNI", 60*time.Second, history)

	// Historical phases took much longer than defaults, so ETA scales up (capped at 3x):
	// (120*3 - 60) + (300*3) = 1200s
	expected := 1200 * time.Second
	if remaining != expected {
		t.Errorf("expected %v, got %v", expected, remaining)
	}
}

func TestEstimateRemaining_ElapsedExceedsExpected(t *testing.T) {
	// At Infrastructure phase, but already spent 60s (over the 30s estimate)
	remaining := EstimateRemaining("Infrastructure", 60*time.Second, nil)

	// Overrun scales future predictions: 60s/30s = 2x
	// Should be: max(0, 60-60)=0 + (120 + 60 + 180 + 120 + 300) * 2 = 1560s
	expected := 1560 * time.Second
	if remaining != expected {
		t.Errorf("expected %v, got %v", expected, remaining)
	}
}

func TestPerformanceScale(t *testing.T) {
	now := metav1.Now()
	past := metav1.NewTime(now.Add(-45 * time.Second))
	history := []k8znerv1alpha1.PhaseRecord{
		{Phase: "Infrastructure", StartedAt: past, EndedAt: &now},
	}

	scale := PerformanceScale("Image", 0, history)
	if scale < 1.49 || scale > 1.51 {
		t.Fatalf("expected ~1.5 scale, got %f", scale)
	}
}

func TestAddonExpectedDuration(t *testing.T) {
	d, ok := AddonExpectedDuration("traefik")
	if !ok || d != 30*time.Second {
		t.Fatalf("expected traefik default 30s, got %v (ok=%v)", d, ok)
	}
	_, ok = AddonExpectedDuration("unknown")
	if ok {
		t.Fatal("expected unknown addon duration to be absent")
	}
}

func TestEstimateRemaining_CompletePhase(t *testing.T) {
	remaining := EstimateRemaining("Complete", 0, nil)
	if remaining != 0 {
		t.Errorf("expected 0, got %v", remaining)
	}
}

func TestEstimateRemaining_UnknownPhase(t *testing.T) {
	remaining := EstimateRemaining("Unknown", 0, nil)
	if remaining != 0 {
		t.Errorf("expected 0 for unknown phase, got %v", remaining)
	}
}

func TestEstimateRemaining_LastPhase(t *testing.T) {
	// At Addons phase, 100s elapsed
	remaining := EstimateRemaining("Addons", 100*time.Second, nil)

	// Should be: max(0, 300-100) = 200s (no future phases)
	expected := 200 * time.Second
	if remaining != expected {
		t.Errorf("expected %v, got %v", expected, remaining)
	}
}

func TestTotalEstimate(t *testing.T) {
	total := TotalEstimate()

	// Sum of all phase timings: 30 + 120 + 60 + 180 + 120 + 300 = 810s
	expected := 810 * time.Second
	if total != expected {
		t.Errorf("expected %v, got %v", expected, total)
	}
}
