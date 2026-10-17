// Package geo converts between the coordinate datums used by different map
// providers, so mujian can store one datum and still render correctly in
// clients that expect another.
//
// Background: in mainland China, public map services licensed for distribution
// (AutoNavi/Gaode, Tencent, and Apple Maps operating inside China) use the
// GCJ-02 datum ("Mars coordinates", 国测局坐标系). Raw GPS, the WGS-84 datum,
// and the RFC 5545 ICS GEO property / geo: URI scheme all expect WGS-84.
// Feeding GCJ-02 numbers where WGS-84 is expected produces the familiar
// 300–700 m offset seen when a calendar pin is opened in iOS Maps.
//
// mujian stores venue coordinates exactly as pasted from Chinese map apps
// (GCJ-02) and converts to WGS-84 only at the output boundary (ICS export and
// OpenStreetMap map tiles). Coordinates outside the GCJ-02 region (overseas,
// where Apple Maps already yields WGS-84) are passed through unchanged.
package geo

import "math"

const (
	a  = 6378245.0                // semi-major axis of the Krasovsky 1940 ellipsoid
	ee = 0.00669342162296594323   // eccentricity squared
	pi = math.Pi
)

// OutOfChina reports whether the given coordinate lies outside the GCJ-02
// obfuscation region (mainland China). Outside this region no transform is
// applied, so the coordinate is returned unchanged.
func OutOfChina(lat, lng float64) bool {
	return !(lng > 73.66 && lng < 135.05 && lat > 3.86 && lat < 53.55)
}

func transformLat(x, y float64) float64 {
	ret := -100.0 + 2.0*x + 3.0*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*pi) + 20.0*math.Sin(2.0*x*pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(y*pi) + 40.0*math.Sin(y/3.0*pi)) * 2.0 / 3.0
	ret += (160.0*math.Sin(y/12.0*pi) + 320*math.Sin(y*pi/30.0)) * 2.0 / 3.0
	return ret
}

func transformLng(x, y float64) float64 {
	ret := 300.0 + x + 2.0*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*pi) + 20.0*math.Sin(2.0*x*pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(x*pi) + 40.0*math.Sin(x/3.0*pi)) * 2.0 / 3.0
	ret += (150.0*math.Sin(x/12.0*pi) + 300.0*math.Sin(x/30.0*pi)) * 2.0 / 3.0
	return ret
}

// WGS84ToGCJ02 converts a WGS-84 coordinate to GCJ-02.
func WGS84ToGCJ02(lat, lng float64) (float64, float64) {
	if OutOfChina(lat, lng) {
		return lat, lng
	}
	dLat := transformLat(lng-105.0, lat-35.0)
	dLng := transformLng(lng-105.0, lat-35.0)
	radLat := lat / 180.0 * pi
	magic := math.Sin(radLat)
	magic = 1 - ee*magic*magic
	sqrtMagic := math.Sqrt(magic)
	dLat = (dLat * 180.0) / ((a*(1-ee))/(magic*sqrtMagic) * pi)
	dLng = (dLng * 180.0) / (a/sqrtMagic*math.Cos(radLat) * pi)
	return lat + dLat, lng + dLng
}

// GCJ02ToWGS84 converts a GCJ-02 coordinate back to WGS-84. The inverse is
// solved by a couple of fixed-point iterations; the residual drops well below
// a centimetre, which is more than adequate for map pins and calendar
// locations.
func GCJ02ToWGS84(lat, lng float64) (float64, float64) {
	if OutOfChina(lat, lng) {
		return lat, lng
	}
	wLat, wLng := lat, lng
	for i := 0; i < 2; i++ {
		gLat, gLng := WGS84ToGCJ02(wLat, wLng)
		wLat -= gLat - lat
		wLng -= gLng - lng
	}
	return wLat, wLng
}

// BD09ToGCJ02 converts a Baidu BD-09 coordinate to GCJ-02.
func BD09ToGCJ02(lat, lng float64) (float64, float64) {
	x := lng - 0.0065
	y := lat - 0.006
	z := math.Sqrt(x*x+y*y) - 0.00002*math.Sin(y*pi)
	theta := math.Atan2(y, x) - 0.000003*math.Cos(x*pi)
	return z*math.Sin(theta) + 0.006, z*math.Cos(theta) + 0.0065
}

// GCJ02ToBD09 converts a GCJ-02 coordinate to Baidu BD-09.
func GCJ02ToBD09(lat, lng float64) (float64, float64) {
	z := math.Sqrt(lng*lng + lat*lat) + 0.00002*math.Sin(lat*pi)
	theta := math.Atan2(lat, lng) + 0.000003*math.Cos(lng*pi)
	bdLng := z*math.Cos(theta) + 0.0065
	bdLat := z*math.Sin(theta) + 0.006
	return bdLat, bdLng
}
