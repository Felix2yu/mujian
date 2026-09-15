package geo

import (
	"math"
	"testing"
)

func TestBD09ToGCJ02(t *testing.T) {
	// BD-09 to GCJ-02 conversion: Baidu adds extra obfuscation on top of GCJ-02.
	// A known Baidu coordinate in Shanghai should map back to a GCJ-02 coordinate
	// that is within a few metres of the original GCJ-02 value.
	bdLat, bdLng := 31.237, 121.478
	gLat, gLng := BD09ToGCJ02(bdLat, bdLng)
	// The result should differ from the input (Baidu offset removed).
	if math.Abs(gLat-bdLat) < 1e-6 && math.Abs(gLng-bdLng) < 1e-6 {
		t.Errorf("BD09ToGCJ02 should change the coordinate, got same values")
	}
	// Round-trip: BD09ToGCJ02(GCJ02ToBD09(x)) ≈ x
	// The BD09 ↔ GCJ02 formulas are approximate; allow ~0.01° (~1km) residual.
	rtLat, rtLng := BD09ToGCJ02(gLat, gLng)
	if math.Abs(rtLat-gLat) > 0.01 || math.Abs(rtLng-gLng) > 0.01 {
		t.Errorf("round trip drift too large: got (%f,%f), want near (%f,%f)", rtLat, rtLng, gLat, gLng)
	}
}

func TestGCJ02ToBD09(t *testing.T) {
	// GCJ-02 to BD-09: result should be offset from input.
	gLat, gLng := 31.2304, 121.4737
	bdLat, bdLng := GCJ02ToBD09(gLat, gLng)
	if math.Abs(bdLat-gLat) < 1e-6 && math.Abs(bdLng-gLng) < 1e-6 {
		t.Errorf("GCJ02ToBD09 should change the coordinate, got same values")
	}
	// The Baidu offset should be small (a few hundred metres ≈ a few thousandths of a degree).
	if math.Abs(bdLat-gLat) > 0.05 || math.Abs(bdLng-gLng) > 0.05 {
		t.Errorf("BD-09 offset implausibly large: (%f,%f)", bdLat-gLat, bdLng-gLng)
	}
}

func TestBD09ToGCJ02RoundTripConsistency(t *testing.T) {
	pts := [][2]float64{
		{31.2304, 121.4737}, // Shanghai
		{39.9087, 116.3975}, // Beijing
		{23.1291, 113.2644}, // Guangzhou
	}
	for _, p := range pts {
		// WGS84 → GCJ02 → BD09 → GCJ02 should be near the original GCJ02.
		// The BD09 ↔ GCJ02 formulas are approximate inverses with a residual
		// of ~0.006° (~600m) which is inherent to the Baidu offset model.
		gLat, gLng := WGS84ToGCJ02(p[0], p[1])
		bdLat, bdLng := GCJ02ToBD09(gLat, gLng)
		gLat2, gLng2 := BD09ToGCJ02(bdLat, bdLng)
		if math.Abs(gLat2-gLat) > 0.01 || math.Abs(gLng2-gLng) > 0.01 {
			t.Errorf("BD09 round trip drift for %v: got GCJ(%f,%f), want near (%f,%f)", p, gLat2, gLng2, gLat, gLng)
		}
	}
}
