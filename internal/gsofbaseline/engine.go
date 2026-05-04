package gsofbaseline

import (
	"fmt"
	"math"
	"sync"

	"timing-analyzer/internal/gsof"
)

// MatchedPoint is one matched epoch (heading receiver GPS TOW from type 1).
type MatchedPoint struct {
	GPSTOWSec float64 `json:"gps_tow_s"`
	// DeltaTowSec is |TOW_heading − TOW_reference| (type 41 TOW or moving-base type-1 TOW).
	DeltaTowSec float64 `json:"delta_tow_s"`
	HorizM      float64 `json:"horiz_m"`
	SlantM      float64 `json:"slant_m"`
	BearingDeg  float64 `json:"bearing_deg"`
	SVsHeading  int     `json:"svs_heading"`
	// SVsMovingBase is moving-base SV count when reference_source is moving_base; otherwise 0.
	SVsMovingBase int    `json:"svs_moving_base"`
	ReferenceSource string `json:"reference_source"` // "type41" | "moving_base"
	RangeRefM       float64 `json:"range_ref_m"`
	RangeDeltaM     float64 `json:"range_delta_m"`
	RangeOK         bool    `json:"range_ok"`
	RangeNote       string  `json:"range_note,omitempty"`
}

// Base41TowSample is type-41 base position keyed by the record's GPS TOW.
type Base41TowSample struct {
	GPSTOWSec float64 `json:"gps_tow_s"`
	LatDeg    float64 `json:"lat_deg"`
	LonDeg    float64 `json:"lon_deg"`
	HeightM   float64 `json:"height_m"`
}

// EngineConfig controls matching and optional range check.
type EngineConfig struct {
	MatchMaxTowDeltaSec  float64
	RangeCheckTolM       float64
	ExpectedRangeM       float64
	RangeRefFromAttitude bool
	// MovingBaseConfigured is true when a second transport is enabled (UI / status).
	MovingBaseConfigured bool
}

// Engine merges heading GSOF with either type-41 (heading stream) or moving-base LLH.
type Engine struct {
	cfg EngineConfig
	mu  sync.Mutex

	heading41Ring []Base41TowSample
	hAtt          []AttitudeRangeSample

	bEpochs []EpochSample
	bAtt    []AttitudeRangeSample

	points []MatchedPoint

	lastBase35Heading *gsof.ReceivedBaseInfo
	lastBase41Heading *gsof.BasePositionQualityInfo
	headingSerial       string

	lastBase35Moving *gsof.ReceivedBaseInfo
	lastBase41Moving *gsof.BasePositionQualityInfo
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

func base41ToSample(b gsof.BasePositionQualityInfo) Base41TowSample {
	return Base41TowSample{
		GPSTOWSec: b.GPSTOWSec,
		LatDeg:    b.LatDeg,
		LonDeg:    b.LonDeg,
		HeightM:   b.HeightM,
	}
}

// IngestHeading processes GSOF from the heading receiver (required).
func (e *Engine) IngestHeading(gsofBuffer []byte) {
	w := WalkGSOFPacket(gsofBuffer)
	e.mu.Lock()
	defer e.mu.Unlock()
	if w.Base35 != nil {
		cp := *w.Base35
		e.lastBase35Heading = &cp
	}
	for _, b := range w.Base41Records {
		e.heading41Ring = append(e.heading41Ring, base41ToSample(b))
	}
	if len(w.Base41Records) > 0 {
		last := w.Base41Records[len(w.Base41Records)-1]
		cp := last
		e.lastBase41Heading = &cp
	}
	e.heading41Ring = trimFront(e.heading41Ring, maxRing)
	if w.Serial15 != nil {
		e.headingSerial = fmt.Sprintf("%d", *w.Serial15)
	}
	e.hAtt = append(e.hAtt, w.AttitudeRanges...)
	e.hAtt = trimFront(e.hAtt, maxRing)

	for _, ep := range w.Epochs {
		tgt, ok := e.resolveTargetLocked(ep.GPSTOWSec)
		if !ok {
			continue
		}
		h := HaversineM(ep.LatDeg, ep.LonDeg, tgt.lat, tgt.lon)
		s := SlantM(h, ep.HeightM, tgt.h)
		br := InitialBearingDeg(ep.LatDeg, ep.LonDeg, tgt.lat, tgt.lon)
		pt := MatchedPoint{
			GPSTOWSec:       ep.GPSTOWSec,
			DeltaTowSec:     tgt.deltaTow,
			HorizM:          h,
			SlantM:          s,
			BearingDeg:      br,
			SVsHeading:      ep.SVsUsed,
			SVsMovingBase:   tgt.svsMoving,
			ReferenceSource: tgt.source,
			RangeOK:         true,
		}
		ref, refSrc, okRef := e.referenceRangeLocked(tgt.refTow, tgt.useHeadingAtt)
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

type resolvedTarget struct {
	lat, lon, h   float64
	svsMoving     int
	deltaTow      float64
	source        string
	refTow        float64
	useHeadingAtt bool
}

func (e *Engine) resolveTargetLocked(towHeading float64) (resolvedTarget, bool) {
	var out resolvedTarget
	if t41, ok := e.nearestHeading41Locked(towHeading); ok {
		out = resolvedTarget{
			lat: t41.LatDeg, lon: t41.LonDeg, h: t41.HeightM,
			svsMoving:     0,
			deltaTow:      TowAbsDiffSeconds(towHeading, t41.GPSTOWSec),
			source:        "type41",
			refTow:        t41.GPSTOWSec,
			useHeadingAtt: true,
		}
		return out, true
	}
	b, ok := e.nearestMovingBaseLocked(towHeading)
	if !ok {
		return out, false
	}
	out = resolvedTarget{
		lat: b.LatDeg, lon: b.LonDeg, h: b.HeightM,
		svsMoving:     b.SVsUsed,
		deltaTow:      TowAbsDiffSeconds(towHeading, b.GPSTOWSec),
		source:        "moving_base",
		refTow:        b.GPSTOWSec,
		useHeadingAtt: false,
	}
	return out, true
}

// IngestMovingBase processes GSOF from the optional moving-base receiver.
func (e *Engine) IngestMovingBase(gsofBuffer []byte) {
	w := WalkGSOFPacket(gsofBuffer)
	e.mu.Lock()
	defer e.mu.Unlock()
	if w.Base35 != nil {
		cp := *w.Base35
		e.lastBase35Moving = &cp
	}
	if len(w.Base41Records) > 0 {
		last := w.Base41Records[len(w.Base41Records)-1]
		cp := last
		e.lastBase41Moving = &cp
	}
	e.bEpochs = append(e.bEpochs, w.Epochs...)
	e.bEpochs = trimFront(e.bEpochs, maxRing)
	e.bAtt = append(e.bAtt, w.AttitudeRanges...)
	e.bAtt = trimFront(e.bAtt, maxRing)
}

func (e *Engine) nearestHeading41Locked(towH float64) (Base41TowSample, bool) {
	var best Base41TowSample
	bestD := math.MaxFloat64
	found := false
	for _, s := range e.heading41Ring {
		d := TowAbsDiffSeconds(s.GPSTOWSec, towH)
		if d < bestD && d <= e.cfg.MatchMaxTowDeltaSec {
			bestD = d
			best = s
			found = true
		}
	}
	return best, found
}

func (e *Engine) nearestMovingBaseLocked(towH float64) (EpochSample, bool) {
	var best EpochSample
	bestD := math.MaxFloat64
	found := false
	for _, b := range e.bEpochs {
		d := TowAbsDiffSeconds(b.GPSTOWSec, towH)
		if d < bestD && d <= e.cfg.MatchMaxTowDeltaSec {
			bestD = d
			best = b
			found = true
		}
	}
	return best, found
}

func (e *Engine) referenceRangeLocked(towRef float64, attitudeFromHeading bool) (ref float64, source string, ok bool) {
	if e.cfg.ExpectedRangeM > 0 {
		return e.cfg.ExpectedRangeM, "expected_range_param", true
	}
	if !e.cfg.RangeRefFromAttitude {
		return 0, "", false
	}
	pool := e.bAtt
	if attitudeFromHeading {
		pool = e.hAtt
	}
	if len(pool) == 0 {
		return 0, "", false
	}
	var best AttitudeRangeSample
	bestD := math.MaxFloat64
	for _, a := range pool {
		d := TowAbsDiffSeconds(a.GPSTOWSec, towRef)
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
	src := "type27_range_moving_base"
	if attitudeFromHeading {
		src = "type27_range_heading"
	}
	return best.RangeM, src, true
}

// Snapshot returns a copy for JSON / SSE.
func (e *Engine) Snapshot(version string) EngineSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	snap := EngineSnapshot{
		Version:               version,
		Points:                append([]MatchedPoint(nil), e.points...),
		Base35Heading:         e.lastBase35Heading,
		Base41Heading:         e.lastBase41Heading,
		HeadingSerial:         e.headingSerial,
		Base35Moving:          e.lastBase35Moving,
		Base41Moving:          e.lastBase41Moving,
		MovingBaseConfigured:  e.cfg.MovingBaseConfigured,
		HasHeadingType41Ring: len(e.heading41Ring) > 0,
		NeedsMovingBase:       e.cfg.MovingBaseConfigured == false && len(e.heading41Ring) == 0,
	}
	return snap
}

// EngineSnapshot is JSON-serializable UI state.
type EngineSnapshot struct {
	Version string `json:"version"`
	Points  []MatchedPoint `json:"points"`

	Base35Heading *gsof.ReceivedBaseInfo          `json:"base_35_heading,omitempty"`
	Base41Heading *gsof.BasePositionQualityInfo   `json:"base_41_heading,omitempty"`
	HeadingSerial string                          `json:"heading_serial,omitempty"`

	Base35Moving *gsof.ReceivedBaseInfo           `json:"base_35_moving,omitempty"`
	Base41Moving *gsof.BasePositionQualityInfo    `json:"base_41_moving,omitempty"`

	MovingBaseConfigured bool `json:"moving_base_configured"`
	// HasHeadingType41Ring is true once at least one type 41 sample was seen on the heading stream.
	HasHeadingType41Ring bool `json:"has_heading_type41"`
	// NeedsMovingBase is true when no type-41 ring on heading and moving base transport was not configured (cannot use LLH fallback).
	NeedsMovingBase bool `json:"needs_moving_base"`
}
