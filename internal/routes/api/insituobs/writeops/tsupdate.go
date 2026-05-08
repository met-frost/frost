package obswriteops

import (
	"encoding/json"
	"fmt"
	"net/http"

	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	storagebackends "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends"
	timeseries "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/tsregistry"
)

type tsUpdateResponse struct {
	Rejected int `json:"rejected"`
	Accepted int `json:"accepted"`
	Applied  int `json:"applied"`
}

// HandleTsUpdate handles a request to the obs/<time series type>/ts/update route.
func HandleTsUpdate(
	defaultTS timeseries.TimeSeries, responseWriter http.ResponseWriter, request *http.Request,
	sbe storagebackends.StorageBackend) {

	// tsApply applies the 'ts/update' operation on a time series by updating it in the storage
	// backend and registry.
	//
	// Returns (..., ..., nil) if the operation was successfully applied, otherwise
	// (HTTP status code, error details (nil if n/a), error).
	tsApply := func(dts dataset.SingleTSeries, hdr timeseries.Header) (int, interface{}, error) {

		statusCode, err := sbe.UpdateTimeSeries(defaultTS.Type(), hdr)
		if err != nil {
			return statusCode, nil, fmt.Errorf(
				"sbe.UpdateTimeSeries() failed (tstype: %s; hdr: %v): %v",
				defaultTS.Type(), hdr, err)
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

		stshdr := fmt.Sprintf(`{"id": %s, "extra": %s}`, string(bstsid), string(bstsextra))

		err = tsregistry.UpdateTimeSeries(defaultTS.Type(), stshdr)
		if err != nil {
			return http.StatusInternalServerError, nil,
				fmt.Errorf(
					"tsregistry.UpdateTimeSeries() failed (tstype: %s; id: %v): %v",
					defaultTS.Type(), hdr["id"], err)
		}

		return -1, nil, nil
	}

	// okResponse returns the payload to be used in a successful response.
	okResponsePayload := func(
		tsRejected, tsAccepted, tsApplied int, tsApplyStatuses []*TsApplyStatus) interface{} {

		_ = tsApplyStatuses // n/a

		return tsUpdateResponse{
			Rejected: tsRejected,
			Accepted: tsAccepted,
			Applied:  tsApplied,
		}
	}

	applyWriteOperation("tsupdate", defaultTS, responseWriter, request, tsApply, okResponsePayload)
}
