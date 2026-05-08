package glider

// This file contains code specific to time series type 'glider'.
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

// Glider implements the TimeSeries interface.
type Glider struct {
	BaseRoute           string
	timeseries.BaseID   // see https://golang.org/doc/effective_go#embedding
	timeseries.Header   // ditto
	timeseries.FromTime // ditto
	timeseries.ToTime   // ditto
}

type hdrID struct { // hdr/id part - must correspond with hdrIDSchema
	Source    string `json:"source"`
	GliderID  string `json:"gliderid"`
	Parameter string `json:"parameter"`
}

type hdrExtra struct { // hdr/extra part - must correspond with hdrExtraSchema
	Name string `json:"name"`
}

type hdrIDRegVal struct { // value part of hdrIDReg
	ts    *timeseries.TimeSeries // pointer to time series instance in global registry
	extra hdrExtra               // hdr/extra part
}

var (
	hdrIDReg map[hdrID]hdrIDRegVal // registry of all time series header IDs of type glider

	schemaLoaders timeseries.SchemaLoaders
)

func init() {
	hdrIDReg = map[hdrID]hdrIDRegVal{}

	schemaLoaders.HdrID = gojsonschema.NewStringLoader(hdrIDSchema())
	schemaLoaders.HdrExtra = gojsonschema.NewStringLoader(hdrExtraSchema())
	schemaLoaders.ObsBody = gojsonschema.NewStringLoader(obsBodySchema())
}

// --- END global types, vars and initialization ---------------------------------------------

// --- BEGIN JSON schemas --------------------------------------------------------------

// NOTE: all object keys must be lowercase
func hdrIDSchema() string {
	return `{
		"title": "header_id",
		"type": "object",
		"properties": {
			"source": {
				"type": "string"
			},
			"gliderid": {
				"type": "string"
			},
		    "parameter": {
				"type": "string"
			}
		},
		"required": ["source", "gliderid", "parameter"],
		"additionalProperties": false
	}`
}

// NOTE: all object keys must be lowercase
func hdrExtraSchema() string {
	return `{
		"title": "header_extra",
		"oneOf": [
			{
				"type": "null"
			},
			{
				"type": "object",
				"properties": {
					"name": {
						"type": "string"
					}
				},
				"required": [],
				"additionalProperties": false
			}
		]
	}`
}

// NOTE: all object keys must be lowercase
func obsBodySchema() string {
	return `{
		"title": "observations_body",
		"type": "object",
		"properties": {
			"pos": {
				"type": "object",
				"properties": {
					"lon": {
						"type": "number",
						"description": "longitude degrees",
						"example": 4.70036
					},
					"lat": {
						"type": "number",
						"description": "latitude degrees",
						"example": 60.30059
					},
					"depth": {
						"type": "number"
					},
					"qc_flag": {
						"type": "string"
					}
				},
				"required": ["lon", "lat", "depth"],
				"additionalProperties": false
			},
			"value": {
				"type": "number"
			},
			"qc_flag": {
				"type": "string"
			}
		},
		"required": ["pos", "value"],
		"additionalProperties": false
	}`
}

// --- END JSON schemas --------------------------------------------------------------

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
func (ts *Glider) Clear() {
	hdrIDReg = map[hdrID]hdrIDRegVal{}
}

// Type ... (see documentation in TimeSeries interface)
func (ts *Glider) Type() string {
	return "glider"
}

// Description ... (see documentation in TimeSeries interface)
func (ts *Glider) Description() string {
	return "<description of the glider time series type ...>"
}

// CreateInstance ... (see documentation in TimeSeries interface)
func (ts *Glider) CreateInstance(
	baseID timeseries.BaseID, hdr timeseries.Header, id, extra string,
	fromTime timeseries.FromTime, toTime timeseries.ToTime) (*timeseries.TimeSeries, error) {
	var ts0 timeseries.TimeSeries = &Glider{
		BaseID: baseID, Header: hdr, FromTime: fromTime, ToTime: toTime}
	return &ts0, nil
}

// FinalizeInstance ... (see documentation in TimeSeries interface)
func (ts *Glider) FinalizeInstance(
	tsNew *timeseries.TimeSeries, baseID timeseries.BaseID, hdr timeseries.Header,
	id, extra string, fromTime timeseries.FromTime, toTime timeseries.ToTime) (error, error) {
	return addToReg(tsNew, id, extra), nil
}

// GetHeader ... (see documentation in TimeSeries interface)
func (ts *Glider) GetHeader() (*timeseries.Header, error) {
	return &ts.Header, nil
}

// GetHeaderID ... (see documentation in TimeSeries interface)
func (ts *Glider) GetHeaderID() (map[string]interface{}, error) {
	return ts.Header["id"], nil
}

// UpdateExtra ... (see documentation in TimeSeries interface)
func (ts *Glider) UpdateExtra(mtsextra string) error {
	return fmt.Errorf("UpdateExtra() not implemented for time series type glider")
}

// UnlimitedResponse ... (see documentation in TimeSeries interface)
func (ts *Glider) UnlimitedResponse(
	tsSeq *timeseries.InstanceSeq, tspec timespecification.TimeSpecification) (
	bool, string, int, error) {
	return false, "", -1, nil
}

// GetInstances ... (see documentation in TimeSeries interface)
func (ts *Glider) GetInstances(
	queryParams url.Values, roles []string, tsSeq *timeseries.InstanceSeq) (int, error) {

	_ = roles // n/a

	// extract query params (TODO: consider all instances (Get considers only the first one))
	sources := common.ExtractCSVValsLC(queryParams.Get("sources"))
	gliderids := common.ExtractCSVValsLC(queryParams.Get("gliderids"))
	parameters := common.ExtractCSVValsLC(queryParams.Get("parameters"))
	names := common.ExtractCSVValsLC(queryParams.Get("names"))

	// add matching time series to result
	for id, regVal := range hdrIDReg {
		switch {
		case !common.StringList(sources).ContainsMatch(id.Source, true):
			continue
		case !common.StringList(gliderids).ContainsMatch(id.GliderID, true):
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
func (ts *Glider) FinalizeInstanceOrder(tsSeq *timeseries.InstanceSeq) (int, error) {
	_ = tsSeq // n/a
	return -1, nil
}

// FindInstanceFromID ... (see documentation in TimeSeries interface)
func (ts *Glider) FindInstanceFromID(sid []byte) (*timeseries.TimeSeries, error) {
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
func (ts *Glider) HeaderFilterSpecial(
	reqInfo timeseries.RequestInfo, tsSeq *timeseries.InstanceSeq) (int, error) {

	// get geo search info from custom request info
	gsInfo, ok := reqInfo.Custom.(geometry.GeoSearchInfo)
	if !ok {
		return http.StatusInternalServerError, fmt.Errorf(
			"reqInfo.Custom not of type geometry.GeoSearchInfo: %T", reqInfo.Custom)
	}

	// filter out from tsSeq time series that don't match inside/outside regions
	statusCode, err := timeseries.HeaderFilterSpecialGeo(gsInfo, tsSeq)
	if err != nil {
		return statusCode, fmt.Errorf("timeseries.HeaderFilterSpecialGeo() failed: %v", err)
	}

	return -1, nil
}

// HeaderPxmtyFilter ... (see documentation in TimeSeries interface)
func (ts *Glider) HeaderPxmtyFilter(
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
func (ts *Glider) ObsBodyModify(t time.Time, body *map[string]interface{}) (int, error) {
	return -1, nil // for now don't modify any obs
}

// ObsFilter ... (see documentation in TimeSeries interface)
func (ts *Glider) ObsFilter(
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
func (ts *Glider) ValidateHdrID(hdrID interface{}) error {
	return common.SchemaValidate(schemaLoaders.HdrID, hdrID)
}

// ValidateHdrExtra ... (see documentation in TimeSeries interface)
func (ts *Glider) ValidateHdrExtra(hdrExtra interface{}) error {
	return common.SchemaValidate(schemaLoaders.HdrExtra, hdrExtra)
}

// ValidateObsBody ... (see documentation in TimeSeries interface)
func (ts *Glider) ValidateObsBody(obsBody interface{}) error {
	return common.SchemaValidate(schemaLoaders.ObsBody, obsBody)
}

// IngestHook ... (see documentation in TimeSeries interface)
func (ts *Glider) IngestHook(dts dataset.SingleTSeries, sbe interface{}) ([]error, []error) {
	return nil, nil // no actions
}

// HeaderIDsEqual ... (see documentation in TimeSeries interface)
func (ts *Glider) HeaderIDsEqual(hdr1, hdr2 map[string]interface{}) (bool, error) {
	for _, key := range []string{"source", "gliderid", "parameter"} {
		if !common.StringsEqual(hdr1, hdr2, key) {
			return false, nil
		}
	}
	return true, nil // no differences found
}

// CreateCustomReqInfo ... (see documentation in TimeSeries interface)
func (ts *Glider) CreateCustomReqInfo(queryParams url.Values) (interface{}, error) {
	gsInfo, err := geometry.GetGeoSearchInfo(queryParams)
	if err != nil {
		return nil, fmt.Errorf("geometry.GetGeoSearchInfo() failed: %v", err)
	}
	return gsInfo, nil
}

// GetHeaderGeoPoints ... (see documentation in TimeSeries interface)
func (ts *Glider) GetHeaderGeoPoints() ([]timeseries.PointInterval, error) {
	// NOTE: a geo point is not defined in the header of this time series type
	return []timeseries.PointInterval{}, nil
}

// GetObsBodyGeoPoint ... (see documentation in TimeSeries interface)
func (ts *Glider) GetObsBodyGeoPoint(obsBody map[string]interface{}) (
	geometry.Point, bool, error) {
	// NOTE: a (lon, lat, depth) geo point is mandatory in the obs body of this time series type

	posIF, found := obsBody["pos"]
	if !found {
		return geometry.Point{}, false, fmt.Errorf("obs/body/pos not found")
	}

	// convert to map[string]interface{}
	pos, ok := posIF.(map[string]interface{})
	if !ok {
		return geometry.Point{}, false,
			fmt.Errorf("posIF not a map[string]interface{}: %T", posIF)
	}

	// extract longitude, latitude, and depth
	lonIF, found := pos["lon"]
	if !found {
		return geometry.Point{}, false, fmt.Errorf("obs/body/pos/lon not found")
	}
	latIF, found := pos["lat"]
	if !found {
		return geometry.Point{}, false, fmt.Errorf("obs/body/pos/lat not found")
	}
	depthIF, found := pos["depth"]
	if !found {
		return geometry.Point{}, false, fmt.Errorf("obs/body/pos/depth not found")
	}

	// convert to float64
	lon, ok := common.ConvertIFToFloat64(lonIF)
	if !ok {
		return geometry.Point{}, false,
			fmt.Errorf("longitude not convertible to float64: %v (type: %T)", lonIF, lonIF)
	}
	lat, ok := common.ConvertIFToFloat64(latIF)
	if !ok {
		return geometry.Point{}, false,
			fmt.Errorf("latitude not convertible to float64: %v (type: %T)", latIF, latIF)
	}
	depth, ok := common.ConvertIFToFloat64(depthIF)
	if !ok {
		return geometry.Point{}, false,
			fmt.Errorf("depth not convertible to float64: %v (type: %T)", depthIF, depthIF)
	}

	height := -depth // convert depth to meters *above* mean sea level
	point, err := geometry.MakePoint(lon, lat, &height)
	if err != nil {
		return geometry.Point{}, false, fmt.Errorf("geometry.MakePoint() failed: %v", err)
	}

	return point, true, nil // return (mandatory) geo point
}

// GetSupportedQueryParams ... (see documentation in TimeSeries interface)
func (ts *Glider) GetSupportedQueryParams() common.StringSet {
	sqp := timeseries.GetSupportedQueryParams()
	sqp.SetFromList([]string{
		"sources",
		"gliderids",
		"parameters",
		"names",
	})
	return sqp
}

// GetStatus ... (see documentation in TimeSeries interface)
func (ts *Glider) GetStatus(queryParams url.Values) (interface{}, error) {
	return nil, fmt.Errorf("GetStatus() not implemented for times series type glider")
}

// OAGetTags ... (see documentation in OAPublisher interface)
func (ts *Glider) OAGetTags() (map[string]openapi.Tag, error) {

	rank := 11
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
func (ts *Glider) OAGetDefs() (map[string]string, error) {
	return nil, nil
}

// getGetPathParameters returns for the /glider/get endpoint the subset of the OpenAPI 'parameters'
// part that is specific to this time series type.
func getGetPathParameters() string {

	return common.NormalizeWhitespace(`[
		{
			"name": "sources",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "The sources to get data for as a comma-separated list of names.
				Only time series with a source that matches at least one of the names will be
				considered. Matching is case-insensitive and a name may contain asterisks for
				wildcard matching.",
			"example": "University of Bergen*"
		},
		{
			"name": "gliderids",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "The glider IDs to get data for as a comma-separated list of names.
				Only time series with a glider ID that matches at least one of the names will be
				considered. Matching is case-insensitive and a name may contain asterisks for
				wildcard matching.",
			"example": "56*,5620625"
		},
		{
			"name": "parameters",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "The parameters to get data for as a comma-separated list of names.
				Only time series with a parameter that matches at least one of the names will be
				considered. Matching is case-insensitive and a name may contain asterisks for
				wildcard matching.",
			"example": "*temp*"
		},
		{
			"name": "names",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "The names to get data for as a comma-separated list of names.
				Only time series with a name that matches at least one of the names will be
				considered. Matching is case-insensitive and a name may contain asterisks for
				wildcard matching.",
			"example": "sg562"
		}
	]`)
}

// OAGetPaths ... (see documentation in OAPublisher interface)
func (ts *Glider) OAGetPaths() ([]openapi.Path, error) {

	var err error

	tagName := fmt.Sprintf("obs/%s", ts.Type())

	tsCreateObj, err := obsopenapi.CreateTsCreateObject(
		ts.Type(), hdrIDSchema(), hdrExtraSchema(), "Create new time series", "obsGliderTsCreate",
		tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateTsCreateObject(%s) failed: %v", ts.Type(), err)
	}

	tsDeleteObj, err := obsopenapi.CreateTsDeleteObject(
		ts.Type(), hdrIDSchema(), hdrExtraSchema(), "Delete time series", "obsGliderTsDelete",
		tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateTsDeleteObject(%s) failed: %v", ts.Type(), err)
	}

	tsUpdateObj, err := obsopenapi.CreateTsUpdateObject(
		ts.Type(), hdrIDSchema(), hdrExtraSchema(), "Update time series (NOT YET IMPLEMENTED)",
		"obsGliderTsUpdate", tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateTsUpdateObject(%s) failed: %v", ts.Type(), err)
	}

	putObj, err := obsopenapi.CreatePutObject(
		ts.Type(), "", `{"error": "no example provided yet"}`, hdrIDSchema(), hdrExtraSchema(),
		obsBodySchema(), "obsGliderPut", tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreatePutObject(%s) failed: %v", ts.Type(), err)
	}

	getObj, err := obsopenapi.CreateGetObject(
		ts.Type(), getGetPathParameters(), hdrIDSchema(), hdrExtraSchema(), obsBodySchema(),
		"obsGliderGet", tagName, openapi.DocLevelBoth())
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateGetObject(%s) failed: %v", ts.Type(), err)
	}

	statusObj, err := obsopenapi.CreateStatusObject(
		ts.Type(), "obsGliderStatus", tagName,
		`<html><span style=\"color:red\">(TO BE DOCUMENTED)</html>`)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateStatusObject(%s) failed: %v", ts.Type(), err)
	}

	baseRouteTS := fmt.Sprintf("%s/%s", ts.BaseRoute, ts.Type())

	return []openapi.Path{
		{
			Name:   fmt.Sprintf("%s/ts/create", baseRouteTS),
			Object: tsCreateObj,
		},
		{
			Name:   fmt.Sprintf("%s/ts/delete", baseRouteTS),
			Object: tsDeleteObj,
		},
		{
			Name:   fmt.Sprintf("%s/ts/update", baseRouteTS),
			Object: tsUpdateObj,
		},
		{
			Name:   fmt.Sprintf("%s/put", baseRouteTS),
			Object: putObj,
		},
		{
			Name:   fmt.Sprintf("%s/get", baseRouteTS),
			Object: getObj,
		},
		{
			Name:   fmt.Sprintf("%s/status", baseRouteTS),
			Object: statusObj,
		},
	}, nil
}

// --- END implementation of TimeSeries interface --------------------------------------
