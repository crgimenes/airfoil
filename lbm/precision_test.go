package lbm

import (
	"math"
	"testing"
)

// Reference forces after 500 steps, captured from the float64-population
// solver before the populations were narrowed to float32. Storing populations
// at single precision moves these values in the sixth significant digit, which
// is the float32 noise floor and far below anything the readouts or the color
// maps can show. The guard exists so a future change to the kernel or to the
// storage precision has to face the numbers rather than pass unnoticed.
var precisionRef = []struct {
	name        string
	broadside   bool
	fx, mz, sep float64
}{
	{"thin", false, 0.0770296427, -7.6644494466, 0.025806451612903226},
	{"broadside", true, 4.4725827712, -445.0219857300, 0.9419354838709677},
}

// relTol is generous next to the measured drift (about 2e-6) and still tight
// enough to catch a real regression.
const relTol = 1e-4

func TestPrecisionStaysWithinTolerance(t *testing.T) {
	for _, tc := range precisionRef {
		t.Run(tc.name, func(t *testing.T) {
			s := benchSolver(tc.broadside)
			for range 500 {
				s.Step()
			}
			if !s.Finite() {
				t.Fatal("solver went non-finite over 500 steps")
			}
			checkRel(t, "Fx", s.Fx, tc.fx)
			checkRel(t, "Mz", s.Mz, tc.mz)
			// Sep is a ratio of face counts, so it is exact regardless of the
			// population precision.
			if s.Sep != tc.sep {
				t.Errorf("Sep = %v, want exactly %v", s.Sep, tc.sep)
			}
		})
	}
}

func checkRel(t *testing.T, name string, got, want float64) {
	t.Helper()
	rel := math.Abs(got-want) / math.Abs(want)
	if rel > relTol {
		t.Errorf("%s = %.10f, want %.10f (relative error %.2e exceeds %.0e)", name, got, want, rel, relTol)
	}
}
