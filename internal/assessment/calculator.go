package assessment

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"specimen-transit-guard/internal/domain"
)

type Calculator struct {
	rules          Rules
	mu             sync.RWMutex
	readingsByHash map[[sha256.Size]byte][]domain.TemperatureReading
}

func New(rules Rules) *Calculator {
	return &Calculator{rules: rules, readingsByHash: make(map[[sha256.Size]byte][]domain.TemperatureReading)}
}

func (c *Calculator) CoverageSlop() time.Duration { return c.rules.CoverageSlop }

func (c *Calculator) Evaluate(id string, tc domain.TransitCase, evidence domain.HandoffEvidence, input []domain.TemperatureReading, now time.Time) (domain.DeviationAssessment, error) {
	if len(input) < c.rules.MinimumReadings {
		return domain.DeviationAssessment{}, fmt.Errorf("%w: 至少需要 %d 条温度读数", domain.ErrEvidenceIncomplete, c.rules.MinimumReadings)
	}
	readings := c.sortedReadings(input)
	missingCoverage := make([]string, 0, 2)
	if readings[0].RecordedAt.Sub(evidence.TransportStartedAt) > c.rules.CoverageSlop {
		missingCoverage = append(missingCoverage, "缺少运输起点温度覆盖")
	}
	if evidence.TransportEndedAt.Sub(readings[len(readings)-1].RecordedAt) > c.rules.CoverageSlop {
		missingCoverage = append(missingCoverage, "缺少运输终点温度覆盖")
	}
	if len(missingCoverage) > 0 {
		return domain.DeviationAssessment{}, fmt.Errorf("%w: %v", domain.ErrEvidenceIncomplete, missingCoverage)
	}
	for i := range readings {
		if readings[i].RecordedAt.Before(evidence.TransportStartedAt) || readings[i].RecordedAt.After(evidence.TransportEndedAt) {
			return domain.DeviationAssessment{}, fmt.Errorf("%w: 读数超出运输时间", domain.ErrInvalid)
		}
		if i > 0 && !readings[i].RecordedAt.After(readings[i-1].RecordedAt) {
			return domain.DeviationAssessment{}, fmt.Errorf("%w: 读数采集时间必须唯一", domain.ErrInvalid)
		}
	}
	result := domain.DeviationAssessment{
		ID: id, TransitCaseID: tc.ID, RuleVersion: CurrentRuleVersion, Result: domain.AssessmentPass,
		Severity: domain.SeverityNone, EvaluatedAt: now.UTC(), Excursions: []domain.Excursion{},
		Triggers: []string{}, TriggerDetails: []domain.AssessmentTrigger{}, MissingWindows: []domain.MissingWindow{},
		LowTemperature:  domain.DirectionalExcursionStats{Direction: "low", Intervals: []domain.Excursion{}},
		HighTemperature: domain.DirectionalExcursionStats{Direction: "high", Intervals: []domain.Excursion{}},
	}
	c.calculateGaps(&result, readings, evidence)
	c.calculateExcursions(&result, readings, tc)
	if result.LowTemperature.IntervalCount > 0 {
		addTrigger(&result, "low_temperature_excursion", "temperature_low", "温度低于允许下限")
	}
	if result.HighTemperature.IntervalCount > 0 {
		addTrigger(&result, "high_temperature_excursion", "temperature_high", "温度高于允许上限")
	}
	for _, window := range result.MissingWindows {
		addTrigger(&result, window.ID, "missing_reading_window", fmt.Sprintf("存在 %.1f 分钟缺测窗口", window.Minutes))
	}
	if len(result.Triggers) > 0 {
		result.Result = domain.AssessmentInvestigate
		result.Severity = domain.SeverityGeneral
	}
	if result.LowTemperature.MaximumDeviationC >= c.rules.MajorDeviationC || result.HighTemperature.MaximumDeviationC >= c.rules.MajorDeviationC ||
		result.LowTemperature.ExposureMinutes+result.HighTemperature.ExposureMinutes >= c.rules.MajorExposure || hasMajorGap(result.MissingWindows, c.rules.MajorGap) {
		result.Severity = domain.SeverityMajor
	}
	return result, nil
}

func (c *Calculator) sortedReadings(input []domain.TemperatureReading) []domain.TemperatureReading {
	raw, _ := json.Marshal(input)
	key := sha256.Sum256(raw)

	c.mu.RLock()
	cached, ok := c.readingsByHash[key]
	c.mu.RUnlock()
	if ok {
		return append([]domain.TemperatureReading(nil), cached...)
	}

	readings := append([]domain.TemperatureReading(nil), input...)
	sort.Slice(readings, func(i, j int) bool { return readings[i].RecordedAt.Before(readings[j].RecordedAt) })
	stored := append([]domain.TemperatureReading(nil), readings...)

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.readingsByHash[key]; ok {
		return append([]domain.TemperatureReading(nil), existing...)
	}
	c.readingsByHash[key] = stored
	return readings
}

func addTrigger(out *domain.DeviationAssessment, id, kind, summary string) {
	out.Triggers = append(out.Triggers, id)
	out.TriggerDetails = append(out.TriggerDetails, domain.AssessmentTrigger{ID: id, Type: kind, Summary: summary})
}

func hasMajorGap(windows []domain.MissingWindow, threshold time.Duration) bool {
	for _, window := range windows {
		if time.Duration(window.Minutes*float64(time.Minute)) >= threshold {
			return true
		}
	}
	return false
}

func (c *Calculator) calculateGaps(out *domain.DeviationAssessment, rs []domain.TemperatureReading, ev domain.HandoffEvidence) {
	points := []time.Time{ev.TransportStartedAt}
	for _, r := range rs {
		points = append(points, r.RecordedAt)
	}
	points = append(points, ev.TransportEndedAt)
	for i := 1; i < len(points); i++ {
		minutes := points[i].Sub(points[i-1]).Minutes()
		if minutes > out.LargestGapMinutes {
			out.LargestGapMinutes = minutes
		}
		if points[i].Sub(points[i-1]) > c.rules.MaximumGap {
			out.MissingWindows = append(out.MissingWindows, domain.MissingWindow{
				ID: fmt.Sprintf("missing_reading_window_%d", len(out.MissingWindows)+1), StartedAt: points[i-1], EndedAt: points[i], Minutes: minutes,
				TouchesStartBoundary: i == 1, TouchesEndBoundary: i == len(points)-1,
			})
		}
	}
}

func (c *Calculator) calculateExcursions(out *domain.DeviationAssessment, rs []domain.TemperatureReading, tc domain.TransitCase) {
	calculateDirection := func(direction string, outside func(float64) bool, deviation func(float64) float64) domain.DirectionalExcursionStats {
		stats := domain.DirectionalExcursionStats{Direction: direction, Intervals: []domain.Excursion{}}
		var current *domain.Excursion
		for i, reading := range rs {
			if outside(reading.TemperatureC) {
				if current == nil {
					current = &domain.Excursion{StartedAt: reading.RecordedAt, EndedAt: reading.RecordedAt, MinimumC: reading.TemperatureC, MaximumC: reading.TemperatureC, StartReading: reading, EndReading: reading}
				}
				current.EndedAt, current.EndReading = reading.RecordedAt, reading
				if reading.TemperatureC < current.MinimumC {
					current.MinimumC = reading.TemperatureC
				}
				if reading.TemperatureC > current.MaximumC {
					current.MaximumC = reading.TemperatureC
				}
				if delta := deviation(reading.TemperatureC); delta > stats.MaximumDeviationC {
					stats.MaximumDeviationC = delta
				}
				if i > 0 {
					stats.ExposureMinutes += reading.RecordedAt.Sub(rs[i-1].RecordedAt).Minutes()
				}
			} else if current != nil {
				stats.Intervals = append(stats.Intervals, *current)
				current = nil
			}
		}
		if current != nil {
			stats.Intervals = append(stats.Intervals, *current)
		}
		stats.IntervalCount = len(stats.Intervals)
		return stats
	}
	out.LowTemperature = calculateDirection("low", func(v float64) bool { return v < tc.TemperatureMinC }, func(v float64) float64 { return tc.TemperatureMinC - v })
	out.HighTemperature = calculateDirection("high", func(v float64) bool { return v > tc.TemperatureMaxC }, func(v float64) float64 { return v - tc.TemperatureMaxC })
	out.Excursions = append(out.Excursions, out.LowTemperature.Intervals...)
	out.Excursions = append(out.Excursions, out.HighTemperature.Intervals...)
	sort.Slice(out.Excursions, func(i, j int) bool { return out.Excursions[i].StartedAt.Before(out.Excursions[j].StartedAt) })
	out.ExcursionCount = len(out.Excursions)
	out.ExposureMinutes = out.LowTemperature.ExposureMinutes + out.HighTemperature.ExposureMinutes
}
