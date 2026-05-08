package obs

import (
	"encoding/json"
	"fmt"
	"net/http"

	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/tsregistry"

	localhttp "gitlab.met.no/frost/frost/internal/http"
	storagebackend "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends"
)

type clearResponse struct {
	StorageBackend string `json:"storagebackend"`
}

// HandleClear handles a request to the obs/clear route.
func HandleClear(
	responseWriter http.ResponseWriter, request *http.Request, sbe storagebackend.StorageBackend) {

	if !sbe.SupportsClear() {
		localhttp.SetErrorResponse(
			http.StatusInternalServerError,
			fmt.Errorf(
				"'clear' operation not supported by storage backend: %s", sbe.Description()),
			responseWriter, request)
		return
	}

	// clear backend storage
	statusCode, err := sbe.Clear()
	if err != nil {
		localhttp.SetErrorResponse(
			statusCode,
			fmt.Errorf(
				"failed to clear all data from backend storage (%s): %v", sbe.Description(), err),
			responseWriter, request)
		return
	}

	// clear registry
	tsregistry.Clear()

	// all data successfully cleared, so format a report in the response
	js, err := json.Marshal(clearResponse{StorageBackend: sbe.Description()})
	if err != nil {
		localhttp.SetErrorResponse(
			http.StatusInternalServerError,
			fmt.Errorf("failed to format JSON response: %v", err),
			responseWriter,
			request)
		return
	}
	responseWriter.Header().Set("Content-Type", "application/json;charset=UTF-8")
	localhttp.SetOkResponse(js, responseWriter, request)
}
