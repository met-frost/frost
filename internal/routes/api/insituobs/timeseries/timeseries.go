// Package timeseries ... TO BE DOCUMENTED.
package timeseries

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/xeipuuv/gojsonschema"
	"gitlab.met.no/frost/frost/internal/common"
	"gitlab.met.no/frost/frost/internal/common/geometry"
	"gitlab.met.no/frost/frost/internal/openapi"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	"gitlab.met.no/frost/frost/internal/routes/api/timespecification"
)

// BaseID is the base ID for the time series instance. This must be unique in the context of the
// (time series, storage back-end) combination, and is typically kept internally by the storage
// back-end. Often the base ID will be the same as the ID part of hdr (hdr/id), but in some
// contexts it is convenient to keep a separate base ID, such as when hdr/id can be modified during
// the lifetime of the time series.
// NOTE: since the base ID must be globally unique for this tstype within storage back-end,
// it means that if for example the storage back-end keeps open and restricted time series in
// separate sub-storages (typically databases), then which one to access can always be derivable
// from the base ID.
type BaseID string

// Header is a JSON structure to represent the time series header.
type Header map[string]map[string]any

// FromTime is the optional beginning of the validity interval for the time series instance.
// math.MinInt64 indicates that the beginning is undefined.
type FromTime int64

// ToTime is the optional end of the validity interval for the time series instance.
// math.MinInt64 indicates that the end is undefined.
type ToTime int64

// GetBaseID returns the base ID via the receiver. This trick provides a way for implementations of
// methods in the TimeSeries interface to access the embedded BaseID field of the specific type.
// (Inspired by: https://stackoverflow.com/questions/26027350/go-interface-fields)
func (baseID *BaseID) GetBaseID() *BaseID {
	return baseID
}

// GetFromTime returns the fromTime via the receiver. (Same trick as for GetBaseID - see this)
func (t *FromTime) GetFromTime() *FromTime {
	return t
}

// GetToTime returns the toTime via the receiver. (Same trick as for GetBaseID - see this)
func (t *ToTime) GetToTime() *ToTime {
	return t
}

// Validate returns nil if h is a valid header, otherwise error.
func (h *Header) Validate() error {
	// if len(*h) < 2 {
	// 	return fmt.Errorf("invalid header (expected at least 2 keys, found %d): %v", len(*h), *h)
	// }
	if _, found := (*h)["id"]; !found {
		return fmt.Errorf("invalid header (no 'id' key found): %v", *h)
	}
	// if _, found := (*h)["extra"]; !found {
	// 	return fmt.Errorf("invalid header (no 'extra' key found): %v", *h)
	// }
	// note: there could be more keys, such as 'extra', and 'available'
	return nil
}

// ConvertKeysToLower converts all object keys in h to lowercase.
// Returns nil on success, otherwise error.
func (h *Header) ConvertKeysToLower() error {
	for k, v := range *h {
		err := common.ConvertObjKeysToLower(&v)
		if err != nil {
			return fmt.Errorf("common.ConvertObjKeysToLower() failed: %v", err)
		}
		(*h)[k] = v
	}
	return nil
}

// SchemaLoaders keeps JSON schema loaders for header and obs body.
type SchemaLoaders struct {
	HdrID, HdrExtra, ObsBody gojsonschema.JSONLoader
}

// RequestInfo keeps information about a single request.
type RequestInfo struct {
	Request     *http.Request // original request
	QueryParams url.Values    // original query parameters
	Custom      any           // information specific to the time series type
}

// PointInterval keeps a point and its validity interval.
type PointInterval struct {
	Point geometry.Point
	From  FromTime
	To    ToTime
}

type InstanceSeq []*TimeSeries

// TimeSeries is the interface that every time series type is required to implement.
type TimeSeries interface {

	// Clear clears any auxiliary structures used for this time series type.
	Clear()

	// GetBaseID returns a pointer to the BaseID field required to be embedded by every
	// time series type.
	GetBaseID() *BaseID

	// GetFromTime returns a pointer to the fromTime field required to be embedded by every
	// time series type.
	GetFromTime() *FromTime

	// GetToTime returns a pointer to the fromTime field required to be embedded by every
	// time series type.
	GetToTime() *ToTime

	// Type returns the trimmed and case-insensitive type name of the time series.
	Type() string

	// Description returns a brief description of the time series type.
	Description() string

	// CreateInstance creates a new time series instance of this type.
	// math.MinInt64 for either fromTime or toTime indicates an undefined beginning/end of the
	// validity interval (time series that are still active typically have an undefined toTime).
	//
	// IMPORTANT: A successful return from this function must not assume that the new instance will
	// actually be inserted in the main registry (it won't if an equivalent instance already
	// exists!). Any finalization of the new instance must be carried out in FinalizeInstance().
	//
	// Returns (ts, nil) on success, otherwise (..., error).
	// ### TODO: consider if hdr should be obsoleted by id and extra!
	CreateInstance(
		baseID BaseID, hdr Header, id, extra string, fromTime FromTime,
		toTime ToTime) (*TimeSeries, error)

	// FinalizeInstance is called after the new instance (tsNew) has been added to the main
	// registry as a result of calling CreateInstance (see documentation for this).
	//
	// The function is typically used for adding the instance to special structures that are
	// specific to the ts type (use cases include:
	// - geo search and other types of indexes for faster search, and
	// - normalization for less use of memory).
	//
	// Returns (ts, nil, nil) on success, otherwise
	//     (..., <any non-fatal error>, <any general error>).
	FinalizeInstance(tsNew *TimeSeries, baseID BaseID, hdr Header, id, extra string,
		fromTime FromTime, toTime ToTime) (error, error)

	// GetHeader returns the full header specific to the time series type.
	// The returned header should be equivalent to the header passed to CreateInstance().
	// Some of the header fields (typically in the hdr/extra part) may in practice be kept
	// in normalized data structures to save memory.
	GetHeader() (*Header, error)

	// GetHeaderID returns the 'id' part of the header specific to the time series type.
	// (See also GetHeader)
	GetHeaderID() (map[string]any, error)

	// UpdateExtra updates the 'extra' part of the time series instance.
	// stsextra is a serialized subset of the 'extra' part associated with the instance.
	UpdateExtra(stsextra string) error

	// UnlimitedResponse decides if the request, represented by tsSeq and tspec, qualifies for an
	// unlimited response for the time series type (for example: 1) requesting data from only a
	// single time series (i.e. potentially time intensive), or 2) requesting data from only a
	// short time period (i.e. potentially space intensive)).
	// Returns (bool, <reason why the request doesn't qualify for an unlimited response, or "" if
	// the time series type doesn't support unlimited response>, ..., nil)
	// on success, otherwise (..., ..., HTTP status code, error).
	UnlimitedResponse(
		tsSeq *InstanceSeq, tspec timespecification.TimeSpecification) (bool, string, int, error)

	// GetInstances adds to tsSeq time series instances that match queryParams and roles
	// (for authorizing access to restricted time series if applicable, and in that case typically
	// assumed to be already extracted from queryParams).
	// Returns (..., nil) on success, otherwise (HTTP status code, error).
	GetInstances(queryParams url.Values, roles []string, tsSeq *InstanceSeq) (int, error)

	// FinalizeInstanceOrder allows this ts type to reorder tsSeq to ensure that the storage
	// back-end can be implemented efficiently (e.g. by minimizing the number of SELECT calls).
	// For example, ts types lard{base|ranked} will sort on access mode (open/restricted).
	// Returns (..., nil) on success, otherwise (HTTP status code, error).
	FinalizeInstanceOrder(tsSeq *InstanceSeq) (int, error)

	// FindInstanceFromID finds the time series instance of this type that matches
	// serialized ID sid.
	// Returns (*TimeSeries, nil) if found, (nil, nil) if not found, or (..., error) on error.
	FindInstanceFromID(sid []byte) (*TimeSeries, error)

	// HeaderFilterSpecial performs additional filtering on tss by removing
	// items that don't match time series dependent filters derived from reqInfo.
	// Note: This function should only perform filtering that can not be expressed using
	// GetInstances.
	// Returns (..., nil) on success, otherwise (HTTP status code, error).
	// ### TODO: do this as part of the above GetInstances instead!
	HeaderFilterSpecial(reqInfo RequestInfo, tsSeq *InstanceSeq) (int, error)

	// HeaderPxmtyFilter filters tsSeq according to header geo points and proximity geo search
	// specified in reqInfo. Also registeres respective closest distances into each time series in
	// the resulting tsSeq.
	// Returns (HTTP status code, error) on error, otherwise (..., nil).
	HeaderPxmtyFilter(reqInfo RequestInfo, tsSeq *InstanceSeq) (int, error)

	// ObsBodyModify modifies the observation body, typically by adding extra fields derived
	// from the existing ones.
	// The function will be called before ObsFilter().
	// Returns (-1, nil) on success, otherwise (HTTP status code, error).
	ObsBodyModify(t time.Time, body *map[string]any) (int, error)

	// ObsFilter decides, based on the value of reqInfo, if the specific observation
	// (t, body) is to be kept in the output dataset.
	// The function will be called after ObsBodyModify().
	// Returns (bool, -1, nil) on success (true: keep, false: drop),
	// otherwise (..., HTTP status code, error).
	ObsFilter(t time.Time, body map[string]any, reqInfo RequestInfo) (bool, int, error)

	// ValidateHdrID validates hdrID against the schema specific to this time series type.
	// Returns nil if valid, otherwise an error with details about the validation failure.
	ValidateHdrID(hdrID any) error

	// ValidateHdrExtra validates hdrExtra against the schema specific to this time series type.
	// Returns nil if valid, otherwise an error with details about the validation failure.
	ValidateHdrExtra(hdrExtra any) error

	// ValidateObsBody validates obsBody against the schema specific to this time series type.
	// Returns nil if valid, otherwise an error with details about the validation failure.
	ValidateObsBody(obsBody any) error

	// IngestHook is called as part of handling a request to the /put endpoint and defines any
	// actions to execute for observations in dts right before writing it to the storage backend.
	//
	// The ingestHook might need access to the storage backend, but the latter is passed as
	// 'sbe any' rather than '*storagebackends.StorageBackend' to avoid importing the
	// storagebackends package as this would have caused the compiler to complain about a circular
	// dependency.
	//
	// The combinations of return values are:
	//
	// [1] (nil, nil):
	//     No errors occured. Any actions were successfully executed and all observations in dts
	//     may be written to the storage backend without any need for further follow-up processing.
	//
	// [2] (nil, recoverableErrors []error):
	//     At least one recoverable error occurred, but no fatal ones. All observations in dts
	//     may be written to the storage backend, but may need to be followed up in order to be
	//     considered completely processed. Morever, these errors may not be fixed by the ingest
	//     client. For example: one of the hook actions involved sending decoded contents from dts
	//     to an internal system S which was temporarily down, so retrying the send operation to S
	//     can be done without involving the ingest client.
	//     An element in recoverableErrors (of the same size as dts.Observations) is nil if no
	//     problems were encountered for the corresponding observation. Otherwise the element
	//     indicates the problem. The error details will typically be used for deciding exactly how
	//     to follow up later (e.g. for a Postgres storage backend we might keep a column to mark
	//     whether or not processing of an observation is considered complete).
	//     NOTE: even if the recoverable problem affects only a small subset of the contents of
	//     an observation X in dts (typically only some of the specific observations that X is
	//     made up of), it is typically fine for the ingestHook to report only the first such
	//     problem and then bail out. The reason for this is that we assume that a recovery
	//     operation will reprocess the original observation in its entirety.
	//
	// [3] (fatalErrors []error, recoverableErrors nil OR []error):
	//     At least one fatal error occurred, and possibly also at least one recoverable error.
	//     None of the observations that correspond with the fatal errors should be written to the
	//     storage backend, and those problems can only be fixed by the ingest client (by ensuring
	//     that all observation values are well-formed, for example).
	//     An element in fatalErrors (of the same size as dts.Observations) is nil if no problems
	//     were encountered for the corresponding observation. Otherwise the element indicates the
	//     problem.
	//     Observations for which no fatal errors occurred may still be written to the storage
	//     backend and handled in the same way as in case [1] or [2], but they don't have to.
	//     It should be left to the ingest client to decide whether to resend all observations in
	//     or only the ones with fatal errors. Resending an observation without a fatal error should
	//     anyway be an idempotent operation.
	//
	IngestHook(dts dataset.SingleTSeries, sbe any) ([]error, []error)

	// HeaderIDsEqual checks if hdr1 and hdr2 are equal wrt. hdr/id.
	// Returns (true|false, nil) on success, otherwise (..., error).
	HeaderIDsEqual(hdr1, hdr2 map[string]any) (bool, error)

	// CreateCustomReqInfo creates the part of a RequestInfo that is specific to the time
	// series type.
	// Returns (custom part, nil) on success, otherwise (..., error).
	CreateCustomReqInfo(queryParams url.Values) (any, error)

	// GetHeaderGeoPoints extracts any geo points defined in the time series header
	// (typically in hdr/extra). The points are returned in chronological order (the point for
	// the most recent interval at the highest index).
	//
	// Only valid geo points (with lon and lat both of type number etc.) are included in the result.
	// It is up to each time series type to decide if an invalid point should result in 1) the
	// function failing or 2) the point silently being omitted from the result.
	//
	// Returns (points, nil) on success, otherwise (..., error).
	GetHeaderGeoPoints() ([]PointInterval, error)

	// GetObsBodyGeoPoint extracts any geo point defined in obsBody.
	// Returns (point, true/false, nil) on success, otherwise (..., ..., error).
	GetObsBodyGeoPoint(obsBody map[string]any) (geometry.Point, bool, error)

	// GetSupportedQueryParams returns the supported query parameters (in trimmed, lower case form!)
	// for this time series type.
	GetSupportedQueryParams() common.StringSet

	// GetStatus returns overall status for this time series type
	GetStatus(queryParams url.Values) (any, error)

	// --- BEGIN functions used for defining the OpenAPI Specification (OAS) ---------------

	// OAGetTags ... (see documentation in OAPublisher interface)
	OAGetTags() (map[string]openapi.Tag, error)

	// OAGetDefs ... (see documentation in OAPublisher interface)
	OAGetDefs() (map[string]string, error)

	// OAGetPaths ... (see documentation in OAPublisher interface)
	OAGetPaths() ([]openapi.Path, error)

	// --- END functions used for defining the OpenAPI Specification (OAS) ---------------
}

// GetHeaderGeoPointsFromExtraPosLatLon implements GetHeaderGeoPoints() assuming a single,
// valid lat,lon position always exists at hdr/extra/pos for the time series in question.
func GetHeaderGeoPointsFromExtraPosLatLon(ts TimeSeries) ([]PointInterval, error) {

	// get header
	hdr, err := ts.GetHeader()
	if err != nil {
		return []PointInterval{}, fmt.Errorf("(*ts).GetHeader() failed: %v", err)
	}

	extra, found := (*hdr)["extra"]
	if !found {
		return []PointInterval{}, fmt.Errorf("hdr/extra not found")
	}

	posIF, found := extra["pos"]
	if !found {
		return []PointInterval{}, fmt.Errorf("hdr/extra/pos not found")
	}

	// convert to map[string]any
	pos, ok := posIF.(map[string]any)
	if !ok {
		return []PointInterval{},
			fmt.Errorf("posIF not a map[string]any: %T", posIF)
	}

	// extract longitude and latitude
	lonIF, found := pos["lon"]
	if !found {
		return []PointInterval{}, fmt.Errorf("hdr/extra/pos/lon not found")
	}
	latIF, found := pos["lat"]
	if !found {
		return []PointInterval{}, fmt.Errorf("hdr/extra/pos/lat not found")
	}

	// attempt to convert to float64
	lon, lonOk := common.ConvertIFToFloat64(lonIF)
	lat, latOk := common.ConvertIFToFloat64(latIF)

	if lonOk && latOk {
		// point valid, so return array with single, valid geo point (but with undefined from/to)
		point, err := geometry.MakePoint(lon, lat, nil)
		if err != nil {
			return []PointInterval{}, fmt.Errorf("geometry.MakePoint() failed: %v", err)
		}

		return []PointInterval{
			{Point: point, From: math.MinInt64, To: math.MinInt64},
		}, nil
	} else {
		// point invalid, so return empty array
		return []PointInterval{}, nil
	}
}

// HeaderFilterSpecialGeo removes from tsSeq items that don't match geo filtering defined by
// gsInfo.IORegions. See also documentation of HeaderFilterSpecial in TimeSeries interface.
// Returns (..., nil) on success, otherwise (HTTP status code, error).
func HeaderFilterSpecialGeo(gsInfo geometry.GeoSearchInfo, tsSeq *InstanceSeq) (
	int, error) {

	if gsInfo.MobileOnly {
		return -1, nil // n/a; leave tsSeq unmodified
	}

	ioRegions := gsInfo.IORegions
	if ioRegions == nil {
		return -1, nil // n/a; leave tsSeq unmodified
	}

	if (len(ioRegions.Inside) > 0) || (len(ioRegions.Outside) > 0) {

		// collect the matching time series into mTsSeq
		mTsSeq := InstanceSeq{}
		for _, ts := range *tsSeq {

			// extract any history of valid geo points from the time series header
			points, err := (*ts).GetHeaderGeoPoints()
			if err != nil {
				return http.StatusInternalServerError,
					fmt.Errorf("(*ts0).GetHeaderGeoPoints() failed: %v", err)
			}

			// WARNING: for now we match just the most recent location (if any)
			// (hence 'len(points)-1')
			if (len(points) > 0) && geometry.MatchesRegions(
				points[len(points)-1].Point, ioRegions.Inside, ioRegions.Outside) {
				// valid geo points found and the most recent one matches inside/outside
				// filter, so keep
				mTsSeq = append(mTsSeq, ts)
			}
		}

		*tsSeq = mTsSeq // replace tsSeq with mTsSeq

	} //else {
	// by convention, any position matches an empty geo filter, so leave tsSeq unmodified
	//}

	return -1, nil
}

// ObsBodyFilterGeo decides if a single observation body of time series ts should be kept or
// filtered out according to any geo filter in reqInfo.
// Returns (true/false, ..., nil) on success, otherwise (..., HTTP status code, error).
func ObsBodyFilterGeo(ts any, reqInfo RequestInfo, body map[string]any) (
	bool, int, error) {

	// get geo search info from request info
	gsInfo, ok := reqInfo.Custom.(geometry.GeoSearchInfo)
	if !ok {
		return false, http.StatusInternalServerError, fmt.Errorf(
			"reqInfo.Custom not of type geometry.GeoSearchInfo: %T", reqInfo.Custom)
	}

	if gsInfo.StationaryOnly {
		return true, -1, nil // n/a; don't apply filter
	}

	// filter wrt. any "inside" or "outside" parameters
	ioRegions := gsInfo.IORegions
	if (ioRegions != nil) && ((len(ioRegions.Inside) > 0) || (len(ioRegions.Outside) > 0)) {
		// convert ts to a time series
		ts0, ok := ts.(TimeSeries)
		if !ok {
			return false, http.StatusInternalServerError,
				fmt.Errorf("ts not a timeseries.TimeSeries: %T", ts)
		}

		// extract any geo point from the observation body
		point, found, err := ts0.GetObsBodyGeoPoint(body)
		if err != nil {
			return false, http.StatusInternalServerError,
				fmt.Errorf("ts0.GetObsBodyGeoPoint() failed: %v", err)
		}

		if (!found) || (!geometry.MatchesRegions(point, ioRegions.Inside, ioRegions.Outside)) {
			// either no geo point found or it didn't match; in either case filter out observation
			return false, -1, nil
		} //else {
		// geo point found and matching inside/outside filter, so keep observation
		//}
	} //else {
	// by convention, any position matches an empty inside/outside filter, so keep observation
	//}

	return true, -1, nil // keep observation
}

// setProximityDistance adds the proximity distance dist to the header of ts so that it will
// be included in the response.
func setProximityDistance(ts *TimeSeries, dist float64) error {
	hdr, err := (*ts).GetHeader()
	if err != nil {
		return fmt.Errorf("(*ts).GetHeader() failed: %v", err)
	}

	m := map[string]any{}
	m["distance"] = dist
	(*hdr)["nearest"] = m // TODO: add this as an optional field in the OpenAPI response schema!!!

	return nil
}

// HeaderPxmtyFilter filters tss according to header geo points and proximity geo search specified
// in reqInfo. A time series will be removed from tss if either 1) it's distance from the closest
// proximity point is greater than maxdist, or 2) tss already contains at least maxcount other
// time series closer to the closest proximity point.
// The distance to the closest proximity point will be registered into the respective time series
// in the final tss.
// Returns (..., nil) on success, otherwise (HTTP status code, error).
func HeaderPxmtyFilter(gsInfo geometry.GeoSearchInfo, tsSeq *InstanceSeq) (int, error) {

	if gsInfo.MobileOnly {
		return -1, nil // n/a -> leave tsSeq unmodified
		// NOTE: proximity points are only supported for potentially stationary time series, i.e.
		// time series where only geo positions in the header are considered. Supporting proximity
		// points for mobile time series would require all candidate observations to be considered
		// before returning the result to the client. This is not compatible with the fundamental
		// design of processing one time series at a time. In contrast, for stationary time series
		// we can get away with comparing geo positions in time series headers only
		// (time series headers are kept in the internal registry anyway).
	}

	ppoints := gsInfo.PxmtyPoints
	if ppoints == nil {
		return -1, nil // n/a -> leave tsSeq unmodified
	}

	// modify tss according to ppoints

	type TimeSeriesDistance struct {
		TimeSeries *TimeSeries
		Distance   float64 // distance to closest proximity point in kilometers
	}

	clist := []TimeSeriesDistance{} // candidate list

	// loop over time series
	for _, ts := range *tsSeq {
		// extract any history of valid geo points from the time series header
		points, err := (*ts).GetHeaderGeoPoints()
		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("(*ts).GetHeaderGeoPoints() failed: %v", err)
		}

		// WARNING: for now we consider just the most recent geo point (if any)
		// (hence 'len(points)-1')
		if len(points) > 0 {
			dist, err := geometry.ClosestValidDistance(points[len(points)-1].Point, *ppoints)
			if err != nil {
				return http.StatusInternalServerError,
					fmt.Errorf("geometry.ClosestValidDistance() failed: %v", err)
			}
			if dist >= 0 {
				// a closest valid (i.e. within valid height range of, and not farther away from
				// the maximum distance to, that proximity point) distance was found, so add to
				// candidate list
				clist = append(clist, TimeSeriesDistance{
					TimeSeries: ts,
					Distance:   dist,
				})
			}
		} //else {
		// since proximity points were defined and this time series didn't have a valid geo
		// point, it is not considered
		//}
	}

	// sort clist on increasing distance
	sort.Slice(clist, func(i, j int) bool {
		return clist[i].Distance < clist[j].Distance
	})

	// replace tss with clist
	*tsSeq = []*TimeSeries{}
	for i, tsd := range clist {
		*tsSeq = append(*tsSeq, tsd.TimeSeries)

		err := setProximityDistance(tsd.TimeSeries, tsd.Distance) // register to show in response
		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("setProximityDistance() failed: %v", err)
		}

		if i >= (ppoints.MaxCount - 1) {
			// keep at most ppoints.MaxCount time series
			break
		}
	}

	return -1, nil // success
}

// GetUnsupportedQueryParamsMsg returns "" if queryParams contains no unsupported query parameters
// for time series type ts, otherwise a message containing these parameters.
func GetUnsupportedQueryParamsMsg(ts *TimeSeries, queryParams url.Values) string {
	return common.GetUnsupportedQueryParamsMsg((*ts).GetSupportedQueryParams(), queryParams)
}

// GetSupportedQueryParams returns query parameters supported by all time series types
func GetSupportedQueryParams() common.StringSet {
	return common.StringSetFromList([]string{
		"time",
		"latestmaxage",
		"latestlimit",
		"itemlimit",
		"incobs",
		"inside",
		"outside",
		"polygon",
		"nearest",
		"geopostype",
	})
}

// --- BEGIN EDR support (OGC Environmental Data Retrieval) ---------------------------------

type EDRTsInfo struct {
	StationID    int64
	ParameterID  int64
	Level        int64
	Ts           *TimeSeries
	StandardName string
	Method       string
	Duration     string
	Unit         string
}

type EDRTsInfos []EDRTsInfo
// --- END EDR support (OGC Environmental Data Retrieval) ---------------------------------
