package badevann

// This file contains code specific to time series type 'badevann'.
// In particular, all timeseries.TimeSeries instances referred to in this file are
// of that time series type.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/xeipuuv/gojsonschema"
	"gitlab.met.no/frost/frost/internal/common"
	"gitlab.met.no/frost/frost/internal/common/geometry"
	"gitlab.met.no/frost/frost/internal/openapi"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	obsopenapi "gitlab.met.no/frost/frost/internal/routes/api/insituobs/openapi"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
	"gitlab.met.no/frost/frost/internal/routes/api/timespecification"
)

// --- BEGIN global types, vars and initialization ---------------------------------------------

// Badevann implements the TimeSeries interface.
type Badevann struct {
	BaseRoute string
	timeseries.BaseID       // see https://golang.org/doc/effective_go#embedding
	timeseries.Header       // ditto
	timeseries.FromTime     // ditto
	timeseries.ToTime       // ditto
}

type hdrID struct { // hdr/id part - must correspond with hdrIDSchema
	Source    string `json:"source"`
	BuoyID    string `json:"buoyid"`
	Parameter string `json:"parameter"`
}

type pos struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

type hdrExtra struct { // hdr/extra part - must correspond with hdrExtraSchema
	Name string `json:"name"`
	Pos  pos    `json:"pos"`
}

type hdrIDRegVal struct { // value part of hdrIDReg
	ts    *timeseries.TimeSeries // pointer to time series instance in global registry
	extra hdrExtra               // hdr/extra part
}

var (
	hdrIDReg map[hdrID]hdrIDRegVal // registry of all time series header IDs of type badevann

	schemaLoaders timeseries.SchemaLoaders
)

func init() {
	hdrIDReg = map[hdrID]hdrIDRegVal{}

	schemaLoaders.HdrID = gojsonschema.NewStringLoader(hdrIDSchema())
	schemaLoaders.HdrExtra = gojsonschema.NewStringLoader(hdrExtraSchema())
	schemaLoaders.ObsBody = gojsonschema.NewStringLoader(obsBodySchema())

}

// --- END global types, vars and initialization ---------------------------------------------

// BEGIN JSON schemas --------------------------------------------------------------------

// NOTE: all object keys must be lowercase
func hdrIDSchema() string {
	return `{
		"title": "header_id",
		"type": "object",
		"properties": {
			"source": {
				"type": "string"
			},
			"buoyid": {
				"type": "string"
			},
		    "parameter": {
				"type": "string"
			}
		},
		"required": ["source", "buoyid", "parameter"],
		"additionalProperties": false
	}`
}

// NOTE: all object keys must be lowercase
// TODO: Consider if lon and lat should be number instead of string. This would make ts/create fail
// for time series with e.g. "lon": "None".
func hdrExtraSchema() string {
	return `{
		"title": "header_extra",
		"type": "object",
		"properties": {
			"name": {
				"type": "string"
			},
			"pos": {
				"type": "object",
				"properties": {
					"lon": {
						"type": "string"
					},
					"lat": {
						"type": "string"
					}
				},
				"required": ["lon", "lat"],
				"additionalProperties": false
			}
		},
		"required": ["name", "pos"],
		"additionalProperties": false
	}`
}

// NOTE: all object keys must be lowercase
func obsBodySchema() string {
	return `{
		"title": "observations_body",
		"type": "object",
		"properties": {
			"value": {
				"type": "string"
			}
		},
		"required": ["value"],
		"additionalProperties": false
	}`
}

// END JSON schemas --------------------------------------------------------------------

// --- BEGIN other local functions --------------------------------------------------------------

// addToReg adds the adds the time series instance to hdrIDReg.
// Returns nil upon success, otherwise any non-fatal error.
func addToReg(ts *timeseries.TimeSeries, id, extra string) error {

	// unmarshal id into id0
	var id0 hdrID
	err := json.Unmarshal([]byte(id), &id0)
	if err != nil {
		return fmt.Errorf("json.Unmarshal(id) failed: %v", err)
	}

	// unmarshal extra into extra0
	var extra0 hdrExtra
	err = json.Unmarshal([]byte(extra), &extra0)
	if err != nil {
		return fmt.Errorf("json.Unmarshal(extra) failed: %v", err)
	}

	// add to hdrIDReg
	hdrIDReg[id0] = hdrIDRegVal{
		ts:    ts,
		extra: extra0,
	}

	return nil
}

// --- END other local functions --------------------------------------------------------------

// --- BEGIN implementation of TimeSeries interface --------------------------------------

// Clear ... (see documentation in TimeSeries interface)
func (ts *Badevann) Clear() {
	hdrIDReg = map[hdrID]hdrIDRegVal{}
}

// Type ... (see documentation in TimeSeries interface)
func (ts *Badevann) Type() string {
	return "badevann"
}

// Description ... (see documentation in TimeSeries interface)
func (ts *Badevann) Description() string {
	return "<description of the badevann time series type ...>"
}

// CreateInstance ... (see documentation in TimeSeries interface)
func (ts *Badevann) CreateInstance(
	baseID timeseries.BaseID, hdr timeseries.Header, id, extra string,
	fromTime timeseries.FromTime, toTime timeseries.ToTime) (*timeseries.TimeSeries, error) {
	var ts0 timeseries.TimeSeries = &Badevann{
		BaseID: baseID, Header: hdr, FromTime: fromTime, ToTime: toTime}
	return &ts0, nil
}

// FinalizeInstance ... (see documentation in TimeSeries interface)
func (ts *Badevann) FinalizeInstance(
	tsNew *timeseries.TimeSeries, baseID timeseries.BaseID, hdr timeseries.Header,
	id, extra string, fromTime timeseries.FromTime, toTime timeseries.ToTime) (error, error) {
	return addToReg(tsNew, id, extra), nil
}

// GetHeader ... (see documentation in TimeSeries interface)
func (ts *Badevann) GetHeader() (*timeseries.Header, error) {
	return &ts.Header, nil
}

// GetHeaderID ... (see documentation in TimeSeries interface)
func (ts *Badevann) GetHeaderID() (map[string]interface{}, error) {
	return ts.Header["id"], nil
}

// UpdateExtra ... (see documentation in TimeSeries interface)
func (ts *Badevann) UpdateExtra(mtsextra string) error {
	return fmt.Errorf("UpdateExtra() not implemented for time series type badevann")
}

// UnlimitedResponse ... (see documentation in TimeSeries interface)
func (ts *Badevann) UnlimitedResponse(
	tsSeq *timeseries.InstanceSeq, tspec timespecification.TimeSpecification) (
		bool, string, int, error) {
	return false, "", -1, nil
}

// GetInstances ... (see documentation in TimeSeries interface)
func (ts *Badevann) GetInstances(
	queryParams url.Values, tsSeq *timeseries.InstanceSeq) (int, error) {

	// extract query params (TODO: consider all instances (Get considers only the first one))
	sources := common.ExtractCSVValsLC(queryParams.Get("sources"))
	buoyids := common.ExtractCSVValsLC(queryParams.Get("buoyids"))
	parameters := common.ExtractCSVValsLC(queryParams.Get("parameters"))
	names := common.ExtractCSVValsLC(queryParams.Get("names"))

	// add matching time series to result
	for id, regVal := range hdrIDReg {
		switch {
		case !common.StringList(sources).ContainsMatch(id.Source, true):
			continue
		case !common.StringList(buoyids).ContainsMatch(id.BuoyID, true):
			continue
		case !common.StringList(parameters).ContainsMatch(id.Parameter, true):
			continue
		case !common.StringList(names).ContainsMatch(regVal.extra.Name, true):
			continue
		}
		*tsSeq = append(*tsSeq, regVal.ts) // no mismatches found
	}

	return -1, nil // no errors found
}

// FinalizeInstanceOrder ... (see documentation in TimeSeries interface)
func (ts *Badevann) FinalizeInstanceOrder(tsSeq *timeseries.InstanceSeq) (int, error) {
	_ = tsSeq // n/a
	return -1, nil
}

// FindInstanceFromID ... (see documentation in TimeSeries interface)
func (ts *Badevann) FindInstanceFromID(sid []byte) (*timeseries.TimeSeries, error) {
	var hid hdrID
	err := json.Unmarshal(sid, &hid)
	if err != nil {
		fmt.Printf("json.Unmarshal() failed: %v\n", err)
	}

	if rv, found := hdrIDReg[hid]; found {
		return rv.ts, nil // found
	}

	return nil, nil // not found
}

// HeaderFilterSpecial ... (see documentation in TimeSeries interface)
func (ts *Badevann) HeaderFilterSpecial(
	reqInfo timeseries.RequestInfo, tsSeq *timeseries.InstanceSeq) (int, error) {

	// get geo search info from custom request info
	gsInfo, ok := reqInfo.Custom.(geometry.GeoSearchInfo)
	if !ok {
		return http.StatusInternalServerError, fmt.Errorf(
			"reqInfo.Custom not of type geometry.GeoSearchInfo: %T", reqInfo.Custom)
	}

	// filter out from tsSeq time series that don't match gsInfo.IORegions
	statusCode, err := timeseries.HeaderFilterSpecialGeo(gsInfo, tsSeq)
	if err != nil {
		return statusCode, fmt.Errorf("timeseries.HeaderFilterSpecialGeo() failed: %v", err)
	}

	return -1, nil
}

// HeaderPxmtyFilter ... (see documentation in TimeSeries interface)
func (ts *Badevann) HeaderPxmtyFilter(
	reqInfo timeseries.RequestInfo, tsSeq *timeseries.InstanceSeq) (int, error) {

	// get geo search info from custom request info
	gsInfo, ok := reqInfo.Custom.(geometry.GeoSearchInfo)
	if !ok {
		return http.StatusInternalServerError, fmt.Errorf(
			"reqInfo.Custom not of type geometry.GeoSearchInfo: %T", reqInfo.Custom)
	}

	return timeseries.HeaderPxmtyFilter(gsInfo, tsSeq)
}

// ObsBodyModify ... (see documentation in TimeSeries interface)
func (ts *Badevann) ObsBodyModify(t time.Time, body *map[string]interface{}) (int, error) {
	return -1, nil // for now don't modify any obs
}

// ObsFilter ... (see documentation in TimeSeries interface)
func (ts *Badevann) ObsFilter(
	t time.Time, body map[string]interface{}, reqInfo timeseries.RequestInfo) (bool, int, error) {
	var keep bool
	var statusCode int
	var err error

	// position
	keep, statusCode, err = timeseries.ObsBodyFilterGeo(ts, reqInfo, body)
	if err != nil {
		return false, statusCode, fmt.Errorf("timeseries.ObsBodyFilterGeo() failed: %v", err)
	}
	if !keep {
		return false, -1, nil // observation filtered out
	}

	// other attributes ...

	return true, -1, nil // observation not filtered out
}

// ValidateHdrID ... (see documentation in TimeSeries interface)
func (ts *Badevann) ValidateHdrID(hdrID interface{}) error {
	return common.SchemaValidate(schemaLoaders.HdrID, hdrID)
}

// ValidateHdrExtra ... (see documentation in TimeSeries interface)
func (ts *Badevann) ValidateHdrExtra(hdrExtra interface{}) error {
	return common.SchemaValidate(schemaLoaders.HdrExtra, hdrExtra)
}

// ValidateObsBody ... (see documentation in TimeSeries interface)
func (ts *Badevann) ValidateObsBody(obsBody interface{}) error {
	return common.SchemaValidate(schemaLoaders.ObsBody, obsBody)
}

// IngestHook ... (see documentation in TimeSeries interface)
func (ts *Badevann) IngestHook(dts dataset.SingleTSeries, sbe interface{}) ([]error, []error) {
	return nil, nil // no actions
}

// HeaderIDsEqual ... (see documentation in TimeSeries interface)
func (ts *Badevann) HeaderIDsEqual(hdr1, hdr2 map[string]interface{}) (bool, error) {
	for _, key := range []string{"source", "buoyid", "parameter"} {
		if !common.StringsEqual(hdr1, hdr2, key) {
			return false, nil
		}
	}
	return true, nil // no differences found
}

// CreateCustomReqInfo ... (see documentation in TimeSeries interface)
func (ts *Badevann) CreateCustomReqInfo(queryParams url.Values) (interface{}, error) {
	gsInfo, err := geometry.GetGeoSearchInfo(queryParams)
	if err != nil {
		return nil, fmt.Errorf("geometry.GetGeoSearchInfo() failed: %v", err)
	}
	return gsInfo, nil
}

// GetHeaderGeoPoints ... (see documentation in TimeSeries interface)
func (ts *Badevann) GetHeaderGeoPoints() ([]timeseries.PointInterval, error) {
	// NOTE: as we consider hdr/extra/pos/{lat|lon} mandatory for this time series
	// type, we just propagate any error from the following call
	return timeseries.GetHeaderGeoPointsFromExtraPosLatLon(ts)
}

// GetObsBodyGeoPoint ... (see documentation in TimeSeries interface)
func (ts *Badevann) GetObsBodyGeoPoint(obsBody map[string]interface{}) (
	geometry.Point, bool, error) {
	// NOTE: a geo point is not defined in the obs body of this time series type
	return geometry.Point{}, false, nil
}

// GetSupportedQueryParams ... (see documentation in TimeSeries interface)
func (ts *Badevann) GetSupportedQueryParams() common.StringSet {
	sqp := timeseries.GetSupportedQueryParams()
	sqp.SetFromList([]string{
		"sources",
		"buoyids",
		"parameters",
		"names",
	})
	return sqp
}

// GetStatus ... (see documentation in TimeSeries interface)
func (ts *Badevann) GetStatus(queryParams url.Values) (interface{}, error) {
	return nil, fmt.Errorf("GetStatus() not implemented for times series type badevann")
}

// OAGetTags ... (see documentation in OAPublisher interface)
func (ts *Badevann) OAGetTags() (map[string]openapi.Tag, error) {

	rank := 10
	return map[string]openapi.Tag{
		fmt.Sprintf("obs/%s", ts.Type()): {
			Rank: &rank,
			Description: `<span style="background-color:#ffff99;font-weight:bold;font-size:150%">
				((work in progress))</span>`,
			DocLevel: openapi.DocLevelAdvancedOnly(),
		},
	}, nil
}

// OAGetDefs ... (see documentation in OAPublisher interface)
func (ts *Badevann) OAGetDefs() (map[string]string, error) {
	return nil, nil
}

// getGetPathParameters returns for the /badevann/get endpoint the subset of the OpenAPI
// 'parameters' part that is specific to this time series type.
func getGetPathParameters() string {

	return common.NormalizeWhitespace(`[
		{
			"name": "sources",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "The source of the buoy data (often the organization submitting and/or
				owning the buoy(s)). Use asterisks (\\*) for wildcard matching.
				__Example__: yr.no,badetassen*\n"
		},
		{
			"name": "buoyids",
			"docgroup": "where",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "A comma-separated list of internal MET Norway buoy id numbers.
				Use asterisks (\\*) for wildcard matching.
				__Example__: 12*,456\n"
		},
		{
			"name": "parameters",
			"docgroup": "what",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "A comma-separated list of [weather parameters](
				/docs/parameters#elementids).
				Use asterisks (\\*) for wildcard matching.
				__Example__: temp*\n"
		},
		{
			"name": "names",
			"docgroup": "where",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "A comma-separated list of internal MET Norway buoy names.
				Use asterisks (\\*) for wildcard matching.
				__Example__: *strand*\n"
		}
	]`)
}

// OAGetPaths ... (see documentation in OAPublisher interface)
func (ts *Badevann) OAGetPaths() ([]openapi.Path, error) {

	var err error

	tagName := fmt.Sprintf("obs/%s", ts.Type())

	tsCreateObj, err := obsopenapi.CreateTsCreateObject(
		ts.Type(), hdrIDSchema(), hdrExtraSchema(), "Create new time series",
		"obsBadevannTsCreate", tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateTsCreateObject(%s) failed: %v", ts.Type(), err)
	}

	tsDeleteObj, err := obsopenapi.CreateTsDeleteObject(
		ts.Type(), hdrIDSchema(), hdrExtraSchema(), "Delete time series", "obsBadevannTsDelete",
		tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateTsDeleteObject(%s) failed: %v", ts.Type(), err)
	}

	tsUpdateObj, err := obsopenapi.CreateTsUpdateObject(
		ts.Type(), hdrIDSchema(), hdrExtraSchema(), "Update time series (NOT YET IMPLEMENTED)",
		"obsBadevannTsUpdate", tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateTsUpdateObject(%s) failed: %v", ts.Type(), err)
	}

	putObj, err := obsopenapi.CreatePutObject(
		ts.Type(), "", `{"error": "no example provided yet"}`, hdrIDSchema(), hdrExtraSchema(),
		obsBodySchema(), "obsBadevannPut", tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreatePutObject(%s) failed: %v", ts.Type(), err)
	}

	getObj, err := obsopenapi.CreateGetObject(
		ts.Type(), getGetPathParameters(), hdrIDSchema(), hdrExtraSchema(), obsBodySchema(),
		"obsBadevannGet", tagName, openapi.DocLevelBoth())
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateGetObject(%s) failed: %v", ts.Type(), err)
	}

	statusObj, err := obsopenapi.CreateStatusObject(
		ts.Type(), "obsBadevannStatus", tagName,
		`<html><span style=\"color:red\">(TO BE DOCUMENTED)</html>`)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateStatusObject(%s) failed: %v", ts.Type(), err)
	}

	baseRouteTS := fmt.Sprintf("%s/%s", ts.BaseRoute, ts.Type())

	return []openapi.Path{
		{
			Name: fmt.Sprintf("%s/ts/create", baseRouteTS),
			Object: tsCreateObj,
		},
		{
			Name: fmt.Sprintf("%s/ts/delete", baseRouteTS),
			Object: tsDeleteObj,
		},
		{
			Name: fmt.Sprintf("%s/ts/update", baseRouteTS),
			Object: tsUpdateObj,
		},
		{
			Name: fmt.Sprintf("%s/put", baseRouteTS),
			Object: putObj,
		},
		{
			Name: fmt.Sprintf("%s/get", baseRouteTS),
			Object: getObj,
		},
		{
			Name: fmt.Sprintf("%s/status", baseRouteTS),
			Object: statusObj,
		},
	}, nil
}

// --- END implementation of TimeSeries interface --------------------------------------
