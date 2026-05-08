// Package obs implements initialization for and requests to time series types under the
// /api/v1/obs path.
package obs

// Initialization code and general definitions for the /api/v1/obs/* routes.

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"gitlab.met.no/frost/frost/internal/common"
	"gitlab.met.no/frost/frost/internal/openapi"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/readrestriction"
	obssbends "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends"
	obssbelocal "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends/local"
	obssbepostgres "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends/postgres"
	timeseries "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
	tsbadevann "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries/badevann"
	tsfrost0 "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries/frost0"
	tsglider "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries/glider"
	tsverticalprofile "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries/verticalprofile"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/tsregistry"
	obswriteops "gitlab.met.no/frost/frost/internal/routes/api/insituobs/writeops"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/writerestriction"
	"gitlab.met.no/frost/frost/pkg/middleware"
)

func baseRoute() string {
	return "/api/v1/obs"
}

// InitTimeSeriesRegistry initializes the global time series registry (used by the obs/*
// routes / obs package) with supported time series types and their currently available instances.
// Returns nil upon success, otherwise error.
func InitTimeSeriesRegistry() error {
	tsregistry.TSReg = make(tsregistry.TimeSeriesRegistry)

	var tsTypeInfo tsregistry.TimeSeriesTypeInfo

	// --- frost0 --------------
	tsTypeInfo = tsregistry.TimeSeriesTypeInfo{
		Default:   &tsfrost0.Frost0{},
		Instances: make(map[string]*timeseries.TimeSeries),
	}
	tsregistry.TSReg[tsTypeInfo.Default.Type()] = tsTypeInfo

	// --- badevann --------------
	tsTypeInfo = tsregistry.TimeSeriesTypeInfo{
		Default:   &tsbadevann.Badevann{},
		Instances: make(map[string]*timeseries.TimeSeries),
	}
	tsregistry.TSReg[tsTypeInfo.Default.Type()] = tsTypeInfo

	// --- glider --------------
	tsTypeInfo = tsregistry.TimeSeriesTypeInfo{
		Default:   &tsglider.Glider{},
		Instances: make(map[string]*timeseries.TimeSeries),
	}
	tsregistry.TSReg[tsTypeInfo.Default.Type()] = tsTypeInfo

	// --- verticalprofile --------------
	tsTypeInfo = tsregistry.TimeSeriesTypeInfo{
		Default:   &tsverticalprofile.VerticalProfile{},
		Instances: make(map[string]*timeseries.TimeSeries),
	}
	tsregistry.TSReg[tsTypeInfo.Default.Type()] = tsTypeInfo

	// more time series types ...

	return nil
}

// InitRestrictions initializes read- and write restriction for time series
// accessible through the obs/* routes.
// Returns nil upon success, otherwise error.
func InitRestrictions() error {
	err := readrestriction.Load()
	if err != nil {
		return fmt.Errorf("readrestriction.Load() failed: %v", err)
	}
	err = writerestriction.Load()
	if err != nil {
		return fmt.Errorf("writerestriction.Load() failed: %v", err)
	}
	return nil
}

// CreateStorageBackend creates a storage backend for the /api/v1/obs/* routes.
func CreateStorageBackend(obeFromEnv string) (obssbends.StorageBackend, error) {

	var sbe obssbends.StorageBackend

	// set up storage backend
	switch obeFromEnv {
	case "local": // a memory structure in this process

		lsb, err := obssbelocal.NewLocal()
		if err != nil {
			return nil, fmt.Errorf("obssbelocal.NewLocal() failed: %v", err)
		}
		sbe = lsb

	case "postgres": // a postgres server

		psb, err := obssbepostgres.NewPostgres(
			common.Getenv("PSBHOST", "localhost"),
			common.Getenv("PSBPORT", "5433"),
			common.Getenv("PSBUSER", "postgres"),
			common.Getenv("PSBPASSWORD", ""))
		if err != nil {
			return nil, fmt.Errorf("obssbepostgres.NewPostgres() failed: %v", err)
		}
		sbe = psb

	default:
		return nil, fmt.Errorf(
			"invalid storage backend: %s, expected one of local, postgres, or lard",
			obeFromEnv)
	}

	return sbe, nil
}

type timeSeriesInfo struct {
	defaultTS         *timeseries.TimeSeries
	storageBackend    obssbends.StorageBackend
	responseItemLimit int
}

func newTimeSeriesInfo(
	ts timeseries.TimeSeries, storageBackend obssbends.StorageBackend, responseItemLimit int) (
	*timeSeriesInfo, error) {

	var defaultTS *timeseries.TimeSeries
	if ts != nil {
		dts, err := tsregistry.DefaultTimeSeries(ts.Type())
		if err != nil {
			return nil, fmt.Errorf("tsregistry.DefaultTimeSeries(%s) failed: %v", ts.Type(), err)
		}
		defaultTS = &dts
	}

	return &timeSeriesInfo{
		defaultTS:         defaultTS,
		storageBackend:    storageBackend,
		responseItemLimit: responseItemLimit,
	}, nil
}

// registerRoutesForTimeSeries registers routes under /api/v1/obs/* for a specific time series type.
//
// Returns nil upon success, otherwise error.
func registerRoutesForTimeSeries(
	tstype, baseRouteTS string, tsInfo *timeSeriesInfo, externalRouter *mux.Router,
	metrics *middleware.Metrics) error {

	// PATH: tstype/ts/create
	h := tsInfo.handleTsCreate
	externalRouter.HandleFunc(
		fmt.Sprintf("%s/ts/create", baseRouteTS),
		metrics.Endpoint(fmt.Sprintf("/v1/insituobs/%s/ts/create", tstype), h))

	// PATH: tstype/ts/delete
	h = tsInfo.handleTsDelete
	externalRouter.HandleFunc(
		fmt.Sprintf("%s/ts/delete", baseRouteTS),
		metrics.Endpoint(fmt.Sprintf("/v1/insituobs/%s/ts/delete", tstype), h))

	// PATH: tstype/ts/update
	h = tsInfo.handleTsUpdate
	externalRouter.HandleFunc(
		fmt.Sprintf("%s/ts/update", baseRouteTS),
		metrics.Endpoint(fmt.Sprintf("/v1/insituobs/%s/ts/update", tstype), h))

	// PATH: tstype/put
	h = tsInfo.handlePut
	externalRouter.HandleFunc(
		fmt.Sprintf("%s/put", baseRouteTS),
		metrics.Endpoint(fmt.Sprintf("/v1/insituobs/%s/put", tstype), h))

	// PATH: tstype/get
	h = tsInfo.handleGet
	externalRouter.HandleFunc(
		fmt.Sprintf("%s/get", baseRouteTS), // /api/v1/obs/frost0/get
		metrics.Endpoint(
			fmt.Sprintf("/v1/insituobs/%s/get", tstype), h), // /v1/insituobs/frost0/get
	)

	// PATH: tstype/status
	h = tsInfo.handleStatus
	externalRouter.HandleFunc(
		fmt.Sprintf("%s/status", baseRouteTS),
		metrics.Endpoint(fmt.Sprintf("/v1/insituobs/%s/status", tstype), h))

	return nil
}

// RegRoutesAndOAPubs registers the following for functionality under /api/v1/obs/*:
//   - endpoint handlers (to externalRouter and metrics)
//   - OpenAPI docs (to pubs).
//
// The in/out-parameter features contains features to try to activate (NOTE: additional
// features may be added, e.g. to support other features).
// The in/out-parameter availableFeatures will contain all possible features.
// The in/out-parameter activeFeatures will contain the intersection of features and
// availableFeatures.
//
// Returns nil upon success, otherwise error.
func RegRoutesAndOAPubs(
	pubs *openapi.OAPublishers, externalRouter *mux.Router, metrics *middleware.Metrics,
	sbe obssbends.StorageBackend, responseItemLimit int, staticFilesDir string, features,
	activeFeatures, availableFeatures *common.StringSet) error {

	infoCommon, err := newTimeSeriesInfo(nil, sbe, responseItemLimit)
	if err != nil {
		return fmt.Errorf("newTimeSeriesInfo(...nil...) failed: %v", err)
	}

	// define an endpoint for clearing the entire contents of the storage backend and
	// internal registers
	// NOTE: This is currently not wrapped with auth...
	externalRouter.HandleFunc(
		"/api/v1/obs/clear", metrics.Endpoint("/v1/obs/clear", infoCommon.handleClear))
	// TODO: add OpenAPI documentation (but this should perhaps remain an undocumented
	// feature that is used for testing only?)

	pubs.AddPublisher(infoCommon) // register OpenAPI publisher

	registerForTSType := func(ts timeseries.TimeSeries) error {

		if ts == nil {
			return fmt.Errorf("ts is nil")
		}

		tstype := ts.Type()
		baseRouteTS := fmt.Sprintf("%s/%s", baseRoute(), tstype)
		availableFeatures.Set(baseRouteTS)

		if features.ContainsMatch(baseRouteTS) {
			tsInfo, err := newTimeSeriesInfo(ts, sbe, responseItemLimit)
			if err != nil {
				return fmt.Errorf("newTimeSeriesInfo(...%s...) failed: %v", tstype, err)
			}

			err = registerRoutesForTimeSeries(
				tstype, baseRouteTS, tsInfo, externalRouter, metrics)
			if err != nil {
				return fmt.Errorf(
					"registerRoutesForTimeSeries(%s, ...) failed: %v", baseRouteTS, err)
			}

			pubs.AddPublisher(ts) // register OpenAPI publisher

			activeFeatures.Set(baseRouteTS)
			tsregistry.Enable(tstype)
		}

		return nil
	}

	for _, ts := range []timeseries.TimeSeries{
		&tsfrost0.Frost0{BaseRoute: baseRoute()},
		&tsbadevann.Badevann{BaseRoute: baseRoute()},
		&tsglider.Glider{BaseRoute: baseRoute()},
		&tsverticalprofile.VerticalProfile{BaseRoute: baseRoute()},
		// add more time series types here ...
	} {
		err := registerForTSType(ts)
		if err != nil {
			return fmt.Errorf("registerForTSType() failed: %v", err)
		}
	}

	return nil
}

// common handlers (independent of time series type):

func (tsInfo *timeSeriesInfo) handleClear(
	responseWriter http.ResponseWriter, request *http.Request) {
	HandleClear(responseWriter, request, tsInfo.storageBackend)
}

func (tsInfo *timeSeriesInfo) handleTsCreate(
	responseWriter http.ResponseWriter, request *http.Request) {
	obswriteops.HandleTsCreate(*tsInfo.defaultTS, responseWriter, request, tsInfo.storageBackend)
}

func (tsInfo *timeSeriesInfo) handleTsDelete(
	responseWriter http.ResponseWriter, request *http.Request) {
	obswriteops.HandleTsDelete(*tsInfo.defaultTS, responseWriter, request, tsInfo.storageBackend)
}

func (tsInfo *timeSeriesInfo) handleTsUpdate(
	responseWriter http.ResponseWriter, request *http.Request) {
	obswriteops.HandleTsUpdate(*tsInfo.defaultTS, responseWriter, request, tsInfo.storageBackend)
}

func (tsInfo *timeSeriesInfo) handlePut(
	responseWriter http.ResponseWriter, request *http.Request) {
	obswriteops.HandlePut(*tsInfo.defaultTS, responseWriter, request, tsInfo.storageBackend)
}

func (tsInfo *timeSeriesInfo) handleGet(
	responseWriter http.ResponseWriter, request *http.Request) {
	HandleGet(
		*tsInfo.defaultTS, responseWriter, request, tsInfo.storageBackend,
		tsInfo.responseItemLimit)
}

func (tsInfo *timeSeriesInfo) handleStatus(
	responseWriter http.ResponseWriter, request *http.Request) {
	HandleStatus(*tsInfo.defaultTS, responseWriter, request, tsInfo.storageBackend)
}

// OAGetTags ... (see documentation in OAPublisher interface)
func (tsInfo *timeSeriesInfo) OAGetTags() (map[string]openapi.Tag, error) {
	return map[string]openapi.Tag{}, nil // n/a
}

// OAGetDefs ... (see documentation in OAPublisher interface)
func (tsInfo *timeSeriesInfo) OAGetDefs() (map[string]string, error) {
	return nil, nil
}

// OAGetPaths ... (see documentation in OAPublisher interface)
func (tsInfo *timeSeriesInfo) OAGetPaths() ([]openapi.Path, error) {
	return []openapi.Path{}, nil // n/a
}
