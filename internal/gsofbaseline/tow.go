package gsofbaseline

import "math"

const gpsWeekSec = 604800.0

// TowAbsDiffSeconds returns |a−b| in seconds, treating GPS TOW as circular modulo one week.
func TowAbsDiffSeconds(a, b float64) float64 {
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return math.NaN()
	}
	d := math.Abs(a - b)
	if d > gpsWeekSec/2 {
		d = gpsWeekSec - d
	}
	return d
}
