// Package endpointmanager implements an endpoint manager.
package endpointmanager

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"gitlab.met.no/frost/frost/internal/common"
	localhttp "gitlab.met.no/frost/frost/internal/http"
	"gitlab.met.no/frost/frost/internal/openapi"
	"gitlab.met.no/frost/frost/pkg/middleware"
)

type QueryParamInfo struct {
	Name        string
	Description string
	Example     string
	Required    bool
}

type QueryParamInfos []QueryParamInfo

// containsName returns true iff qpInfos contains an item whose Name field equals name.
func (qpInfos *QueryParamInfos) containsName(name string) bool {
	for _, qpInfo := range *qpInfos {
		if qpInfo.Name == name {
			return true
		}
	}
	return false
}

type ResponseInfo struct {
	Description string
	Schema      string
}

type ResponseInfos map[int]ResponseInfo // map from HTTP status code to ResponseInfo

// namesToCSV returns the Name fields of qpInfos as an alphabetically sorted comma-separated string.
func (qpInfos *QueryParamInfos) namesToCSV() string {
	names := []string{}
	for _, qpInfo := range *qpInfos {
		names = append(names, qpInfo.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ExtractQueryParameters extracts query parameters from request into a map according to qpInfos.
//
// Return (query params map, nil) on success, otherwise (..., error).
func ExtractQueryParameters(
	request *http.Request, qpInfos *QueryParamInfos) (map[string]string, error) {

	// extract candidate params from request into qp0
	qp0, err := localhttp.ExtractQueryParameters(request)
	if err != nil {
		return nil, fmt.Errorf("localhttp.ExtractQueryParameters() failed: %v", err)
	}

	// ensure that each param in qp0 is defined at most once
	qp := map[string]string{}
	for qpName, qpVals := range qp0 {
		if len(qpVals) != 1 {
			return nil, fmt.Errorf("query parameter '%s' is defined multiple times", qpName)
		}
		qp[qpName] = qpVals[0]
	}

	// ensure that qp doesn't contain params that are not defined in qpInfos
	for qpName := range qp {
		if !qpInfos.containsName(qpName) {
			return nil, fmt.Errorf(
				"query parameter '%s' is not among the supported ones: %s",
				qpName, qpInfos.namesToCSV())
		}
	}

	// ensure that qp contains all required params in qpInfos (qpInfo.Required == true)
	// return qp
	for _, qpInfo := range *qpInfos {
		if qpInfo.Required {
			if _, found := qp[qpInfo.Name]; !found {
				return nil,
					fmt.Errorf("required query parameter '%s' is not specified", qpInfo.Name)
			}
		}
	}

	return qp, nil
}

type EndpointInfo struct {
	Path string // The endpoint's URL path. For example: /api/v1/idf/{station|grid}[/available],
	// all four of which are associated with feature 'idf'.
	DocLevel     string
	HandlerFunc  http.HandlerFunc
	Summary      string
	Description  string
	OperationID  string
	Tags         []string
	QueryParams  *QueryParamInfos
	Responses    ResponseInfos
	OADefs       map[string]string
}

// EndpointManager defines information and structures needed by an endpoint manager.
type EndpointManager struct {
	router            *mux.Router
	metrics           *middleware.Metrics
	requestedFeatures *common.StringSet // requested features
	availableFeatures *common.StringSet // eventually all supported features
	activeFeatures    *common.StringSet // eventually the intersection of requestedFeatures and
	// availableFeatures
	epInfos map[string]EndpointInfo // path to EndpointInfo registry
	oaTags  map[string]openapi.Tag  // tag name to Tag registry
}

// NewEndpointManager creates and returns a new endpoint manager.
func NewEndpointManager(
	oaPubs *openapi.OAPublishers, router *mux.Router, metrics *middleware.Metrics,
	requestedFeatures, availableFeatures, activeFeatures *common.StringSet) *EndpointManager {

	epMgr := EndpointManager{
		router:            router,
		metrics:           metrics,
		requestedFeatures: requestedFeatures,
		availableFeatures: availableFeatures,
		activeFeatures:    activeFeatures,
		epInfos:           map[string]EndpointInfo{},
		oaTags:            map[string]openapi.Tag{},
	}

	oaPubs.AddPublisher(&epMgr) // register OpenAPI publisher

	return &epMgr
}

// regHandlerFunc registers the handler function for an endpint.
func (epMgr *EndpointManager) regHandlerFunc(epInfo EndpointInfo) {

	// define final handler function
	handler := epInfo.HandlerFunc

	// bind path to handler function
	epMgr.router.HandleFunc(epInfo.Path, epMgr.metrics.Endpoint(epInfo.Path, handler))
}

// addToRegistry adds an endpoint to the registry.
//
// Returns nil on success, otherwise error.
func (epMgr *EndpointManager) addToRegistry(epInfo EndpointInfo) error {

	_, found := epMgr.epInfos[epInfo.Path]
	if found {
		return fmt.Errorf("endpoint already registered: >%s<", epInfo.Path)
	}
	epMgr.epInfos[epInfo.Path] = epInfo
	return nil
}

// Feature represents a feature with a unique name and a function for registering its endpoints.
type Feature interface {
	Name() string
	RegEndpoints(*EndpointManager) error
}

// RegFeature registers a feature if requested.
//
// Returns (<whether feature was registered>, nil) on success, otherwise (..., error).
func (epMgr *EndpointManager) RegFeature(feature Feature) (bool, error) {

	epMgr.availableFeatures.Set(feature.Name()) // add to available features
	if !epMgr.requestedFeatures.ContainsMatch(feature.Name()) {
		return false, nil // feature is not requested, so indicate that we bailed out
	}

	epMgr.activeFeatures.Set(feature.Name()) // add to active features

	// register the feature's endpoints
	err := feature.RegEndpoints(epMgr)
	if err != nil {
		return false, fmt.Errorf("RegEndpoints() failed for feature >%s<: %v", feature.Name(), err)
	}

	return true, nil // feature was successfully registered
}

// RegEndpoint registers an endpoint.
//
// Returns nil on success, otherwise error.
func (epMgr *EndpointManager) RegEndpoint(epInfo EndpointInfo) error {

	// allow an empty list to be specified as either nil or &QueryParamInfos{}
	if epInfo.QueryParams == nil {
		epInfo.QueryParams = &QueryParamInfos{}
	}

	// add to registry
	if err := epMgr.addToRegistry(epInfo); err != nil {
		return fmt.Errorf("epMgr.addToRegistry() failed: %v", err)
	}

	// register handler function
	epMgr.regHandlerFunc(epInfo)

	return nil // indicate that endpoint was registered
}

// RegOATag registers an OpenAPI tag.
//
// Returns nil on success, otherwise error.
func (epMgr *EndpointManager) RegOATag(name string, tag openapi.Tag) error {

	for name0, tag0 := range epMgr.oaTags {
		if name0 == name {
			return fmt.Errorf("tag name already registered: >%s<", name)
		}
		if (tag0.Rank != nil) && (tag.Rank != nil) && (*tag0.Rank == *tag.Rank) {
			return fmt.Errorf(
				"the rank %d for tag >%s< is already registered for tag >%s<",
				*tag.Rank, name, name0)
		}
	}

	epMgr.oaTags[name] = tag
	return nil

}

// OAGetDefs ... (see documentation in OAPublisher interface)
func (epMgr *EndpointManager) OAGetDefs() (map[string]string, error) {

	defs := map[string]string{}

	for epPath, epInfo := range epMgr.epInfos {

		for defName, def := range epInfo.OADefs {
			_, found := defs[defName]
			if found {
				return nil, fmt.Errorf("defName >%s< already defined (epPath: %s)", defName, epPath)
			}
			defs[defName] = def
		}
	}

	return defs, nil
}

// OAGetPaths ... (see documentation in OAPublisher interface)
func (epMgr *EndpointManager) OAGetPaths() ([]openapi.Path, error) {

	oaPaths := []openapi.Path{}

	for epPath, epInfo := range epMgr.epInfos {

		params := openapi.OAParameters{}
		for _, qp := range *epInfo.QueryParams {
			params = append(params, openapi.OAParameter{
				Name:        qp.Name,
				Description: common.NormalizeWhitespace(qp.Description),
				Example:     common.NormalizeWhitespace(qp.Example),
				In:          "query",
				Required:    qp.Required,
				Schema:      map[string]string{"type": "string"},
			})
		}

		responses := openapi.OAResponses{}
		for statusCode, respInfo := range epInfo.Responses {
			key := fmt.Sprintf("%d", statusCode)

			schema := map[string]any{}
			err := json.Unmarshal([]byte(common.NormalizeWhitespace(respInfo.Schema)), &schema)
			if err != nil {
				return nil, fmt.Errorf("json.Unmarshal() failed: %v", err)
			}

			responses[key] = openapi.OAResponse{
				Description: common.NormalizeWhitespace(respInfo.Description),
				Content: map[string]any{
					"application/json": map[string]any{
						"schema": schema,
					},
				},
			}
		}

		oaGet := openapi.OAGet{
			Summary:     common.NormalizeWhitespace(epInfo.Summary),
			Description: common.NormalizeWhitespace(epInfo.Description),
			OperationID: epInfo.OperationID,
			Tags:        epInfo.Tags,
			Parameters:  params,
			Responses:   responses,
		}

		oaPath := openapi.Path{
			Name: epPath,
			Object: map[string]any{
				"doclevel": epInfo.DocLevel,
				"get":      oaGet,
			},
		}

		oaPaths = append(oaPaths, oaPath)
	}

	return oaPaths, nil
}

// OAGetTags ... (see documentation in OAPublisher interface)
func (epMgr *EndpointManager) OAGetTags() (map[string]openapi.Tag, error) {

	oaTags := map[string]openapi.Tag{}

	for epPath, epInfo := range epMgr.epInfos {
		for _, tagName := range epInfo.Tags {
			tag, found := epMgr.oaTags[tagName]
			if !found {
				return nil, fmt.Errorf("tag not registered for endpoint %s: >%s<", epPath, tagName)
			}
			oaTags[tagName] = tag
		}
	}

	return oaTags, nil
}
