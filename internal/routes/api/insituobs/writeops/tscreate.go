package obswriteops

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"gitlab.met.no/frost/frost/internal/common"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	storagebackends "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends"
	timeseries "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/tsregistry"
)

type tsCreateResponse struct {
	Rejected int `json:"rejected"`
	Accepted int `json:"accepted"`
	Applied  int `json:"applied"`
}

// HandleTsCreate handles a request to the obs/<time series type>/ts/create route.
func HandleTsCreate(
	defaultTS timeseries.TimeSeries, responseWriter http.ResponseWriter, request *http.Request,
	sbe storagebackends.StorageBackend) {

	// tsApply applies the 'ts/create' operation on a time series by adding it to the storage
	// backend and registry.
	//
	// Returns (..., ..., nil) if the operation was successfully applied, otherwise
	// (HTTP status code, error details (nil if n/a), error).
	tsApply := func(dts dataset.SingleTSeries, hdr timeseries.Header) (int, interface{}, error) {

		baseID, statusCode, err := sbe.CreateTimeSeries(defaultTS.Type(), hdr)
		if err != nil {
			return statusCode, nil, fmt.Errorf(
				"sbe.CreateTimeSeries() failed (sbe: %s; tstype: %s; hdr: %v): %v",
				sbe.Description(), defaultTS.Type(), hdr, err)
		}

		bstsid, err := json.Marshal(hdr["id"])
		if err != nil {
			return http.StatusInternalServerError, nil,
				fmt.Errorf("json.Marshal(id) failed: %v", err)
		}
		bstsextra, err := json.Marshal(hdr["extra"])
		if err != nil {
			return http.StatusInternalServerError, nil,
				fmt.Errorf("json.Marshal(extra) failed: %v", err)
		}
		stsextra := "{}" // default value in case of (equivalent to) NULL
		if common.IsNonEmptyJSONObject(string(bstsextra)) {
			stsextra = string(bstsextra)
		}

		stsid := string(bstsid)
		stshdr := fmt.Sprintf(`{"id": %s, "extra": %s}`, stsid, stsextra)

		// just set fromTime and toTime as "not in use" for now (TODO?)
		fromTime := int64(math.MinInt64)
		toTime := int64(math.MinInt64)

		nonFatalErrors := []error{}
		_, err = tsregistry.AddTimeSeries(
			defaultTS.Type(), baseID, stshdr, stsid, stsextra, fromTime, toTime, &nonFatalErrors)
		if err != nil {
			return http.StatusInternalServerError, nil,
				fmt.Errorf(
					"tsregistry.AddTimeSeries() failed (tstype: %s; id: %v): %v",
					defaultTS.Type(), hdr["id"], err)
		}

		return -1, nil, nil
	}

	// okResponse returns the payload to be used in a successful response.
	okResponsePayload := func(
		tsRejected, tsAccepted, tsApplied int, tsApplyStatuses []*TsApplyStatus) interface{} {

		_ = tsApplyStatuses // n/a

		return tsCreateResponse{
			Rejected: tsRejected,
			Accepted: tsAccepted,
			Applied:  tsApplied,
		}
	}

	applyWriteOperation("tscreate", defaultTS, responseWriter, request, tsApply, okResponsePayload)
}
