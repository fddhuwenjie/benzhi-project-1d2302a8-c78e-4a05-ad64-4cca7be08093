package assessment_cache_race_test

import (
	"sync"
	"testing"
	"time"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/domain"
)

func TestConcurrentEvaluateUsesSynchronizedCache(t *testing.T) {
	calculator := assessment.New(assessment.DefaultRules())
	startedAt := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	evidence := domain.HandoffEvidence{
		TransportStartedAt: startedAt,
		TransportEndedAt:   startedAt.Add(20 * time.Minute),
	}
	readings := []domain.TemperatureReading{
		{RecordedAt: startedAt, TemperatureC: 5},
		{RecordedAt: startedAt.Add(10 * time.Minute), TemperatureC: 10},
		{RecordedAt: startedAt.Add(20 * time.Minute), TemperatureC: 5},
	}

	start := make(chan struct{})
	results := make(chan domain.DeviationAssessment, 2)
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	for _, caseID := range []string{"case-a", "case-b"} {
		caseID := caseID
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, err := calculator.Evaluate("assessment-"+caseID, domain.TransitCase{
				ID: caseID, TemperatureMinC: 2, TemperatureMaxC: 8,
			}, evidence, readings, startedAt.Add(time.Hour))
			results <- result
			errors <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errors)

	for err := range errors {
		if err != nil {
			t.Fatalf("并发判定失败: %v", err)
		}
	}
	for result := range results {
		if result.Result != domain.AssessmentInvestigate || result.ExcursionCount != 1 {
			t.Fatalf("并发判定结果错误: %+v", result)
		}
	}
}
