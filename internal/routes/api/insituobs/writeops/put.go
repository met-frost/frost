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

type putResponse struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Deleted  int `json:"deleted"`
}

// errors2strings converts errs to strings.
//
// Returns a string array where item[i] is "" if errs[i] is nil, otherwise errs[i].Error().
func errors2strings(errs []error) []string {

	ess := make([]string, len(errs))
	for i, err := range errs {
		if err != nil {
			// assert(err.Error() != ""); TODO: write a check for this?
			ess[i] = err.Error()
		} else {
			ess[i] = ""
		}
	}

	return ess
}

// HandlePut handles a request to the obs/<time series type>/put route. The time series
// type is represented by defaultTS.
func HandlePut(
	defaultTS timeseries.TimeSeries, responseWriter http.ResponseWriter, request *http.Request,
	sbe storagebackends.StorageBackend) {

	totalSummary := storagebackends.ObsWriteSummary{}

	// tsApply applies the 'put' operation on a time series by inserting/updating/deleting
	// observations in the storage backend.
	//
	// Returns (..., ..., nil) if the operation was successfully applied, otherwise
	// (HTTP status code, error details (nil if n/a), error).
	tsApply := func(dts dataset.SingleTSeries, hdr timeseries.Header) (int, interface{}, error) {

		err := hdr.ConvertKeysToLower()
		if err != nil {
			return http.StatusInternalServerError, nil,
				fmt.Errorf("hdr.ConvertKeysToLower() failed: %v", err)
		}

		bstsid, err := json.Marshal(hdr["id"])
		if err != nil {
			return http.StatusInternalServerError, nil,
				fmt.Errorf("json.Marshal(id) failed: %v", err)
		}

		// ensure that the time series already exists in the registry
		if exists, reason := tsregistry.TimeSeriesExists(
			defaultTS.Type(), string(bstsid)); !exists {
			return http.StatusInternalServerError, nil,
				fmt.Errorf("time series not found in internal registry: %s", reason)
		}

		// call ingest hook
		fatalErrs, recovErrs := defaultTS.IngestHook(dts, sbe)
		if fatalErrs != nil { // ingest hook failed

			// check precondition
			if len(fatalErrs) != len(dts.Observations) { // internal error #1
				return http.StatusInternalServerError, nil,
					fmt.Errorf(
						"non-nil fatalErrs returned by ingest hook, but len(fatalErrs) "+
						"(%d) != len(dts.Observations) (%d)", len(fatalErrs),
						len(dts.Observations))
			}

			return http.StatusBadRequest, errors2strings(fatalErrs),
				fmt.Errorf("defaultTS.IngestHook() found at least one unrecoverable error")
		}

		// write the observations to the storage backend
		summary, statusCode, err := sbe.Write(
			defaultTS.Type(), hdr, &dts.Observations, recovErrs)
		if err != nil {
			return statusCode, nil, fmt.Errorf(
				"failed to write observations to storage backend (%s): %v",
				sbe.Description(), err)
		}

		totalSummary.Inserted += summary.Inserted
		totalSummary.Updated += summary.Updated
		totalSummary.Deleted += summary.Deleted

		return -1, nil, nil
	}

	// okResponse returns the payload to be used in a 200 Ok response
	okResponsePayload := func(
		tsRejected, tsAccepted, tsApplied int, tsApplyStatuses []*TsApplyStatus) interface{} {

		// TODO: include following stats in the response if explicitly requested (e.g. with
		// an undocumented query parameter verboseokresponse=true)
		_ = tsRejected
		_ = tsAccepted
		_ = tsApplied
		_ = tsApplyStatuses

		return putResponse{
			Inserted:        totalSummary.Inserted,
			Updated:         totalSummary.Updated,
			Deleted:         totalSummary.Deleted,
		}
	}

	applyWriteOperation("put", defaultTS, responseWriter, request, tsApply, okResponsePayload)
}
