// Package frost0 implements the frost0 time series type.
package frost0

// This file contains code specific to time series type 'frost0'.
// In particular, all timeseries.TimeSeries instances referred to in this file are
// of that time series type.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
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

// Frost0 implements the TimeSeries interface.
type Frost0 struct {
	BaseRoute           string
	timeseries.BaseID   // see https://golang.org/doc/effective_go#embedding
	timeseries.Header   // ditto
	timeseries.FromTime // ditto
	timeseries.ToTime   // ditto
}

type hdrID struct { // hdr/id part - must correspond with hdrIDSchema
	Source      string `json:"source"`
	Sensorlevel string `json:"sensorlevel"`
	Element     string `json:"element"`
}

type organization struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

type pos struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

type hdrExtra struct { // hdr/extra part - must correspond with hdrExtraSchema
	Organizations []organization `json:"organizations"`
	Pos           pos            `json:"pos"`
}

type hdrIDRegVal struct { // value part of hdrIDReg
	ts    *timeseries.TimeSeries // pointer to time series instance in global registry
	extra hdrExtra               // hdr/extra part
}

var (
	hdrIDReg map[hdrID]hdrIDRegVal // registry of all time series header IDs of type frost0

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
		    "sensorlevel": {
				"type": "string",
				"example": "10",
				"description": "Sensor level in meters above ground."
			},
		    "element": {
				"type": "string"
			}
		},
		"required": ["source", "sensorlevel", "element"],
		"additionalProperties": false
	}`
}

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
		            "organizations": {
				        "type": "array",
				        "items": {
					        "type": "object",
							"properties": {
								"name": {
									"type": "string"
								},
								"from": {
									"type": "string"
								},
								"to": {
									"type": "string"
								}
							},
							"required": ["name", "from", "to"],
							"additionalProperties": false
				        }
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
				"additionalProperties": false
			}
		]
	}`
}

func obsBodySchema() string {
	return `{
		"title": "observations_body",
		"type": "object",
		"properties": {
			"pos": {
				"oneOf": [
				    {
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
					},
					{
						"type": "null"
					}
			    ]
		    },
			"value": {
				"type": "string",
				"description": "Generic observation value.",
				"example": "-12.7"
			},
			"quality": {
				"type": "string",
				"description": "Generic observation quality.",
				"example": "3"
			}
		},
		"required": ["value"],
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
func (ts *Frost0) Clear() {
	hdrIDReg = map[hdrID]hdrIDRegVal{}
}

// Type ... (see documentation in TimeSeries interface)
func (ts *Frost0) Type() string {
	return "frost0"
}

// Description ... (see documentation in TimeSeries interface)
func (ts *Frost0) Description() string {
	return "<description of the frost0 time series type ...>"
}

// CreateInstance ... (see documentation in TimeSeries interface)
func (ts *Frost0) CreateInstance(
	baseID timeseries.BaseID, hdr timeseries.Header, id, extra string,
	fromTime timeseries.FromTime, toTime timeseries.ToTime) (*timeseries.TimeSeries, error) {
	var ts0 timeseries.TimeSeries = &Frost0{
		BaseID: baseID, Header: hdr, FromTime: fromTime, ToTime: toTime}
	return &ts0, nil
}

// FinalizeInstance ... (see documentation in TimeSeries interface)
func (ts *Frost0) FinalizeInstance(
	tsNew *timeseries.TimeSeries, baseID timeseries.BaseID, hdr timeseries.Header,
	id, extra string, fromTime timeseries.FromTime, toTime timeseries.ToTime) (error, error) {
	return addToReg(tsNew, id, extra), nil
}

// GetHeader ... (see documentation in TimeSeries interface)
func (ts *Frost0) GetHeader() (*timeseries.Header, error) {
	return &ts.Header, nil
}

// GetHeaderID ... (see documentation in TimeSeries interface)
func (ts *Frost0) GetHeaderID() (map[string]interface{}, error) {
	return ts.Header["id"], nil
}

// UpdateExtra ... (see documentation in TimeSeries interface)
func (ts *Frost0) UpdateExtra(stsextra string) error {
	return fmt.Errorf("UpdateExtra() not implemented for time series type frost0")
}

// UnlimitedResponse ... (see documentation in TimeSeries interface)
func (ts *Frost0) UnlimitedResponse(
	tsSeq *timeseries.InstanceSeq, tspec timespecification.TimeSpecification) (
	bool, string, int, error) {
	return false, "", -1, nil
}

// GetInstances ... (see documentation in TimeSeries interface)
func (ts *Frost0) GetInstances(
	queryParams url.Values, roles []string, tsSeq *timeseries.InstanceSeq) (int, error) {

	_ = roles // n/a

	// extract query params (TODO: consider all instances (Get considers only the first one))
	sources := common.ExtractCSVValsLC(queryParams.Get("sources"))
	sensorlevels := common.ExtractCSVValsLC(queryParams.Get("sensorlevels"))
	elements := common.ExtractCSVValsLC(queryParams.Get("elements"))

	// add matching time series to result
	for id, regVal := range hdrIDReg {
		switch {
		case !common.StringList(sources).ContainsMatch(strings.ToLower(id.Source), true):
			continue
		case !common.StringList(sensorlevels).ContainsMatch(strings.ToLower(id.Sensorlevel), true):
			continue
		case !common.StringList(elements).ContainsMatch(strings.ToLower(id.Element), true):
			continue
		}
		*tsSeq = append(*tsSeq, regVal.ts) // no mismatches found
	}

	return -1, nil // no errors found
}

// FinalizeInstanceOrder ... (see documentation in TimeSeries interface)
func (ts *Frost0) FinalizeInstanceOrder(tsSeq *timeseries.InstanceSeq) (int, error) {
	_ = tsSeq // n/a
	return -1, nil
}

// FindInstanceFromID ... (see documentation in TimeSeries interface)
func (ts *Frost0) FindInstanceFromID(sid []byte) (*timeseries.TimeSeries, error) {
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
func (ts *Frost0) HeaderFilterSpecial(
	reqInfo timeseries.RequestInfo, tsSeq *timeseries.InstanceSeq) (int, error) {
	// TODO ...
	return -1, nil
}

// HeaderPxmtyFilter ... (see documentation in TimeSeries interface)
func (ts *Frost0) HeaderPxmtyFilter(
	reqInfo timeseries.RequestInfo, tsSeq *timeseries.InstanceSeq) (int, error) {
	return -1, nil // not yet implemented for this time series type, so leave tsSeq unmodified
}

// ObsBodyModify ... (see documentation in TimeSeries interface)
func (ts *Frost0) ObsBodyModify(t time.Time, body *map[string]interface{}) (int, error) {
	return -1, nil // for now don't modify any obs
}

// ObsFilter ... (see documentation in TimeSeries interface)
func (ts *Frost0) ObsFilter(
	t time.Time, body map[string]interface{}, reqInfo timeseries.RequestInfo) (bool, int, error) {
	return true, -1, nil // for now don't filter out any obs
}

// ValidateHdrID ... (see documentation in TimeSeries interface)
func (ts *Frost0) ValidateHdrID(hdrID interface{}) error {
	return common.SchemaValidate(schemaLoaders.HdrID, hdrID)
}

// ValidateHdrExtra ... (see documentation in TimeSeries interface)
func (ts *Frost0) ValidateHdrExtra(hdrExtra interface{}) error {
	return common.SchemaValidate(schemaLoaders.HdrExtra, hdrExtra)
}

// ValidateObsBody ... (see documentation in TimeSeries interface)
func (ts *Frost0) ValidateObsBody(obsBody interface{}) error {
	return common.SchemaValidate(schemaLoaders.ObsBody, obsBody)
}

// IngestHook ... (see documentation in TimeSeries interface)
func (ts *Frost0) IngestHook(dts dataset.SingleTSeries, sbe interface{}) ([]error, []error) {
	return nil, nil // no actions
}

// HeaderIDsEqual ... (see documentation in TimeSeries interface)
func (ts *Frost0) HeaderIDsEqual(hdr1, hdr2 map[string]interface{}) (bool, error) {
	for _, key := range []string{"source", "sensorlevel", "element"} {
		if !common.StringsEqual(hdr1, hdr2, key) {
			return false, nil
		}
	}
	return true, nil // no differences found
}

// CreateCustomReqInfo ... (see documentation in TimeSeries interface)
func (ts *Frost0) CreateCustomReqInfo(queryParams url.Values) (interface{}, error) {
	return nil, nil // TODO: implement for geo point filter
}

// GetHeaderGeoPoints ... (see documentation in TimeSeries interface)
func (ts *Frost0) GetHeaderGeoPoints() ([]timeseries.PointInterval, error) {
	return []timeseries.PointInterval{}, nil // TODO: implement for geo point filter
}

// GetObsBodyGeoPoint ... (see documentation in TimeSeries interface)
func (ts *Frost0) GetObsBodyGeoPoint(obsBody map[string]interface{}) (
	geometry.Point, bool, error) {
	return geometry.Point{}, false, nil // TODO: implement for geo point filter
}

// GetSupportedQueryParams ... (see documentation in TimeSeries interface)
func (ts *Frost0) GetSupportedQueryParams() common.StringSet {
	sqp := timeseries.GetSupportedQueryParams()
	sqp.SetFromList([]string{
		"sources",
		"sensorlevels",
		"elements",
	})
	return sqp
}

// GetStatus ... (see documentation in TimeSeries interface)
func (ts *Frost0) GetStatus(queryParams url.Values) (interface{}, error) {
	return nil, fmt.Errorf("GetStatus() not implemented for times series type frost0")
}

// OAGetTags ... (see documentation in OAPublisher interface)
func (ts *Frost0) OAGetTags() (map[string]openapi.Tag, error) {

	rank := 15
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
func (ts *Frost0) OAGetDefs() (map[string]string, error) {
	return nil, nil
}

// getGetPathParameters returns for the /frost0/get endpoint the subset of the OpenAPI 'parameters'
// part that is specific to this time series type.
func getGetPathParameters() string {

	return common.NormalizeWhitespace(`[
		{
			"name": "sources",
			"required": false,
			"in": "query",
			"docgroup": "where",
			"schema": {
				"type": "string"
			},
			"description": "The sources to get data for as a comma-separated list of names.
				Only time series with a source that matches at least one of the names will be
				considered. Matching is case-insensitive and a name may contain asterisks for
				wildcard matching.",
			"example": "SN18*,NA937"
		},
		{
			"name": "sensorlevels",
			"doclevel": "advancedonly",
			"docgroup": "where",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "The sensor levels to get data for as a comma-separated list of names.
				Only time series with a sensor level that matches at least one of the names will be
				considered. Matching is case-insensitive and a name may contain asterisks for
				wildcard matching.",
			"example": "*1*,2"
		},
		{
			"name": "elements",
			"required": false,
			"docgroup": "what",
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "The elements to get data for as a comma-separated list of names.
				Only time series with an element that matches at least one of the names will be
				considered. Matching is case-insensitive and a name may contain asterisks for wildcard
				matching.",
			"example": "*wind*,air_temperature"
		}
	]`)
}

// OAGetPaths ... (see documentation in OAPublisher interface)
func (ts *Frost0) OAGetPaths() ([]openapi.Path, error) {

	var err error

	tagName := fmt.Sprintf("obs/%s", ts.Type())

	tsCreateObj, err := obsopenapi.CreateTsCreateObject(
		ts.Type(), hdrIDSchema(), hdrExtraSchema(), "Create new time series", "obsFrost0TsCreate",
		tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateTsCreateObject(%s) failed: %v", ts.Type(), err)
	}

	tsDeleteObj, err := obsopenapi.CreateTsDeleteObject(
		ts.Type(), hdrIDSchema(), hdrExtraSchema(), "Delete time series", "obsFrost0TsDelete",
		tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateTsDeleteObject(%s) failed: %v", ts.Type(), err)
	}

	tsUpdateObj, err := obsopenapi.CreateTsUpdateObject(
		ts.Type(), hdrIDSchema(), hdrExtraSchema(), "Update time series (NOT YET IMPLEMENTED)",
		"obsFrost0TsUpdate", tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateTsUpdateObject(%s) failed: %v", ts.Type(), err)
	}

	putObj, err := obsopenapi.CreatePutObject(
		ts.Type(), "", `{"error": "no example provided yet"}`, hdrIDSchema(), hdrExtraSchema(),
		obsBodySchema(), "obsFrost0Put", tagName)
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreatePutObject(%s) failed: %v", ts.Type(), err)
	}

	getObj, err := obsopenapi.CreateGetObject(
		ts.Type(), getGetPathParameters(), hdrIDSchema(), hdrExtraSchema(), obsBodySchema(),
		"obsFrost0Get", tagName, openapi.DocLevelBoth())
	if err != nil {
		return nil, fmt.Errorf("obsopenapi.CreateGetObject(%s) failed: %v", ts.Type(), err)
	}

	statusObj, err := obsopenapi.CreateStatusObject(
		ts.Type(), "obsFrost0Status", tagName,
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
