package domain

import "time"

type AssessmentResult string

type DeviationSeverity string

const (
	AssessmentPass        AssessmentResult  = "pass"
	AssessmentInvestigate AssessmentResult  = "investigate"
	SeverityNone          DeviationSeverity = "none"
	SeverityGeneral       DeviationSeverity = "general"
	SeverityMajor         DeviationSeverity = "major"
)

type Excursion struct {
	StartedAt    time.Time          `json:"started_at"`
	EndedAt      time.Time          `json:"ended_at"`
	MinimumC     float64            `json:"minimum_c"`
	MaximumC     float64            `json:"maximum_c"`
	StartReading TemperatureReading `json:"start_reading"`
	EndReading   TemperatureReading `json:"end_reading"`
}

type DirectionalExcursionStats struct {
	Direction         string      `json:"direction"`
	IntervalCount     int         `json:"interval_count"`
	ExposureMinutes   float64     `json:"exposure_minutes"`
	MaximumDeviationC float64     `json:"maximum_deviation_c"`
	Intervals         []Excursion `json:"intervals"`
}

type MissingWindow struct {
	ID                   string    `json:"id"`
	StartedAt            time.Time `json:"started_at"`
	EndedAt              time.Time `json:"ended_at"`
	Minutes              float64   `json:"minutes"`
	TouchesStartBoundary bool      `json:"touches_start_boundary"`
	TouchesEndBoundary   bool      `json:"touches_end_boundary"`
}

type AssessmentTrigger struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Summary string `json:"summary"`
}

type DeviationAssessment struct {
	ID                string                    `json:"id"`
	TransitCaseID     string                    `json:"transit_case_id"`
	RuleVersion       string                    `json:"rule_version"`
	Result            AssessmentResult          `json:"result"`
	Severity          DeviationSeverity         `json:"severity"`
	ExcursionCount    int                       `json:"excursion_count"`
	ExposureMinutes   float64                   `json:"exposure_minutes"`
	LargestGapMinutes float64                   `json:"largest_gap_minutes"`
	Triggers          []string                  `json:"triggers"`
	TriggerDetails    []AssessmentTrigger       `json:"trigger_details"`
	Excursions        []Excursion               `json:"excursions"`
	LowTemperature    DirectionalExcursionStats `json:"low_temperature"`
	HighTemperature   DirectionalExcursionStats `json:"high_temperature"`
	MissingWindows    []MissingWindow           `json:"missing_windows"`
	EvaluatedAt       time.Time                 `json:"evaluated_at"`
}
