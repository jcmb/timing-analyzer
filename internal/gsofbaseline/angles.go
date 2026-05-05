package gsofbaseline

import "math"

// AngleDiffDegSigned returns (a − b) folded to (−180, 180] for angles in degrees (e.g. 0–360 headings).
func AngleDiffDegSigned(a, b float64) float64 {
	return math.Mod(a-b+540, 360) - 180
}
