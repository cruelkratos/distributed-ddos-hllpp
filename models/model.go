package models

type BiasDataPoint struct {
	RawEstimate float64
	Bias        float64
}

var HLLPlusPlusThresholds = map[int]float64{
	4:  10,
	5:  20,
	6:  40,
	7:  80,
	8:  220,
	9:  400,
	10: 900,
	11: 1800,
	12: 3100,
	13: 6500,
	14: 11500,
	15: 20000,
	16: 50000,
	17: 120000,
	18: 350000,
}
