package gsof

import "math"

// JSONFloat returns v if it is finite; otherwise 0.
// encoding/json rejects NaN and ±Inf in float64 fields; use this for wire-derived
// values that are exposed on the dashboard / SSE payload.
func JSONFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}
