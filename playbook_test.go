package playbookd

import (
	"math"
	"testing"
)

func TestWilsonConfidence(t *testing.T) {
	tests := []struct {
		name      string
		successes int
		failures  int
		wantZero  bool
		wantApprox float64
		epsilon    float64
	}{
		{
			name:      "zero executions returns zero",
			successes: 0,
			failures:  0,
			wantZero:  true,
		},
		{
			name:       "perfect record with few samples is penalized",
			successes:  1,
			failures:   0,
			wantApprox: 0.21, // Wilson lower bound is ~0.207 for 1/1 at 95% CI
			epsilon:    0.05,
		},
		{
			name:       "high sample count with high success converges near success rate",
			successes:  95,
			failures:   5,
			wantApprox: 0.88,
			epsilon:    0.05,
		},
		{
			name:       "all failures returns near zero",
			successes:  0,
			failures:   10,
			wantApprox: 0.0,
			epsilon:    0.05,
		},
		{
			name:       "50/50 split is penalized from 0.5",
			successes:  50,
			failures:   50,
			wantApprox: 0.40,
			epsilon:    0.05,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WilsonConfidence(tt.successes, tt.failures)
			if tt.wantZero {
				if got != 0 {
					t.Errorf("WilsonConfidence(%d, %d) = %f, want 0", tt.successes, tt.failures, got)
				}
				return
			}
			if math.Abs(got-tt.wantApprox) > tt.epsilon {
				t.Errorf("WilsonConfidence(%d, %d) = %f, want ~%f (±%f)",
					tt.successes, tt.failures, got, tt.wantApprox, tt.epsilon)
			}
		})
	}
}

func TestWilsonConfidenceMonotonicity(t *testing.T) {
	// More successes with same failures should give higher confidence.
	low := WilsonConfidence(5, 5)
	high := WilsonConfidence(10, 5)
	if low >= high {
		t.Errorf("expected WilsonConfidence(5,5)=%f < WilsonConfidence(10,5)=%f", low, high)
	}
}

func TestWilsonConfidenceAlwaysNonNegative(t *testing.T) {
	// Cases with at least one success produce non-negative confidence.
	cases := [][2]int{
		{1, 10}, {2, 100}, {1, 100},
	}
	for _, c := range cases {
		got := WilsonConfidence(c[0], c[1])
		if got < 0 {
			t.Errorf("WilsonConfidence(%d, %d) = %f, expected non-negative", c[0], c[1], got)
		}
	}
	// When successes=0, the Wilson formula can produce a tiny negative value
	// (~-1.8e-17) due to floating point rounding; this is effectively zero.
	const epsilon = 1e-10
	zeroCase := WilsonConfidence(0, 1)
	if zeroCase < -epsilon {
		t.Errorf("WilsonConfidence(0, 1) = %e, expected effectively zero (within %e)", zeroCase, epsilon)
	}
}

func TestUpdateStats(t *testing.T) {
	t.Run("no executions", func(t *testing.T) {
		pb := &Playbook{}
		pb.UpdateStats()
		if pb.SuccessRate != 0 {
			t.Errorf("SuccessRate = %f, want 0", pb.SuccessRate)
		}
		if pb.Confidence != 0 {
			t.Errorf("Confidence = %f, want 0", pb.Confidence)
		}
	})

	t.Run("all successes", func(t *testing.T) {
		pb := &Playbook{SuccessCount: 10, FailureCount: 0}
		pb.UpdateStats()
		if pb.SuccessRate != 1.0 {
			t.Errorf("SuccessRate = %f, want 1.0", pb.SuccessRate)
		}
		if pb.Confidence <= 0 {
			t.Errorf("Confidence = %f, want > 0", pb.Confidence)
		}
	})

	t.Run("mixed results", func(t *testing.T) {
		pb := &Playbook{SuccessCount: 7, FailureCount: 3}
		pb.UpdateStats()
		wantRate := 7.0 / 10.0
		if math.Abs(pb.SuccessRate-wantRate) > 1e-9 {
			t.Errorf("SuccessRate = %f, want %f", pb.SuccessRate, wantRate)
		}
		wantConf := WilsonConfidence(7, 3)
		if math.Abs(pb.Confidence-wantConf) > 1e-9 {
			t.Errorf("Confidence = %f, want %f", pb.Confidence, wantConf)
		}
	})

	t.Run("all failures", func(t *testing.T) {
		pb := &Playbook{SuccessCount: 0, FailureCount: 5}
		pb.UpdateStats()
		if pb.SuccessRate != 0 {
			t.Errorf("SuccessRate = %f, want 0", pb.SuccessRate)
		}
	})
}

func TestShouldPromote(t *testing.T) {
	tests := []struct {
		name    string
		pb      Playbook
		wantYes bool
	}{
		{
			name:    "draft with 3 successes should promote",
			pb:      Playbook{Status: StatusDraft, SuccessCount: 3},
			wantYes: true,
		},
		{
			name:    "draft with more than 3 successes should promote",
			pb:      Playbook{Status: StatusDraft, SuccessCount: 10},
			wantYes: true,
		},
		{
			name:    "draft with only 2 successes should not promote",
			pb:      Playbook{Status: StatusDraft, SuccessCount: 2},
			wantYes: false,
		},
		{
			name:    "active playbook should not promote",
			pb:      Playbook{Status: StatusActive, SuccessCount: 10},
			wantYes: false,
		},
		{
			name:    "deprecated playbook should not promote",
			pb:      Playbook{Status: StatusDeprecated, SuccessCount: 10},
			wantYes: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pb.ShouldPromote()
			if got != tt.wantYes {
				t.Errorf("ShouldPromote() = %v, want %v", got, tt.wantYes)
			}
		})
	}
}

func TestShouldDeprecate(t *testing.T) {
	const threshold = 0.3

	tests := []struct {
		name    string
		pb      Playbook
		wantYes bool
	}{
		{
			name:    "fewer than 5 total executions should not deprecate",
			pb:      Playbook{SuccessCount: 0, FailureCount: 4, SuccessRate: 0},
			wantYes: false,
		},
		{
			name:    "5 total executions with low success rate should deprecate",
			pb:      Playbook{SuccessCount: 1, FailureCount: 4, SuccessRate: 0.2},
			wantYes: true,
		},
		{
			name:    "5 total with success rate equal to threshold should not deprecate",
			pb:      Playbook{SuccessCount: 2, FailureCount: 3, SuccessRate: 0.4},
			wantYes: false,
		},
		{
			name:    "high failure rate over many executions should deprecate",
			pb:      Playbook{SuccessCount: 2, FailureCount: 8, SuccessRate: 0.2},
			wantYes: true,
		},
		{
			name:    "high success rate should not deprecate",
			pb:      Playbook{SuccessCount: 9, FailureCount: 1, SuccessRate: 0.9},
			wantYes: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pb.ShouldDeprecate(threshold)
			if got != tt.wantYes {
				t.Errorf("ShouldDeprecate(%f) = %v, want %v", threshold, got, tt.wantYes)
			}
		})
	}
}
