package assessment

import (
	"testing"
	"time"

	"specimen-transit-guard/internal/domain"
)

func TestEvaluateSortsAndDetectsExcursion(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	tc := domain.TransitCase{ID: "c1", TemperatureMinC: 2, TemperatureMaxC: 8}
	ev := domain.HandoffEvidence{TransportStartedAt: start, TransportEndedAt: start.Add(20 * time.Minute)}
	rs := []domain.TemperatureReading{
		{RecordedAt: start.Add(20 * time.Minute), TemperatureC: 5},
		{RecordedAt: start, TemperatureC: 5},
		{RecordedAt: start.Add(10 * time.Minute), TemperatureC: 10},
	}
	got, err := New(DefaultRules()).Evaluate("a1", tc, ev, rs, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != domain.AssessmentInvestigate || got.ExcursionCount != 1 || got.ExposureMinutes != 10 {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}

func TestEvaluateSeparatesDirectionsAndListsGapWindows(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	tc := domain.TransitCase{ID: "c2", TemperatureMinC: 2, TemperatureMaxC: 8}
	ev := domain.HandoffEvidence{TransportStartedAt: start, TransportEndedAt: start.Add(90 * time.Minute)}
	rs := []domain.TemperatureReading{
		{RecordedAt: start, TemperatureC: 0},
		{RecordedAt: start.Add(10 * time.Minute), TemperatureC: 5},
		{RecordedAt: start.Add(80 * time.Minute), TemperatureC: 14},
		{RecordedAt: start.Add(90 * time.Minute), TemperatureC: 5},
	}
	got, err := New(DefaultRules()).Evaluate("a2", tc, ev, rs, start.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got.LowTemperature.IntervalCount != 1 || got.HighTemperature.IntervalCount != 1 || len(got.MissingWindows) != 1 || got.Severity != domain.SeverityMajor {
		t.Fatalf("unexpected directional assessment: %+v", got)
	}
	if got.LowTemperature.Intervals[0].StartReading.TemperatureC != 0 || got.HighTemperature.Intervals[0].StartReading.TemperatureC != 14 {
		t.Fatalf("boundary readings missing: %+v %+v", got.LowTemperature, got.HighTemperature)
	}
}
