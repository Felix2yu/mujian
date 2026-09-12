package geo

import (
	"math"
	"testing"
)

func TestOutOfChina(t *testing.T) {
	// Shanghai is inside the GCJ-02 region.
	if OutOfChina(31.2304, 121.4737) {
		t.Errorf("Shanghai should be inside China region")
	}
	// Tokyo is outside.
	if !OutOfChina(35.6812, 139.7671) {
		t.Errorf("Tokyo should be outside China region")
	}
}

func TestRoundTrip(t *testing.T) {
	pts := [][2]float64{
		{31.2304, 121.4737}, // Shanghai
		{39.9087, 116.3975}, // Beijing
		{22.5429, 114.0596}, // Shenzhen
	}
	for _, p := range pts {
		gLat, gLng := WGS84ToGCJ02(p[0], p[1])
		wLat, wLng := GCJ02ToWGS84(gLat, gLng)
		if math.Abs(wLat-p[0]) > 1e-5 || math.Abs(wLng-p[1]) > 1e-5 {
			t.Errorf("round trip drift too large for %v: got (%f,%f)", p, wLat, wLng)
		}
	}
}

func TestOffsetMagnitude(t *testing.T) {
	// Within China the GCJ-02 shift from WGS-84 should be in the
	// tens-to-hundreds-of-metres range, never zero, never absurd.
	wLat, wLng := 31.2304, 121.4737
	gLat, gLng := WGS84ToGCJ02(wLat, wLng)
	dLatM := (gLat - wLat) * 111320
	dLngM := (gLng - wLng) * 111320 * math.Cos(wLat*pi/180)
	if dLatM == 0 && dLngM == 0 {
		t.Errorf("expected a non-zero GCJ-02 offset in China")
	}
	if math.Abs(dLatM) > 2000 || math.Abs(dLngM) > 2000 {
		t.Errorf("GCJ-02 offset implausibly large: (%f,%f) m", dLatM, dLngM)
	}
}

func TestOverseasPassThrough(t *testing.T) {
	lat, lng := 35.6812, 139.7671 // Tokyo
	gLat, gLng := WGS84ToGCJ02(lat, lng)
	if gLat != lat || gLng != lng {
		t.Errorf("overseas coordinate should pass through unchanged, got (%f,%f)", gLat, gLng)
	}
	wLat, wLng := GCJ02ToWGS84(lat, lng)
	if wLat != lat || wLng != lng {
		t.Errorf("overseas coordinate should pass through unchanged, got (%f,%f)", wLat, wLng)
	}
}
