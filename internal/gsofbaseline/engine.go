package gsofbaseline

import (
	"math"
	"sync"

	"timing-analyzer/internal/gsof"
)

// MatchedPoint is one matched epoch (stream A TOW as primary key).
type MatchedPoint struct {
	GPSTOWSec float64 `json:"gps_tow_s"`
	// DeltaTowSec is |TOW_A − TOW_B| after GPS week wrap handling.
	DeltaTowSec float64 `json:"delta_tow_s"`
	HorizM      float64 `json:"horiz_m"`
	SlantM      float64 `json:"slant_m"`
	BearingDeg  float64 `json:"bearing_deg"`
	SVsStream1  int     `json:"svs_stream1"`
	SVsStream2  int     `json:"svs_stream2"`
	RangeRefM   float64 `json:"range_ref_m"`
	RangeDeltaM float64 `json:"range_delta_m"`
	RangeOK     bool    `json:"range_ok"`
	RangeNote   string  `json:"range_note,omitempty"`
}

// EngineConfig controls matching and optional range check.
type EngineConfig struct {
	MatchMaxTowDeltaSec float64 // max |TOW_A − TOW_B| to accept a pair
	RangeCheckTolM      float64 // if > 0, compare |slant − ref| to this
	ExpectedRangeM      float64 // if > 0, fixed reference (metres)
	RangeRefFromAttitude bool   // if ExpectedRangeM <= 0 and tol > 0, use type 27 from stream B
}

// Engine merges two GSOF streams by GPS TOW from type 1 (paired type 2 LLH).
type Engine struct {
	cfg EngineConfig
	mu  sync.Mutex

	bEpochs []EpochSample
	bAtt    []AttitudeRangeSample

	points []MatchedPoint

	lastBase35 *gsof.ReceivedBaseInfo
	lastBase41 *gsof.BasePositionQualityInfo
}

func NewEngine(cfg EngineConfig) *Engine {
	if cfg.MatchMaxTowDeltaSec <= 0 {
		cfg.MatchMaxTowDeltaSec = 0.25
	}
	return &Engine{cfg: cfg}
}

func trimFront[T any](s []T, max int) []T {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

// IngestA processes GSOF payload from stream 1 (reference TOW for matching).
func (e *Engine) IngestA(gsofBuffer []byte) {
	w := WalkGSOFPacket(gsofBuffer)
	e.mu.Lock()
	defer e.mu.Unlock()
	if w.Base35 != nil {
		cp := *w.Base35
		e.lastBase35 = &cp
	}
	for _, ep := range w.Epochs {
		b, ok := e.nearestBLocked(ep.GPSTOWSec)
		if !ok {
			continue
		}
		h := HaversineM(ep.LatDeg, ep.LonDeg, b.LatDeg, b.LonDeg)
		s := SlantM(h, ep.HeightM, b.HeightM)
		br := InitialBearingDeg(ep.LatDeg, ep.LonDeg, b.LatDeg, b.LonDeg)
		dtow := TowAbsDiffSeconds(ep.GPSTOWSec, b.GPSTOWSec)
		pt := MatchedPoint{
			GPSTOWSec:   ep.GPSTOWSec,
			DeltaTowSec: dtow,
			HorizM:      h,
			SlantM:      s,
			BearingDeg:  br,
			SVsStream1:  ep.SVsUsed,
			SVsStream2:  b.SVsUsed,
			RangeOK:     true,
		}
		ref, refSrc, okRef := e.referenceRangeLocked(b.GPSTOWSec)
		if e.cfg.RangeCheckTolM > 0 && okRef {
			pt.RangeRefM = ref
			pt.RangeDeltaM = math.Abs(s - ref)
			pt.RangeOK = pt.RangeDeltaM <= e.cfg.RangeCheckTolM
			pt.RangeNote = refSrc
		} else if e.cfg.RangeCheckTolM > 0 && !okRef {
			pt.RangeOK = true
			pt.RangeNote = "no_reference"
		}
		e.points = append(e.points, pt)
		e.points = trimFront(e.points, maxRing)
	}
}

// IngestB processes GSOF payload from stream 2.
func (e *Engine) IngestB(gsofBuffer []byte) {
	w := WalkGSOFPacket(gsofBuffer)
	e.mu.Lock()
	defer e.mu.Unlock()
	if w.Base41 != nil {
		cp := *w.Base41
		e.lastBase41 = &cp
	}
	e.bEpochs = append(e.bEpochs, w.Epochs...)
	e.bEpochs = trimFront(e.bEpochs, maxRing)
	e.bAtt = append(e.bAtt, w.AttitudeRanges...)
	e.bAtt = trimFront(e.bAtt, maxRing)
}

func (e *Engine) nearestBLocked(towA float64) (EpochSample, bool) {
	var best EpochSample
	bestD := math.MaxFloat64
	found := false
	for _, b := range e.bEpochs {
		d := TowAbsDiffSeconds(b.GPSTOWSec, towA)
		if d < bestD && d <= e.cfg.MatchMaxTowDeltaSec {
			bestD = d
			best = b
			found = true
		}
	}
	return best, found
}

func (e *Engine) referenceRangeLocked(towB float64) (ref float64, source string, ok bool) {
	if e.cfg.ExpectedRangeM > 0 {
		return e.cfg.ExpectedRangeM, "expected_range_param", true
	}
	if !e.cfg.RangeRefFromAttitude || len(e.bAtt) == 0 {
		return 0, "", false
	}
	var best AttitudeRangeSample
	bestD := math.MaxFloat64
	for _, a := range e.bAtt {
		d := TowAbsDiffSeconds(a.GPSTOWSec, towB)
		if d < bestD {
			bestD = d
			best = a
		}
	}
	if bestD > e.cfg.MatchMaxTowDeltaSec*2 {
		return 0, "", false
	}
	if best.RangeM <= 0 || math.IsNaN(best.RangeM) {
		return 0, "", false
	}
	return best.RangeM, "type27_range_stream2", true
}

// Snapshot returns a copy for JSON / SSE.
func (e *Engine) Snapshot(version string) EngineSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := EngineSnapshot{
		Version: version,
		Points:  append([]MatchedPoint(nil), e.points...),
		Base35A: e.lastBase35,
		Base41B: e.lastBase41,
	}
	return out
}

// EngineSnapshot is JSON-serializable UI state.
type EngineSnapshot struct {
	Version string                     `json:"version"`
	Points  []MatchedPoint             `json:"points"`
	Base35A *gsof.ReceivedBaseInfo     `json:"base_35_stream1,omitempty"`
	Base41B *gsof.BasePositionQualityInfo `json:"base_41_stream2,omitempty"`
}
