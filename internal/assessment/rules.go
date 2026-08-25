package assessment

import "time"

const CurrentRuleVersion = "temperature-v2"

type Rules struct {
	MaximumGap      time.Duration
	CoverageSlop    time.Duration
	MinimumReadings int
	MajorDeviationC float64
	MajorExposure   float64
	MajorGap        time.Duration
}

func DefaultRules() Rules {
	return Rules{MaximumGap: 30 * time.Minute, CoverageSlop: 10 * time.Minute, MinimumReadings: 2,
		MajorDeviationC: 5, MajorExposure: 60, MajorGap: 2 * time.Hour}
}
