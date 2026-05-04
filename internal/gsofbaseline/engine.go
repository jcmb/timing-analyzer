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

	lastHeadingRover *EpochSample
	lastHeading27    *gsof.AttitudePoint
	headingAttRing   []gsof.AttitudePoint
	lastHeading38Text string
	movingBaseSerial string
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
	if w.LastAttitude27 != nil {
		v := *w.LastAttitude27
		e.lastHeading27 = &v
	}
	for _, ap := range w.Heading27All {
		e.headingAttRing = append(e.headingAttRing, ap)
	}
	e.headingAttRing = trimFront(e.headingAttRing, maxRing)
	if len(w.Last38Payload) > 0 {
		fields := gsof.Decode(38, w.Last38Payload)
		e.lastHeading38Text = FormatGSOFFieldsForCard(fields)
	}
	if len(w.Epochs) > 0 {
		le := w.Epochs[len(w.Epochs)-1]
		cp := le
		e.lastHeadingRover = &cp
	}

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
	if w.Serial15 != nil {
		e.movingBaseSerial = fmt.Sprintf("%d", *w.Serial15)
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
		HeadingRover:          e.lastHeadingRover,
		HeadingType27:         e.lastHeading27,
		Base35Moving:          e.lastBase35Moving,
		Base41Moving:          e.lastBase41Moving,
		MovingBaseSerial:      e.movingBaseSerial,
		MovingBaseConfigured:  e.cfg.MovingBaseConfigured,
		HasHeadingType41Ring: len(e.heading41Ring) > 0,
		NeedsMovingBase:       e.cfg.MovingBaseConfigured == false && len(e.heading41Ring) == 0,
	}
	cc := e.crossCheckHeading41VsMovingBaseLocked()
	if cc != nil {
		snap.Heading41VsMovingBase = cc
	}
	snap.HeadingCheck = e.computeHeadingCheckLocked()
	snap.HeadingType38 = e.lastHeading38Text
	return snap
}

// HeadingCheckResult compares the latest computed bearing to type-27 yaw when a
// type-27 sample exists within the same TOW match window as the last solution.
type HeadingCheckResult struct {
	Available bool `json:"available"`
	// ComputedBearingDeg is initial bearing heading → reference (0–360°, MatchedPoint).
	ComputedBearingDeg float64 `json:"computed_bearing_deg"`
	// Type27YawDeg is GSOF type-27 yaw (degrees).
	Type27YawDeg float64 `json:"type27_yaw_deg"`
	// DeltaDeg is signed shortest angle (computed − yaw) in (−180, 180].
	DeltaDeg    float64 `json:"delta_deg"`
	AbsDeltaDeg float64 `json:"abs_delta_deg"`
	TowLastPointSec float64 `json:"tow_last_point_s"`
	TowType27Sec    float64 `json:"tow_type27_s"`
	TowDeltaSec     float64 `json:"tow_delta_s"`
	Note            string `json:"note,omitempty"`
}

func (e *Engine) computeHeadingCheckLocked() *HeadingCheckResult {
	if len(e.points) == 0 {
		return &HeadingCheckResult{Available: false, Note: "no_bearing_solution"}
	}
	last := e.points[len(e.points)-1]
	var best gsof.AttitudePoint
	bestD := math.MaxFloat64
	found := false
	for _, a := range e.headingAttRing {
		d := TowAbsDiffSeconds(a.GPSTOWSec, last.GPSTOWSec)
		if d <= e.cfg.MatchMaxTowDeltaSec && d < bestD {
			bestD = d
			best = a
			found = true
		}
	}
	if !found && e.lastHeading27 != nil {
		d := TowAbsDiffSeconds(e.lastHeading27.GPSTOWSec, last.GPSTOWSec)
		if d <= e.cfg.MatchMaxTowDeltaSec {
			best = *e.lastHeading27
			bestD = d
			found = true
		}
	}
	if !found {
		return &HeadingCheckResult{
			Available:       false,
			Note:            "no_type27_within_match_window",
			TowLastPointSec: last.GPSTOWSec,
		}
	}
	signed := AngleDiffDegSigned(last.BearingDeg, best.YawDeg)
	return &HeadingCheckResult{
		Available:          true,
		ComputedBearingDeg: last.BearingDeg,
		Type27YawDeg:       best.YawDeg,
		DeltaDeg:           signed,
		AbsDeltaDeg:        math.Abs(signed),
		TowLastPointSec:    last.GPSTOWSec,
		TowType27Sec:       best.GPSTOWSec,
		TowDeltaSec:        bestD,
	}
}

func (e *Engine) crossCheckHeading41VsMovingBaseLocked() *CrossCheckHeading41Moving {
	if !e.cfg.MovingBaseConfigured {
		return nil
	}
	var tow41, lat41, lon41, h41 float64
	var have41 bool
	if e.lastBase41Heading != nil {
		tow41 = e.lastBase41Heading.GPSTOWSec
		lat41 = e.lastBase41Heading.LatDeg
		lon41 = e.lastBase41Heading.LonDeg
		h41 = e.lastBase41Heading.HeightM
		have41 = true
	} else if len(e.heading41Ring) > 0 {
		s := e.heading41Ring[len(e.heading41Ring)-1]
		tow41, lat41, lon41, h41 = s.GPSTOWSec, s.LatDeg, s.LonDeg, s.HeightM
		have41 = true
	}
	if !have41 {
		return nil
	}
	mb, ok := e.nearestMovingBaseLocked(tow41)
	if !ok {
		return &CrossCheckHeading41Moving{
			Active:         true,
			HasPair:        false,
			MatchOK:        false,
			TowType41Sec:   tow41,
			Note:           "no moving-base type 1/2 within match window of heading type 41 TOW",
		}
	}
	hd := math.Abs(h41 - mb.HeightM)
	horiz := HaversineM(lat41, lon41, mb.LatDeg, mb.LonDeg)
	dTow := TowAbsDiffSeconds(tow41, mb.GPSTOWSec)
	okMatch := horiz <= crossCheckHorizTolM && hd <= crossCheckHeightTolM
	return &CrossCheckHeading41Moving{
		Active:           true,
		HasPair:          true,
		MatchOK:          okMatch,
		HorizontalDiffM:  horiz,
		HeightDiffM:      hd,
		TowType41Sec:     tow41,
		TowMovingBaseSec: mb.GPSTOWSec,
		TowDeltaSec:      dTow,
	}
}

// CrossCheckHeading41Moving compares heading-stream type 41 to moving-base type 1+2 LLH
// when both streams are in use (same physical base / rover at base site).
type CrossCheckHeading41Moving struct {
	Active bool `json:"active"`
	// HasPair is true when a moving-base type 1/2 epoch was found within the TOW match window.
	HasPair bool `json:"has_pair"`
	// MatchOK is true when HasPair and horizontal and height deltas are within tolerance.
	MatchOK bool `json:"match_ok"`
	// HorizontalDiffM is great-circle distance between type-41 position and moving-base LLH (metres).
	HorizontalDiffM float64 `json:"horizontal_diff_m"`
	HeightDiffM     float64 `json:"height_diff_m"`
	TowType41Sec    float64 `json:"tow_type41_s"`
	TowMovingBaseSec float64 `json:"tow_moving_base_s"`
	TowDeltaSec     float64 `json:"tow_delta_s"`
	Note            string `json:"note,omitempty"`
}

const crossCheckHorizTolM = 5.0
const crossCheckHeightTolM = 5.0

// EngineSnapshot is JSON-serializable UI state.
type EngineSnapshot struct {
	Version string `json:"version"`
	Points  []MatchedPoint `json:"points"`

	Base35Heading *gsof.ReceivedBaseInfo        `json:"base_35_heading,omitempty"`
	Base41Heading *gsof.BasePositionQualityInfo `json:"base_41_heading,omitempty"`
	HeadingSerial string                        `json:"heading_serial,omitempty"`
	HeadingRover  *EpochSample                  `json:"heading_rover,omitempty"`
	HeadingType27 *gsof.AttitudePoint           `json:"heading_type27,omitempty"`

	Base35Moving *gsof.ReceivedBaseInfo        `json:"base_35_moving,omitempty"`
	Base41Moving *gsof.BasePositionQualityInfo `json:"base_41_moving,omitempty"`

	MovingBaseSerial     string `json:"moving_base_serial,omitempty"`
	MovingBaseConfigured bool   `json:"moving_base_configured"`
	// HasHeadingType41Ring is true once at least one type 41 sample was seen on the heading stream.
	HasHeadingType41Ring bool `json:"has_heading_type41"`
	// NeedsMovingBase is true when no type-41 ring on heading and moving base transport was not configured (cannot use LLH fallback).
	NeedsMovingBase bool `json:"needs_moving_base"`

	// Heading41VsMovingBase is set when moving base is configured and heading type 41 exists; compares to matched moving-base epoch.
	Heading41VsMovingBase *CrossCheckHeading41Moving `json:"heading_41_vs_moving_base,omitempty"`

	HeadingCheck *HeadingCheckResult `json:"heading_check,omitempty"`
	HeadingType38 string               `json:"heading_type38,omitempty"`
}
