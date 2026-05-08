package obs

import (
	"encoding/json"
	"fmt"
	"net/http"

	localhttp "gitlab.met.no/frost/frost/internal/http"
	obssbends "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends"
	timeseries "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
)

// HandleStatus handles a request to the obs/<time series type>/status route. The time series
// type is represented by defaultTS.
func HandleStatus(
	defaultTS timeseries.TimeSeries, responseWriter http.ResponseWriter, request *http.Request,
	sbe obssbends.StorageBackend) {

	queryParams, err := localhttp.ExtractQueryParameters(request)
	if err != nil {
		localhttp.SetErrorResponse(
			http.StatusInternalServerError,
			fmt.Errorf("localhttp.ExtractQueryParameters() failed: %v", err),
			responseWriter, request)
		return
	}

    status, err := defaultTS.GetStatus(queryParams)
	if err != nil {
		localhttp.SetErrorResponse(
			http.StatusInternalServerError,
			fmt.Errorf("defaultTS.GetStatus() failed: %v", err),
			responseWriter, request)
		return
	}

	responseBody, err := json.Marshal(status)
	if err != nil {
		localhttp.SetErrorResponse(
			http.StatusInternalServerError,
			fmt.Errorf("json.Marshal() failed: %v", err),
			responseWriter, request)
		return
	}

	responseWriter.Header().Set("Content-Type", "application/json;charset=UTF-8")
	localhttp.SetOkResponse(responseBody, responseWriter, request)
}
