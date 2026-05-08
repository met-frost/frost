package frost

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/mux"
	"gitlab.met.no/frost/frost/internal/common"
	epmgr "gitlab.met.no/frost/frost/internal/endpointmanager"
	localhttp "gitlab.met.no/frost/frost/internal/http"
	"gitlab.met.no/frost/frost/internal/openapi"
	obs "gitlab.met.no/frost/frost/internal/routes/api/insituobs"
	obssbends "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends"
	"gitlab.met.no/frost/frost/pkg/metaservice"
	"gitlab.met.no/frost/frost/pkg/middleware"
)

// Service ... (TODO: fill in documentation)
type Service struct {
	about             *metaservice.About
	htmlTemplates     *template.Template
	staticFilesDir    string
	InternalRouter    *mux.Router
	ExternalRouter    *mux.Router
	metrics           *middleware.Metrics
	obsStorageBackend obssbends.StorageBackend
	responseItemLimit int
}

// TODO: rename to just regAPI() since we don't want to demonstrate an EDR API for the software paper!
// regNonEDRAPI registers the non-EDR part of the API.
//
// Returns nil upon success, otherwise error.
func (s *Service) regNonEDRAPI(
	features, availableFeatures, activeFeatures *common.StringSet) error {

	oaPubs := openapi.OAPublishers{} // non-EDR publishers
	oaPubs.AddPublisher(s)

	// register routes and more OpenAPI publishers for the non-EDR part of the API
	err := s.regRoutesAndOAPubs(&oaPubs, features, availableFeatures, activeFeatures)
	if err != nil {
		return fmt.Errorf("s.regRoutesAndOAPubs() failed: %v", err)
	}

	// create OAS files from registered non-EDR publishers
	fnameTemplate := fmt.Sprintf("%s/swaggerui/openapi%%s.json", s.staticFilesDir)
	err = oaPubs.CreateFiles(
		fmt.Sprintf(fnameTemplate, "basic"), fmt.Sprintf(fnameTemplate, "advanced"))
	if err != nil {
		return fmt.Errorf("pubs.CreateFiles() failed: %v", err)
	}

	return nil
}

// NewService creates a new service. TODO: complete documentation.
//
// Returns (new service, nil) upon success, otherwise (nil, error).
func NewService(
	staticFilesDir string, responseItemLimit int, respWriteTimeout time.Duration,
	obsSBE obssbends.StorageBackend, features, availableFeatures,
	activeFeatures *common.StringSet) (*Service, error) {

	var err error

	htmlTemplates, err := template.ParseGlob("templates/*")
	if err != nil {
		return nil, fmt.Errorf("template.ParseGlob() failed: %v", err)
	}

	intRouter := mux.NewRouter()
	extRouter := mux.NewRouter()
	metrics := middleware.NewServiceMetrics(middleware.MetricsOpts{
		Name:            "frost",
		Description:     "Frost REST service at MET Norway.",
		ResponseBuckets: []float64{0.001, 0.002, 0.1, 0.5},
	})

	s := Service{
		about:             aboutFrost(),
		htmlTemplates:     htmlTemplates,
		staticFilesDir:    staticFilesDir,
		InternalRouter:    intRouter,
		ExternalRouter:    extRouter,
		metrics:           metrics,
		obsStorageBackend: obsSBE,
		responseItemLimit: responseItemLimit,
	}

	// TODO: rename to regAPI() now that we don't have EDR
	err = s.regNonEDRAPI(features, availableFeatures, activeFeatures)
	if err != nil {
		return nil, fmt.Errorf("s.regNonEDRAPI() failed: %v", err)
	}

	return &s, nil
}

// OAGetTags ... (see documentation in OAPublisher interface)
func (s *Service) OAGetTags() (map[string]openapi.Tag, error) {

	rank := 16
	return map[string]openapi.Tag{
		"overall": {
			Rank:        &rank,
			Description: "",
			DocLevel:    openapi.DocLevelAdvancedOnly(),
		},
	}, nil
}

// OAGetDefs ... (see documentation in OAPublisher interface)
func (s *Service) OAGetDefs() (map[string]string, error) {
	return nil, nil
}

// getHealthzPath gets the path info for the /healthz endpoint.
//
// Returns (path info, nil) upon success, otherwise (..., error).
func getHealthzPath() (openapi.Path, error) {

	// --- BEGIN define 'responses' ------------------------
	schema := `{
        "title": "Service health report.",
        "type": "object",
        "properties": {
            "description": {
                "type": "string",
                "example": "Checked stock of greetings and we are good."
            },
            "errors": {
                "type": "array",
                "items": {
                    "type": "string"
                }
            },
            "status": {
                "type": "string",
                "enum": [
                    "healthy",
                    "unhealthy",
                    "critical"
                ],
                "example": "healthy"
            }
        },
        "required": [
            "status"
        ]
    }
	`

	responsesS := fmt.Sprintf(`{
		"200": {
			"description": "The service is ok.",
			"content": {
				"application/json": {
					"schema": %s
				}
			}
		}
	}`, schema)

	var responses interface{}
	err := json.Unmarshal([]byte(responsesS), &responses)
	if err != nil {
		return openapi.Path{}, fmt.Errorf("json.Unmarshal(responses) failed: %v", err)
	}
	// --- END define 'responses' ------------------------

	// --- BEGIN create toplevel object ------------------------
	obj0 := map[string]interface{}{
		"doclevel": openapi.DocLevelAdvancedOnly(),
		"get": map[string]interface{}{
			"operationId": "healthz",
			"summary":     "Health status of the Frost service.",
			"tags":        []string{"overall"},
			"responses":   responses,
		},
	}
	// --- END create toplevel object ------------------------

	return openapi.Path{
		Name:   "/api/v1/healthz",
		Object: obj0,
	}, nil
}

// getAboutPath gets the path info for the /about endpoint.
//
// Returns (path info, nil) upon success, otherwise (..., error).
func getAboutPath() (openapi.Path, error) {

	// --- BEGIN define 'responses' ------------------------
	schema := `{
        "title": "Overall information about the service.",
        "type": "object",
        "properties": {
            "description": {
                "type": "string"
            },
            "name": {
                "type": "string"
            },
			"provider": {
				"type": "object",
				"properties": {
					"@type": {
						"type": "string"
					},
					"name": {
						"type": "string"
					}
				}
			},
            "termsOfService": {
                "type": "string"
            }
        }
    }
	`

	responsesS := fmt.Sprintf(`{
		"200": {
			"description": "Overall information about the service was successfully downloaded.",
			"content": {
				"application/json": {
					"schema": %s
				}
			}
		}
	}`, schema)

	var responses interface{}
	err := json.Unmarshal([]byte(responsesS), &responses)
	if err != nil {
		return openapi.Path{}, fmt.Errorf("json.Unmarshal(responses) failed: %v", err)
	}
	// --- END define 'responses' ------------------------

	// --- BEGIN create toplevel object ------------------------
	obj0 := map[string]interface{}{
		"doclevel": openapi.DocLevelAdvancedOnly(),
		"get": map[string]interface{}{
			"operationId": "about",
			"summary":     "Overall information about the service.",
			"tags":        []string{"overall"},
			"responses":   responses,
		},
	}
	// --- END create toplevel object ------------------------

	return openapi.Path{
		Name:   "/api/v1/about",
		Object: obj0,
	}, nil
}

// OAGetPaths ... (see documentation in OAPublisher interface)
func (s *Service) OAGetPaths() ([]openapi.Path, error) {

	paths := []openapi.Path{}

	var err error

	healthzPath, err := getHealthzPath()
	if err != nil {
		return nil, fmt.Errorf("getHealthzPath() failed: %v", err)
	}
	paths = append(paths, healthzPath)

	aboutPath, err := getAboutPath()
	if err != nil {
		return nil, fmt.Errorf("getAboutPath() failed: %v", err)
	}
	paths = append(paths, aboutPath)

	return paths, nil
}

// registerOverallRoutes registers overall routes.
//
// Returns nil upon success, otherwise error.
func (s *Service) registerOverallRoutes(metrics *middleware.Metrics) error {

	s.ExternalRouter.HandleFunc("/readiness", metaservice.HandleReadinessStatus(s.checkReadiness))

	// Health of the service
	s.ExternalRouter.HandleFunc("/api/v1/healthz", metaservice.HandleHealthz(s.checkHealthz))

	// Service discovery metadata for the world
	s.ExternalRouter.Handle(
		"/api/v1/about", localhttp.GetHTTPHandlerForBehindProxy(metaservice.HandleAbout(s.about)))

	// Metrics of the service(s) for this app.
	s.ExternalRouter.Handle("/metrics", metrics.Handler())
	// TODO: add OpenAPI documentation

	// Documentation of the service(s)
	s.ExternalRouter.HandleFunc("/docs/{page}", s.handleDocs)

	// Swagger UI
	swui := http.StripPrefix("/swaggerui/", http.FileServer(http.Dir("./static/swaggerui/")))
	s.ExternalRouter.PathPrefix("/swaggerui/").Handler(swui)

	// Static assets.
	s.ExternalRouter.PathPrefix("/static/").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir(s.staticFilesDir))))

	// Send root path of the http service to the docs index page.
	s.ExternalRouter.HandleFunc("/", s.handleDocs)

	return nil
}

// regRoutesAndOAPubs registers non-EDR routes and OpenAPI publishers to pubs.
// The in/out-parameter features contains features to try to activate (NOTE: additional
// features may be added, e.g. to support other features).
// The in/out-parameter availableFeatures will contain all possible features.
// The in/out-parameter activeFeatures will contain the intersection of features and
// availableFeatures.
//
// Returns nil upon success, otherwise error.
func (s *Service) regRoutesAndOAPubs(
	oaPubs *openapi.OAPublishers, features, availableFeatures,
	activeFeatures *common.StringSet) error {

	epMgr := epmgr.NewEndpointManager(
		oaPubs, s.ExternalRouter, s.metrics, features, availableFeatures, activeFeatures)

	var err error

	// register /api/v1/obs/* routes and corresponding OpenAPI publishers
	// TODO: implement and use obs.RegEndpoints(epMgr) instead!
	_ = epMgr
	if err = obs.RegRoutesAndOAPubs(
		oaPubs, s.ExternalRouter, s.metrics, s.obsStorageBackend, s.responseItemLimit,
		s.staticFilesDir, features, availableFeatures, activeFeatures); err != nil {
		return fmt.Errorf("obs.RegRoutesAndOAPubs() failed: %v", err)
	}

	// register overall routes
	err = s.registerOverallRoutes(s.metrics)
	if err != nil {
		return fmt.Errorf("s.registerOverallRoutes() failed: %v", err)
	}

	return nil // no errors occurred
}

// TODO: add documentation
func (s *Service) checkHealthz() (*metaservice.Healthz, error) {
	return &metaservice.Healthz{
		Status:      metaservice.HealthzStatusHealthy,
		Description: "No deps, so everything is ok all the time.",
	}, nil
}

// TODO: add documentation
func (s *Service) checkReadiness() (*metaservice.ReadinessStatus, error) {
	ready := metaservice.GetReadinessStatus()
	return &ready, nil
}

// TODO: add documentation
func aboutFrost() *metaservice.About {
	return &metaservice.About{
		Name:           "Frost REST service",
		Description:    "The purpose of this service is to deliver MET Norway observation data.",
		Responsible:    "Frost Product Team <hello@met.no>",
		Documentation:  &url.URL{Path: "/"},
		TermsOfService: &url.URL{Path: "/docs/termsofservice"},
	}
}

// TODO: fill in documentation
// html docs generated from templates.
func (s *Service) handleDocs(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	page, exists := params["page"]

	var err error
	if !exists {
		err = s.htmlTemplates.ExecuteTemplate(w, "index", s.about)
	} else {
		err = s.htmlTemplates.ExecuteTemplate(w, page, s.about)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
