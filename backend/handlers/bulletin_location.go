package handlers

import "math"

// Bulletin posts never store where someone actually is. The device sends its
// position, and the server snaps it to the centre of a grid cell roughly 250 m
// across before anything is written. Distances shown to other people are
// measured to that cell, and rounded again for display.
const (
	bulletinGridLatStep = 0.00225 // ≈ 250 m north-south
	bulletinGridLngStep = 0.00285 // ≈ 250 m east-west at San Francisco's latitude
)

type sfNeighborhood struct {
	Name  string
	Rings [][][2]float64 // each ring is {lat, lng} points
}

func snapBulletinCoordinate(lat, lng float64) (float64, float64) {
	snap := func(v, step float64) float64 {
		cell := math.Floor(v / step)
		return round5((cell + 0.5) * step)
	}
	return snap(lat, bulletinGridLatStep), snap(lng, bulletinGridLngStep)
}

func round5(v float64) float64 {
	return math.Round(v*1e5) / 1e5
}

// bulletinAreaLabel names the San Francisco neighborhood a point falls in, or
// returns "" outside the city. Uses the snapped point, so the label can never
// be more precise than the stored location.
func bulletinAreaLabel(lat, lng float64) string {
	for _, n := range sfAnalysisNeighborhoods {
		for _, ring := range n.Rings {
			if pointInRing(lat, lng, ring) {
				return n.Name
			}
		}
	}
	return ""
}

// pointInRing is the standard even-odd ray cast.
func pointInRing(lat, lng float64, ring [][2]float64) bool {
	inside := false
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		yi, xi := ring[i][0], ring[i][1]
		yj, xj := ring[j][0], ring[j][1]
		if (yi > lat) != (yj > lat) && lng < (xj-xi)*(lat-yi)/(yj-yi)+xi {
			inside = !inside
		}
	}
	return inside
}

// roundBulletinDistance rounds a distance for display: tenths of a mile, never
// below 0.1, so a neighbour in the same cell reads "0.1 mi" rather than "0".
func roundBulletinDistance(miles float64) float64 {
	r := math.Round(miles*10) / 10
	if r < 0.1 {
		return 0.1
	}
	return r
}

func validBulletinCoordinate(lat, lng float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lng) &&
		lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 &&
		!(lat == 0 && lng == 0)
}

func haversineMiles(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusMiles = 3958.8
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	h := math.Pow(math.Sin(dLat/2), 2) + math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Pow(math.Sin(dLng/2), 2)
	return 2 * earthRadiusMiles * math.Asin(math.Sqrt(h))
}
