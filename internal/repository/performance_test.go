package repository

import (
	"testing"
	"time"
)

func TestPerformanceRecorderAccumulatesAndSortsPhases(t *testing.T) {
	recorder := NewPerformanceRecorder()
	first := recorder.Measure("zeta")
	time.Sleep(time.Millisecond)
	first()
	recorder.Measure("alpha")()
	recorder.Measure("zeta")()

	phases := recorder.Phases()
	if len(phases) != 2 {
		t.Fatalf("phase count = %d, want 2", len(phases))
	}
	if phases[0].Name != "alpha" || phases[1].Name != "zeta" {
		t.Fatalf("phases = %#v, want alpha then zeta", phases)
	}
	if phases[1].DurationNanos <= 0 || phases[1].AllocObjects == 0 {
		t.Fatalf("zeta phase = %#v, want duration and allocation metrics", phases[1])
	}
}

func TestNilPerformanceRecorderDoesNotRequireInstrumentation(t *testing.T) {
	var recorder *PerformanceRecorder
	recorder.Measure("ignored")()
	if phases := recorder.Phases(); phases != nil {
		t.Fatalf("nil recorder phases = %#v, want nil", phases)
	}
}
