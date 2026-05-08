package geometry

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	"gitlab.met.no/frost/frost/internal/common"
)

var (
	regionSchemaLoader      map[string]gojsonschema.JSONLoader
	pxmtyPointsSchemaLoader gojsonschema.JSONLoader
	earthRadius             float64 = 6371 // average earth radius in kilometers
)

func init() {
	regionSchemaLoader = map[string]gojsonschema.JSONLoader{
		"common":   gojsonschema.NewStringLoader(commonRegionSchema()),
		"polygon":  gojsonschema.NewStringLoader(polygonRegionSchema()),
		"circle":   gojsonschema.NewStringLoader(circleRegionSchema()),
		"lonrange": gojsonschema.NewStringLoader(lonRangeRegionSchema()),
		"latrange": gojsonschema.NewStringLoader(latRangeRegionSchema()),
	}
	pxmtyPointsSchemaLoader = gojsonschema.NewStringLoader(pxmtyPointsSchema())
}

// MakePoint creates a Point where lon and lat are assumed to be degrees with ranges
// [-180, 180] and [-90, 90] respectively, and height is assumed to be meters above mean sea level.
// The height is optional; nil means that no height is provided.
// Returns (Point, nil) upon success, otherwise (..., error).
func MakePoint(lon, lat float64, height *float64) (Point, error) {
	lonRad, err := validLonDeg2rad(lon)
	if err != nil {
		return Point{}, fmt.Errorf("validLonDeg2rad() failed: %v", err)
	}
	latRad, err := validLatDeg2rad(lat)
	if err != nil {
		return Point{}, fmt.Errorf("validLatDeg2rad() failed: %v", err)
	}

	return Point{
		Lon:    lonRad,
		Lat:    latRad,
		Height: height,
	}, nil
}

// HorPos represents a spherical coordinate, i.e. a (horizontal) position on the unit sphere
// (see https://en.wikipedia.org/wiki/Spherical_coordinate_system").
// Examples:
//   South pole: HorPos{Lon: ANY, Lat: -math.Pi / 2}
//       (i.e. HorPos{Lon: ANY, Lat:  (-90 / 90) * math.Pi / 2})
//   North pole: HorPos{Lon: ANY, Lat: math.Pi / 2}
//       (i.e. HorPos{Lon: ANY, Lat:  (90 / 90) * math.Pi / 2})
//   Oslo: HorPos{Lon: 0.1876, Lat: 1.0463}
//       (i.e. HorPos{Lon: (10.75 / 180) * math.Pi, Lat: (59.95 / 90) * math.Pi / 2})
type HorPos struct {
	Lon float64 `json:"lon"` // longitude (azimuth) in the range [-math.Pi, math.Pi]
	Lat float64 `json:"lat"` // latitude (inclination) in the range [-math.Pi / 2, math.Pi / 2]
}

// deg2rad converts d from degrees to radians.
func deg2rad(d float64) float64 {
	return (d / 180) * math.Pi
}

// Rad2deg converts r from radians to degrees.
func Rad2deg(r float64) float64 {
	return (r / math.Pi) * 180
}

// validLonDeg2rad validates d as longitude degrees and converts it to radians.
// Returns (longitude radians, nil) if d is in range [-180, 180], otherwise (..., error).
func validLonDeg2rad(d float64) (float64, error) {
	if (d < -180) || (d > 180) {
		return 0, fmt.Errorf("longitude degrees outside valid range [-180, 180]: %v", d)
	}
	return deg2rad(d), nil
}

// validLatDeg2rad validates d as latitude degrees and converts it to radians.
// Returns (latitude radians, nil) if d is in range [-90, 90], otherwise (..., error).
func validLatDeg2rad(d float64) (float64, error) {
	if (d < -90) || (d > 90) {
		return 0, fmt.Errorf("latitude degrees outside valid range [-90, 90]: %v", d)
	}
	return deg2rad(d), nil
}

// validDeg2rad validates pos as lon/lat degrees and converts it to radians.
// Returns (converted pos, nil) upon success, otherwise (..., error).
func (pos HorPos) validDeg2rad() (HorPos, error) {
	lonRad, err := validLonDeg2rad(pos.Lon)
	if err != nil {
		return HorPos{}, fmt.Errorf("validLonDeg2rad() failed: %v", err)
	}
	latRad, err := validLatDeg2rad(pos.Lat)
	if err != nil {
		return HorPos{}, fmt.Errorf("validLatDeg2rad() failed: %v", err)
	}

	return HorPos{
		Lon: lonRad,
		Lat: latRad,
	}, nil
}

// Point represents a spherical 3D point, i.e. a (lon, lat, height).
type Point struct {
	Lon    float64  `json:"lon"` // longitude (azimuth) in the range [-math.Pi, math.Pi]
	Lat    float64  `json:"lat"` // latitude (inclination) in the range [-math.Pi / 2, math.Pi / 2]
	Height *float64 // optional height in MAMSL (meters above medium sea level; may be negative)
}

// validDeg2rad2 validates the lon/lat components of p as degrees and converts them to radians.
// Returns (converted Point, nil) upon success, otherwise (..., error).
func (p Point) validDeg2rad2() (Point, error) {
	lonRad, err := validLonDeg2rad(p.Lon)
	if err != nil {
		return Point{}, fmt.Errorf("validLonDeg2rad() failed: %v", err)
	}
	latRad, err := validLatDeg2rad(p.Lat)
	if err != nil {
		return Point{}, fmt.Errorf("validLatDeg2rad() failed: %v", err)
	}

	return Point{
		Lon:    lonRad,
		Lat:    latRad,
		Height: p.Height, // leave unchanged
	}, nil
}

// HorPolygon represents a polygon on the unit sphere.
type HorPolygon []HorPos // vertexes (positions on the unit sphere)

// size returns the size of a polygon.
func (polygon *HorPolygon) size() int {
	return len(*polygon)
}

// _3DPoint represents a position in Cartesian 3D space.
type _3DPoint struct {
	c [3]float64
}

func make3DPointFromSpherical(lon, lat float64) _3DPoint {
	return _3DPoint{c: [3]float64{
		math.Cos(lon) * math.Cos(lat),
		math.Sin(lon) * math.Cos(lat),
		math.Sin(lat),
	}}
}

func make3DPointFromCrossProduct(a, b _3DPoint) _3DPoint {
	return _3DPoint{c: [3]float64{
		a.c[1]*b.c[2] - a.c[2]*b.c[1],
		a.c[2]*b.c[0] - a.c[0]*b.c[2],
		a.c[0]*b.c[1] - a.c[1]*b.c[0],
	}}
}

func dotProduct(a, b _3DPoint) float64 {
	return a.c[0]*b.c[0] + a.c[1]*b.c[1] + a.c[2]*b.c[2]
}

func (p *_3DPoint) norm() float64 {
	return math.Sqrt(p.c[0]*p.c[0] + p.c[1]*p.c[1] + p.c[2]*p.c[2])
}

// greatCircleArcsIntersect decides if two arcs along the great circle intersect each other.
// Returns true iff the great circle arc from p1 to p2 intersects the great circle arc from p3 to
// p4.
// NOTE: If the two great circles lie (approximately) in the same plane, the function returns false
// by definition (although there are infinitely many intersections in that case).
// If the arcs intersect, the intersection point is returned in isctPos if non-nil.
func greatCircleArcsIntersect(p1, p2, p3, p4 HorPos, isctPos *HorPos) bool {
	// Adopted from http://www.mathworks.com/matlabcentral/newsreader/view_thread/276271 .
	// Alternative approaches:
	//   1: http://stackoverflow.com/questions/2954337/great-circle-rhumb-line-intersection
	//   2: http://www.boeing-727.com/Data/fly%20odds/distance.html
	//   3: http://www.movable-type.co.uk/scripts/latlong-vectors.html

	a0 := make3DPointFromSpherical(p1.Lon, p1.Lat)
	a1 := make3DPointFromSpherical(p2.Lon, p2.Lat)
	b0 := make3DPointFromSpherical(p3.Lon, p3.Lat)
	b1 := make3DPointFromSpherical(p4.Lon, p4.Lat)

	p := make3DPointFromCrossProduct(a0, a1)
	q := make3DPointFromCrossProduct(b0, b1)

	t := make3DPointFromCrossProduct(p, q)
	if t.norm() < math.SmallestNonzeroFloat64 {
		return false // arcs lie (approximately) in the same plane => no intersections by definition
	}

	s1 := dotProduct(make3DPointFromCrossProduct(p, a0), t)
	s2 := dotProduct(make3DPointFromCrossProduct(a1, p), t)
	s3 := dotProduct(make3DPointFromCrossProduct(q, b0), t)
	s4 := dotProduct(make3DPointFromCrossProduct(b1, q), t)

	sign := 0.0
	switch {
	case (s1 > 0) && (s2 > 0) && (s3 > 0) && (s4 > 0):
		sign = 1
	case (s1 < 0) && (s2 < 0) && (s3 < 0) && (s4 < 0):
		sign = -1
	default:
		return false // the arcs don't intersect (or maybe lie in the same plane
		// if this wasn't detected by the above test for this?)
	}

	// the arcs intersect

	if isctPos != nil { // return intersection position
		// ### should the below be '(*isctPos).Lon = ...' instead of 'isctPos.Lon = ...' ???
		isctPos.Lon = math.Atan2(sign*t.c[1], sign*t.c[0])
		isctPos.Lat = math.Atan2(sign*t.c[2], math.Sqrt(t.c[0]*t.c[0]+t.c[1]*t.c[1]))
	}

	return true
}

// Contains decides if polygon contains a position.
//
// If extPos is non-nil, it defines a position that is considered outside of the polygon,
// otherwise such an external position is automatically derived from the polygon.
// WARNING: the result of such a derivation might be incorrect for certain extreme cases that are
// not assumed to occur in practice (TODO: document these cases).
//
// An edge is implicitly assumed between the first and last vertex of the polygon.
//
// WARNING: If the polygon is self-intersecting, the result is undefined.
//
// Returns true or false if pos is respectively inside or outside of the polygon.
func (polygon *HorPolygon) Contains(pos HorPos, extPos *HorPos) bool {
	// assert(polygon.size() >= 3)

	// define, in extHPos0, a position considered outside of the polygon
	var extHPos0 HorPos
	if extPos == nil {
		avgLon := 0.0
		minLat := (*polygon)[0].Lat
		maxLat := (*polygon)[0].Lat
		for _, p := range *polygon {
			avgLon += p.Lon
			minLat = math.Min(p.Lat, minLat)
			maxLat = math.Max(p.Lat, maxLat)
		}
		avgLon /= float64(polygon.size())
		extHPos0 = HorPos{Lon: avgLon, Lat: 0.99 * math.Pi / 2}
		if (math.Pi/2 - maxLat) < (minLat - (-math.Pi / 2)) {
			// polygon is closer to the north pole, so use a position close to the south pole
			// as the external position
			extHPos0.Lat = -extHPos0.Lat
		}
	} else {
		extHPos0 = *extPos // use extPos directly
	}

	// compute the number of intersections between 1) the arc from pos to the external position
	// and 2) arcs forming the polygon
	nisct := 0
	for i := range *polygon {
		if greatCircleArcsIntersect(
			pos, extHPos0, (*polygon)[i], (*polygon)[(i+1)%polygon.size()], nil) {
			nisct++
		}
	}

	// pos is considered inside the polygon if there is an odd number of intersections
	return (nisct % 2) == 1
}

// DistanceTo computes the great-circle distance between p0 and p1, assuming that:
//   - p0 and p1 are both located on the surface of a sphere with radius 1
//   - longitude values are radians in range [-PI, PI]
//   - latitude values are radians in range [-PI/2, PI/2]
// Returns distance in radians.
func (p0 *Point) DistanceTo(p1 HorPos) float64 {
	theta0 := p0.Lon
	phi0 := p0.Lat
	theta1 := p1.Lon
	phi1 := p1.Lat

	dphi := phi1 - phi0
	dtheta := theta1 - theta0

	a :=
		math.Sin(dphi/2)*math.Sin(dphi/2) +
			math.Cos(phi0)*math.Cos(phi1)*
				math.Sin(dtheta/2)*math.Sin(dtheta/2)

	return 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

type Region interface {
	// Returns true iff region contains p.
	ContainsPoint(p Point) bool
}

func commonRegionSchema() string {
	return `{
		"title": "common_region",
		"type": "object",
		"properties": {
			"type": {"enum": ["polygon", "circle", "lonrange", "latrange"]}
		},
		"required": ["type"],
		"additionalProperties": true
	}`
}

// HeightRange represents a height range in terms of meters above the mean sea level.
// A negative height indicates a location below the mean sea level.
type HeightRange struct {
	Min *float64 `json:"minheight"`
	Max *float64 `json:"maxheight"`
}

// Validate validates height range hr.
// Returns nil if height range is valid, otherwise error.
func (hr *HeightRange) Validate() error {
	// assert(hr != nil)
	if (hr.Min != nil) && (hr.Max != nil) && (*hr.Min > *hr.Max) {
		return fmt.Errorf("negative height range: %v > %v", *hr.Min, *hr.Max)
	}
	return nil
}

// Contains returns true iff h (in meters above mean sea level) is considered within height
// range hr.
func (hr *HeightRange) Contains(h *float64) bool {
	if h == nil {
		return true // by convention
	}
	// assert(hr != nil)
	if (hr.Min != nil) && (*h < *hr.Min) {
		return false
	}
	if (hr.Max != nil) && (*h > *hr.Max) {
		return false
	}
	return true
}

// HeightOffsetRange represents a height offset range in terms of meters above the mean sea level
// to be added to an absolute regular height range.
// A negative height indicates a location below the mean sea level.
type HeightOffsetRange struct {
	Min *float64 `json:"minheightoffset"`
	Max *float64 `json:"maxheightoffset"`
}

// Validate validates height offset range hr.
// Returns nil if height offset range is valid, otherwise error.
func (hor *HeightOffsetRange) Validate() error {
	// assert(hor != nil)
	if (hor.Min != nil) && (hor.Max != nil) && (*hor.Min > *hor.Max) {
		return fmt.Errorf("negative height offset range: %v > %v", *hor.Min, *hor.Max)
	}
	return nil
}

// PolygonRegion implements the Region interface.
type PolygonRegion struct {
	HeightRange
	Polygon HorPolygon `json:"pos"`    // polygon on the unit sphere
	ExtPos  *HorPos    `json:"extpos"` // optional, arbitrary position to define the outside
	// of the polygon
}

func (p *PolygonRegion) UnmarshalJSON(data []byte) error {
	// unmarshal into generic map
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	// extract optional height range
	if err := common.ExtractOptionalFloat64Values(m, []common.KeyValueOptionalFloat64{
		{Key: "minheight", Value: &(*p).Min},
		{Key: "maxheight", Value: &(*p).Max},
	}); err != nil {
		return fmt.Errorf("common.ExtractOptionalFloat64Values() failed: %v", err)
	}

	// extract optional external position
	if extPosIF, found := m["extpos"]; found {
		if err := common.UnmarshalIF(extPosIF, &p.ExtPos); err != nil {
			return fmt.Errorf("common.UnmarshalIF(extpos) failed: %v", err)
		}
	}

	// extract polygon
	if posIF, found := m["pos"]; found {
		if err := common.UnmarshalIF(posIF, &p.Polygon); err != nil {
			return fmt.Errorf("common.UnmarshalIF(pos) failed: %v", err)
		}
	} else {
		return fmt.Errorf("pos not found")
	}

	return nil
}

func polygonRegionSchema() string {
	return `{
		"title": "polygon_region",
		"type": "object",
		"properties": {
			"type": {"const": "polygon"},
			"minheight": {"type": "number"},
			"maxheight": {"type": "number"},
			"pos": {
				"type": "array",
				"minItems": 3,
				"items": {
					"type": "object",
					"properties": {
						"lon": {"type": "number"},
						"lat": {"type": "number"}
					},
					"required": ["lon", "lat"],
					"additionalProperties": false
				}
			},
			"extpos": {
				"type": "object",
				"properties": {
					"lon": {"type": "number"},
					"lat": {"type": "number"}
				},
				"required": ["lon", "lat"],
				"additionalProperties": false
			}
		},
		"required": ["type", "pos"],
		"additionalProperties": false
	}`
}

// newPolygonRegion creates a PolygonRegion from serialized representation sreg.
// Returns (new region, nil) upon success, otherwise (nil, error).
func newPolygonRegion(sreg []byte) (*PolygonRegion, error) {
	var r PolygonRegion

	// deserialize from sreg to r
	err := json.Unmarshal(sreg, &r)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	// validate r.HeightRange
	err = r.Validate()
	if err != nil {
		return nil, fmt.Errorf("r.Validate() failed: %v", err)
	}

	// convert lon/lat positions in r.Polygon to radians
	for i, pos := range r.Polygon {
		r.Polygon[i], err = pos.validDeg2rad()
		if err != nil {
			return nil, fmt.Errorf("pos.validDeg2rad() failed: %v", err)
		}
	}

	// convert any r.ExtPos from degrees to radians
	if r.ExtPos != nil {
		*r.ExtPos, err = (*r.ExtPos).validDeg2rad()
		if err != nil {
			return nil, fmt.Errorf("(*r.ExtPos).validDeg2rad() failed: %v", err)
		}
	}

	return &r, nil
}

func (r *PolygonRegion) ContainsPoint(p Point) bool {
	return r.Contains(p.Height) && r.Polygon.Contains(
		HorPos{Lon: p.Lon, Lat: p.Lat}, r.ExtPos)
}

// CircleRegion implements the Region interface.
type CircleRegion struct {
	HeightRange
	Pos    HorPos  // position on unit sphere
	Radius float64 // radius in kilometers
}

func (r *CircleRegion) UnmarshalJSON(data []byte) error {
	// unmarshal into generic map
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	// extract optional values
	if err := common.ExtractOptionalFloat64Values(m, []common.KeyValueOptionalFloat64{
		{Key: "minheight", Value: &(*r).Min},
		{Key: "maxheight", Value: &(*r).Max},
	}); err != nil {
		return fmt.Errorf("common.ExtractOptionalFloat64Values() failed: %v", err)
	}

	// extract required values
	if err := common.ExtractFloat64Values(m, []common.KeyValueFloat64{
		{Key: "lon", Value: &(*r).Pos.Lon},
		{Key: "lat", Value: &(*r).Pos.Lat},
		{Key: "radius", Value: &(*r).Radius},
	}); err != nil {
		return fmt.Errorf("common.ExtractFloat64Values() failed: %v", err)
	}

	return nil
}

func circleRegionSchema() string {
	return `{
		"title": "circle_region",
		"type": "object",
		"properties": {
			"type": {"const": "circle"},
			"minheight": {"type": "number"},
			"maxheight": {"type": "number"},
			"lon": {"type": "number"},
			"lat": {"type": "number"},
			"radius": {"type": "number"}
		},
		"required": ["type", "lon", "lat", "radius"],
		"additionalProperties": false
	}`
}

// newCircleRegion creates a CircleRegion from serialized representation sreg.
// Returns (new region, nil) upon success, otherwise (nil, error).
func newCircleRegion(sreg []byte) (*CircleRegion, error) {
	var r CircleRegion

	// deserialize from sreg to r
	err := json.Unmarshal(sreg, &r)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	// validate r.HeightRange
	err = r.Validate()
	if err != nil {
		return nil, fmt.Errorf("r.Validate() failed: %v", err)
	}

	// convert r.Pos from degrees to radians
	r.Pos, err = r.Pos.validDeg2rad()
	if err != nil {
		return nil, fmt.Errorf("r.Pos.validDeg2rad() failed: %v", err)
	}

	// validate r.Radius
	if r.Radius < 0 {
		return nil, fmt.Errorf("negative radius: %v", r.Radius)
	}

	return &r, nil
}

func (r *CircleRegion) ContainsPoint(p Point) bool {
	return r.Contains(p.Height) && ((&Point{Lon: r.Pos.Lon, Lat: r.Pos.Lat}).DistanceTo(
		HorPos{Lon: p.Lon, Lat: p.Lat})*earthRadius <= r.Radius)
}

// LonRangeRegion implements the Region interface.
type LonRangeRegion struct {
	HeightRange
	Min float64 // minimum longitude in radians [-PI, PI]
	Max float64 // maximum longitude in radians [-PI, PI]
}

func (r *LonRangeRegion) UnmarshalJSON(data []byte) error {
	// unmarshal into generic map
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	// extract optional values
	if err := common.ExtractOptionalFloat64Values(m, []common.KeyValueOptionalFloat64{
		{Key: "minheight", Value: &(*r).HeightRange.Min},
		{Key: "maxheight", Value: &(*r).HeightRange.Max},
	}); err != nil {
		return fmt.Errorf("common.ExtractOptionalFloat64Values() failed: %v", err)
	}

	// extract required values
	if err := common.ExtractFloat64Values(m, []common.KeyValueFloat64{
		{Key: "min", Value: &(*r).Min},
		{Key: "max", Value: &(*r).Max},
	}); err != nil {
		return fmt.Errorf("common.ExtractFloat64Values() failed: %v", err)
	}

	return nil
}

func lonRangeRegionSchema() string {
	// note that min and max are both required since the date line (at -PI and PI) doesn't
	// represent a natural default minimum/maximum value for a longitude (in contrast to the
	// poles at -PI/2 and PI/2 for a latitude)
	return `{
		"title": "lonrange_region",
		"type": "object",
		"properties": {
			"type": {"const": "lonrange"},
			"minheight": {"type": "number"},
			"maxheight": {"type": "number"},
			"min": {"type": "number"},
			"max": {"type": "number"}
		},
		"required": ["type", "min", "max"],
		"additionalProperties": false
	}`
}

// newLonRangeRegion creates a LonRangeRegion from serialized representation sreg.
// Returns (new region, nil) upon success, otherwise (nil, error).
func newLonRangeRegion(sreg []byte) (*LonRangeRegion, error) {
	var r LonRangeRegion

	// deserialize from sreg to r
	err := json.Unmarshal(sreg, &r)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	// validate r.HeightRange
	err = r.Validate()
	if err != nil {
		return nil, fmt.Errorf("r.Validate() failed: %v", err)
	}

	// validate r.Min/Max as degrees and convert to radians
	r.Min, err = validLonDeg2rad(r.Min)
	if err != nil {
		return nil, fmt.Errorf("validLonDeg2rad() failed: %v", err)
	}
	r.Max, err = validLonDeg2rad(r.Max)
	if err != nil {
		return nil, fmt.Errorf("validLonDeg2rad() failed: %v", err)
	}

	// note: since we need to support longitude ranges that include the date line (-PI/PI), we
	// don't require r.Min <= r.Max

	return &r, nil
}

func (r *LonRangeRegion) ContainsPoint(p Point) bool {
	if !r.Contains(p.Height) {
		return false
	}

	if r.Min <= r.Max {
		// range does not contains date line (-PI/PI), so p must be in range [r.Min, r.Max]
		return (r.Min <= p.Lon) && (p.Lon <= r.Max)
	}

	// range contains date line (PI), so p can be in either of the two ranges [r.Min, PI] or
	// [-PI, r.Max]
	return (r.Min <= p.Lon) || (p.Lon <= r.Max)
}

// LatRangeRegion implements the Region interface.
type LatRangeRegion struct {
	HeightRange
	Min *float64 // minimum latitude in radians [-PI/2, PI/2]
	Max *float64 // maximum latitude in radians [-PI/2, PI/2]
}

func (r *LatRangeRegion) UnmarshalJSON(data []byte) error {
	// unmarshal into generic map
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	// extract optional values
	if err := common.ExtractOptionalFloat64Values(m, []common.KeyValueOptionalFloat64{
		{Key: "minheight", Value: &(*r).HeightRange.Min},
		{Key: "maxheight", Value: &(*r).HeightRange.Max},
		{Key: "min", Value: &(*r).Min},
		{Key: "max", Value: &(*r).Max},
	}); err != nil {
		return fmt.Errorf("common.ExtractOptionalFloat64Values() failed: %v", err)
	}

	return nil
}

func latRangeRegionSchema() string {
	return `{
		"title": "latrange_region",
		"type": "object",
		"properties": {
			"type": {"const": "latrange"},
			"minheight": {"type": "number"},
			"maxheight": {"type": "number"},
			"min": {"type": "number"},
			"max": {"type": "number"}
		},
		"required": ["type"],
		"additionalProperties": false
	}`
}

// newLatRangeRegion creates a LatRangeRegion from serialized representation sreg.
// Returns (new region, nil) upon success, otherwise (nil, error).
func newLatRangeRegion(sreg []byte) (*LatRangeRegion, error) {
	var r LatRangeRegion

	// deserialize from sreg to r
	err := json.Unmarshal(sreg, &r)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	// validate r.HeightRange
	err = r.Validate()
	if err != nil {
		return nil, fmt.Errorf("r.Validate() failed: %v", err)
	}

	// validate any r.Min/Max as degrees and convert to radians
	if r.Min != nil {
		*r.Min, err = validLatDeg2rad(*r.Min)
		if err != nil {
			return nil, fmt.Errorf("validLatDeg2rad() failed: %v", err)
		}
	}
	if r.Max != nil {
		*r.Max, err = validLatDeg2rad(*r.Max)
		if err != nil {
			return nil, fmt.Errorf("validLatDeg2rad() failed: %v", err)
		}
	}

	// check that any r.Min <= any r.Max
	if (r.Min != nil) && (r.Max != nil) && (*r.Min > *r.Max) {
		return nil, fmt.Errorf("negative latitude range: %v > %v", *r.Min, *r.Max)
	}

	return &r, nil
}

func (r *LatRangeRegion) ContainsPoint(p Point) bool {
	if !r.Contains(p.Height) {
		return false
	}
	if (r.Min != nil) && (p.Lat < *r.Min) {
		return false
	}
	if (r.Max != nil) && (p.Lat > *r.Max) {
		return false
	}
	return true
}

func schemaValidateRegion(rtype string, region any) error {
	jsonLoader, found := regionSchemaLoader[rtype]
	if !found {
		return fmt.Errorf("unknown region type: %s", rtype)
	}
	err := common.SchemaValidate(jsonLoader, region)
	if err != nil {
		return fmt.Errorf("common.SchemaValidate(%s) failed: %v", rtype, err)
	}
	return nil
}

func getRegionType(region map[string]any) (string, error) {
	err := schemaValidateRegion("common", region)
	if err != nil {
		return "", fmt.Errorf("schemaValidateRegion() failed: %v", err)
	}

	rtypeIF, found := region["type"]
	if !found {
		return "", fmt.Errorf("no 'type' found in region: %v\n", region)
	}

	rtype, ok := rtypeIF.(string)
	if !ok {
		return "", fmt.Errorf("region 'type' not a string (%v); type: %T", rtypeIF, rtypeIF)
	}

	return rtype, nil
}

// newRegion creates a new Region instance from serialized region object sreg.
// Returns (region, nil) upon success, otherwise (nil, error).
func newRegion(sreg []byte) (*Region, error) {
	var regionMap map[string]any
	err := json.Unmarshal(sreg, &regionMap)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	rtype, err := getRegionType(regionMap)
	if err != nil {
		return nil, fmt.Errorf("getRegionType() failed: %v", err)
	}

	err = schemaValidateRegion(rtype, regionMap)
	if err != nil {
		return nil, fmt.Errorf("schemaValidateRegion() failed: %v", err)
	}

	var region Region

	switch rtype {
	case "polygon":
		region, err = newPolygonRegion(sreg)
		if err != nil {
			return nil, fmt.Errorf("newPolygonRegion() failed: %v", err)
		}
	case "circle":
		region, err = newCircleRegion(sreg)
		if err != nil {
			return nil, fmt.Errorf("newCircleRegion() failed: %v", err)
		}
	case "lonrange":
		region, err = newLonRangeRegion(sreg)
		if err != nil {
			return nil, fmt.Errorf("newLonRangeRegion() failed: %v", err)
		}
	case "latrange":
		region, err = newLatRangeRegion(sreg)
		if err != nil {
			return nil, fmt.Errorf("newLatRangeRegion() failed: %v", err)
		}
	default:
		return nil, fmt.Errorf(
			"unknown region type: %s; expected one of polygon, circle,"+
				" lonrange, or latrange", rtype)
	}

	return &region, nil
}

// replacePolygonWithInside modifies queryParams so that any occurrence of "polygon=..." is
// replaced with the corresponding "inside=..." expression.
// Returns (<modified queryParams>, nil) upon success, otherwise (nil, error).
func replacePolygonWithInside(queryParams url.Values) (url.Values, error) {
	if len(queryParams["polygon"]) > 0 {
		// validate
		if len(queryParams["polygon"]) > 1 {
			return nil, fmt.Errorf(
				"at most one 'polygon' query parameter allowed; found %d",
				len(queryParams["polygon"]))
		}
		if len(queryParams["inside"]) > 1 {
			return nil, fmt.Errorf("query parameters 'polygon' and 'inside' cannot be combined")
		}

		// replace "polygon=..." with the corresponding "inside=..." form
		queryParams["inside"] = []string{
			fmt.Sprintf(`[{"type": "polygon", "pos": %s}]`, queryParams["polygon"][0]),
		}
		delete(queryParams, "polygon")
	}

	return queryParams, nil
}

// extractInsideRegions extracts regions from any "inside" parameters in queryParams.
// Each "inside" parameter contains a comma-separated list of JSON objects, each of which represents
// a region.
// Upon success the function returns (two-level array, nil) where the outer and inner array
// represents respectively the "inside" parameters and the regions of each.
// Upon failure the function returns (nil, error).
func extractInsideRegions(queryParams url.Values) ([][]*Region, error) {

	regions := [][]*Region{} // outer array

	queryParams, err := replacePolygonWithInside(queryParams)
	if err != nil {
		return nil, fmt.Errorf("replacePolygonWithInside() failed: %s", err)
	}

	if insides, found := queryParams["inside"]; found {
		for _, inside := range insides {

			var items []map[string]any
			err := json.Unmarshal([]byte(inside), &items)
			if err != nil {
				return nil, fmt.Errorf("json.Unmarshal() failed: %v", err)
			}

			regions2 := []*Region{} // inner array (regions of a single "inside" param.)
			for _, item := range items {
				sreg, err := json.Marshal(item)
				if err != nil {
					return nil, fmt.Errorf("json.Marshal() failed: %v", err)
				}
				reg, err := newRegion(sreg)
				if err != nil {
					return nil, fmt.Errorf(
						"failed to extract inside region: %s", strings.TrimSpace(err.Error()))
				}
				regions2 = append(regions2, reg) // add region to inner array
			}
			regions = append(regions, regions2) // add inner array to outer array
		}
	}

	return regions, nil
}

// extractOutsideRegions extracts regions from any "outside" parameter in queryParams.
// An "outside" parameter contains a comma-separated list of JSON objects, each of which represents
// a region.
// Upon success the function returns (regions of any "outside" parameter, nil), otherwise
// (nil, error).
func extractOutsideRegions(queryParams url.Values) ([]*Region, error) {
	regions := []*Region{}

	if outsides, found := queryParams["outside"]; found {
		if len(outsides) > 1 {
			return nil,
				fmt.Errorf("expected at most one 'outside' parameter, found %d", len(outsides))
		}

		var items []map[string]any
		err := json.Unmarshal([]byte(outsides[0]), &items)
		if err != nil {
			return nil, fmt.Errorf("json.Unmarshal() failed: %v", err)
		}

		for _, item := range items {
			sreg, err := json.Marshal(item)
			if err != nil {
				return nil, fmt.Errorf("json.Marshal() failed: %v", err)
			}
			reg, err := newRegion(sreg)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to extract outside region: %s", strings.TrimSpace(err.Error()))
			}
			regions = append(regions, reg) // add region
		}
	}

	return regions, nil
}

// regionsIncludePoint decides if point p is contained within a two-level array of 'inside' regions.
// Returns true if p is contained in at least one inner-level region in each of the region lists
// at the outer level, otherwise false.
// By organizing regions differently in the inner- and outer levels, different combinations of
// union/intersection matching can be achieved.
func regionsIncludePoint(p Point, regions [][]*Region) bool {
	// matchOuter decides if p matches at outer level; p always matches an empty regions array,
	// hence the default value of true
	matchOuter := true

	for _, regOuter := range regions { // loop over region lists at the outer level

		// matchInner decides if p matches at inner level; p always matches an empty region list,
		// hence the default value of true
		matchInner := true

		if len(regOuter) > 0 {
			matchInner = false // now p needs to match at least one region in the list

			// loop over region list
			for _, regInner := range regOuter {
				if (*regInner).ContainsPoint(p) { // found a match, so we're done
					matchInner = true
					break
				}
			}
		}

		if !matchInner {
			// region list was empty or none of the regions of this list matched => overall mismatch
			matchOuter = false
			break
		} else {
			// matchOuter still true at this point
		}
	}

	if matchOuter {
		// overall match, since either 1) regions array is empty or 2) p matches at least one
		// region in every region list at the outer level
		return true
	}

	// overall mismatch, since both 1) regions array is non-empty, and 2) at least one outer-level
	// region list exists where p doesn't match any region in that list
	return false
}

// regionsExcludePoint decides if point p is not contained within an array of 'outside' regions.
// Returns true if p is not contained in any region, otherwise false.
func regionsExcludePoint(p Point, regions []*Region) bool {
	for _, r := range regions {
		if (*r).ContainsPoint(p) {
			// at least one of the 'outside' regions contained the point => overall mismatch
			return false
		}
	}

	// none of the 'outside' regions contained the point => overall match
	return true
}

// MatchesRegions returns true iff p matches both insideRegions and outsideRegions.
func MatchesRegions(p Point, insideRegions [][]*Region, outsideRegions []*Region) bool {
	if (len(insideRegions) > 0) && (!regionsIncludePoint(p, insideRegions)) {
		return false // 'inside' mismatch
	}
	if (len(outsideRegions) > 0) && (!regionsExcludePoint(p, outsideRegions)) {
		return false // 'outside' mismatch
	}
	return true // no mismatch found
}

type InOutRegions struct {
	Inside  [][]*Region
	Outside []*Region
}

// createInOutRegions creates an InOutRegions object from queryParams.
// Returns (InOutRegions{...}, nil) upon success, otherwise (..., error).
func createInOutRegions(queryParams url.Values) (InOutRegions, error) {
	insideRegions, err := extractInsideRegions(queryParams)
	if err != nil {
		return InOutRegions{}, fmt.Errorf("extractInsideRegions() failed: %v", err)
	}

	outsideRegions, err := extractOutsideRegions(queryParams)
	if err != nil {
		return InOutRegions{}, fmt.Errorf("extractOutsideRegions() failed: %v", err)
	}

	return InOutRegions{
		Inside:  insideRegions,
		Outside: outsideRegions,
	}, nil
}

type ProximityPoints struct {
	MaxDist  float64 `json:"maxdist"`  // maximum distance from pxmty point (kilometers)
	MaxCount int     `json:"maxcount"` // maximum number of resulting points
	HeightOffsetRange
	Points []Point `json:"points"`
}

func pxmtyPointsSchema() string {
	return `{
		"title": "proximity_points",
		"type": "object",
		"properties": {
			"maxdist": {"type": "number", "mimimum": 0},
			"maxcount": {"type": "number", "mimimum": 0},
			"minheightoffset": {"type": "number"},
			"maxheightoffset": {"type": "number"},
			"points": {
				"type": "array",
				"minItems": 1,
				"items": {
					"type": "object",
					"properties": {
						"lon": {"type": "number"},
						"lat": {"type": "number"},
						"height": {"type": "number"}
					},
					"required": ["lon", "lat"],
					"additionalProperties": false
				}
			}
		},
		"required": ["maxdist", "maxcount", "points"],
		"additionalProperties": false
	}`
}

func schemaValidatePxmtyPoints(pxmtyPoints any) error {
	err := common.SchemaValidate(pxmtyPointsSchemaLoader, pxmtyPoints)
	if err != nil {
		return fmt.Errorf("common.SchemaValidate(pxmtyPoints) failed: %v", err)
	}
	return nil
}

// newPxmtyPoints creates a ProximityPoints from serialized representation sppoints.
// Returns (ProximityPoints, nil) upon success, otherwise (..., error).
func newPxmtyPoints(sppoints []byte) (ProximityPoints, error) {
	var ppoints ProximityPoints

	// deserialize from sppoints to ppoints
	err := json.Unmarshal(sppoints, &ppoints)
	if err != nil {
		return ProximityPoints{}, fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	// validate ppoints.HeightRange
	err = ppoints.Validate()
	if err != nil {
		return ProximityPoints{}, fmt.Errorf("ppoints.Validate() failed: %v", err)
	}

	// convert lon/lat positions in ppoints.Points to radians
	for i, point := range ppoints.Points {
		ppoints.Points[i], err = point.validDeg2rad2()
		if err != nil {
			return ProximityPoints{}, fmt.Errorf("point.Pos.validDeg2rad() failed: %v", err)
		}
	}

	return ppoints, nil
}

// createPxmtyPoints creates any ProximityPoints object from queryParams, indicating whether
// one was found at all.
// Returns (ProximityPoints, found, nil) upon success, otherwise (..., ..., error).
func createPxmtyPoints(queryParams url.Values) (ProximityPoints, bool, error) {
	nearests, found := queryParams["nearest"]

	if !found {
		return ProximityPoints{}, false, nil
	}

	if len(nearests) > 1 {
		return ProximityPoints{}, false, fmt.Errorf(
			"at most one 'nearest' query parameter allowed; found %d", len(nearests))
	}

	var nearest map[string]any
	err := json.Unmarshal([]byte(nearests[0]), &nearest)
	if err != nil {
		return ProximityPoints{}, false, fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	err = schemaValidatePxmtyPoints(nearest)
	if err != nil {
		return ProximityPoints{}, false,
			fmt.Errorf("schemaValidatePxmtyPoints() failed: %v", err)
	}

	sppoints, err := json.Marshal(nearest)
	if err != nil {
		return ProximityPoints{}, false, fmt.Errorf("json.Marshal() failed: %v", err)
	}

	pxmtyPoints, err := newPxmtyPoints(sppoints)
	if err != nil {
		return ProximityPoints{}, false, fmt.Errorf("newPxmtyPoints() failed: %v", err)
	}

	return pxmtyPoints, true, nil // found
}

// ClosestValidDistance computes the lowest horizontal distance (in kilometers along the great
// circle) that p has to any of the points px in proximity points ppoints where 1) p.Height is
// within the height interval
// [px.Height + ppoints.HeightOffsetRange.Min, px.Height + ppoints.HeightOffsetRange.Max]
// and 2) the horizontal distance is within ppoints.MaxDist kilometers from px.
// Returns (non-negative distance, nil) if a valid lowest distance can be found,
// (negative distance, nil) if not, and (..., error) if an error occurs.
func ClosestValidDistance(p Point, ppoints ProximityPoints) (float64, error) {
	var closestDist float64 = -1 // no valid closest distance found yet

	// loop over proximity points
	for _, pp := range ppoints.Points {

		if pp.Height != nil { // check if p is within height range of pp
			ppHeight := *pp.Height

			// convert to absolute height range
			hr := HeightRange{}
			if ppoints.Min != nil {
				absPPMinHeight := ppHeight + *ppoints.Min
				hr.Min = &absPPMinHeight
			}
			if ppoints.Max != nil {
				absPPMaxHeight := ppHeight + *ppoints.Max
				hr.Min = &absPPMaxHeight
			}

			if !hr.Contains(p.Height) {
				continue // too low or high to be considered
			}
		} //else {
			// skip height filtering altogether
		//}

		dist := p.DistanceTo(HorPos{Lon: pp.Lon, Lat: pp.Lat}) * earthRadius // distance in kilometers
		// assert(dist >= 0)
		//fmt.Printf("dist: %v\n", dist)

		if dist > ppoints.MaxDist {
			continue // too far away to be considered
		}

		if (closestDist < 0) || (dist < closestDist) {
			closestDist = dist // update closest distance
		}
	}

	return closestDist, nil
}

type GeoSearchInfo struct {
	IORegions   *InOutRegions
	PxmtyPoints *ProximityPoints
	StationaryOnly  bool
	MobileOnly      bool
}

// GetGeoSearchInfo extracts available geo search info from queryParams.
// Returns (GeoSearchInfo{...}, nil) upon success, otherwise (..., error).
func GetGeoSearchInfo(queryParams url.Values) (GeoSearchInfo, error) {

	var gsInfo GeoSearchInfo

	// get any inside/outside regions
	ioRegions, err := createInOutRegions(queryParams)
	if err != nil {
		return GeoSearchInfo{}, fmt.Errorf("CreateInOutRegions() failed: %v", err)
	}
	if (len(ioRegions.Inside) > 0) || (len(ioRegions.Outside) > 0) {
		gsInfo.IORegions = &ioRegions
	}

	// get any proximity points
	pxmtyPoints, found, err := createPxmtyPoints(queryParams)
	if err != nil {
		return GeoSearchInfo{}, fmt.Errorf("CreatePxmtyPoints() failed: %v", err)
	}
	if found {
		gsInfo.PxmtyPoints = &pxmtyPoints
	}

	// get any "geopostype" query parameter
	if geopostypes, found := queryParams["geopostype"]; found {
		if len(geopostypes) > 1 {
			return GeoSearchInfo{}, fmt.Errorf(
				"at most one 'geopostype' query parameter allowed; found %d", len(geopostypes))
		}
		gptype := strings.ToLower(strings.TrimSpace(geopostypes[0]))
		switch gptype {
		case "stationary":
			gsInfo.StationaryOnly = true
			gsInfo.MobileOnly = false
		case "mobile":
			gsInfo.StationaryOnly = false
			gsInfo.MobileOnly = true
		default:
			return GeoSearchInfo{}, fmt.Errorf(
				"invalid 'geopostype': %s; expected one of 'stationary' or 'mobile'",
				gptype)
		}
	} else {
		// by default:
		gsInfo.StationaryOnly = true
		gsInfo.MobileOnly = false
	}

	return gsInfo, nil
}
