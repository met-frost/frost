package obswriteops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"gitlab.met.no/frost/frost/internal/common"
	localjson "gitlab.met.no/frost/frost/internal/common/json"
	localhttp "gitlab.met.no/frost/frost/internal/http"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	timeseries "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/tsregistry"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/writerestriction"
)

// extractDatasetFromRequestNEW extracts into dset the 'dataset' part from a request with a body
// of type application/json.
//
// Returns (-1, nil) upon success, otherwise (HTTP status code, error).
func extractDatasetFromRequest(
	defaultTS timeseries.TimeSeries, request *http.Request, dset *dataset.Dataset) (int, error) {

	var err error

	// ensure that the content is not too big
	const maxContentLength = 1000000 // TODO: define this to some appropriate value
	contentLength := request.ContentLength
	if contentLength > maxContentLength {
		return http.StatusBadRequest,
			fmt.Errorf(
				"max request content length exceeded: %d > %d", contentLength, maxContentLength)
	}

	// validate the media type
	mtype, _, err := mime.ParseMediaType(request.Header.Get("Content-type"))
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("failed to parse media type: %v", err)
	}
	expectedMType := "application/json"
	if mtype != expectedMType {
		return http.StatusBadRequest,
			fmt.Errorf("expected media type %s; got %s", expectedMType, mtype)
	}

	// read the JSON request body
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return http.StatusBadRequest,
			fmt.Errorf("failed to read request body; io.ReadAll() failed: %v", err)
	}

	// validate body
	err = dataset.Validate(body)
	if err != nil {
		return http.StatusBadRequest,
			fmt.Errorf("failed to validate request body; dataset.Validate() failed: %v", err)
	}

	// decode body into dset
	dec := json.NewDecoder(bytes.NewReader(body))
	err = dec.Decode(dset)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("dec.Decode() failed: %v", err)
	}

	// check that the time series type is supported
	_, err = tsregistry.DefaultTimeSeries(dset.TSeriesType)
	if err != nil {
		return http.StatusBadRequest,
			fmt.Errorf(
				"time series type not supported: %s (supported types: %s)",
				dset.TSeriesType, strings.Join(tsregistry.TypeNames(), ", "))
	}

	// check that the time series type is enabled (i.e. specified in the FEATURES env.var.)
	etn := tsregistry.EnabledTypeNames()
	if !etn.Contains(dset.TSeriesType) {
		return http.StatusBadRequest,
			fmt.Errorf(
				"time series type not enabled: %s (enabled types: %s)",
				dset.TSeriesType, strings.Join(etn.ToList(), ", "))
	}

	// check that the time series type in the dataset matches the one in the route
	if dset.TSeriesType != defaultTS.Type() {
		return http.StatusBadRequest,
			fmt.Errorf("time series type in route (%s) differs from the one in the dataset (%s)",
				defaultTS.Type(), dset.TSeriesType)
	}

	// loop over time series in dataset and process the parts of each one
	for _, dts := range dset.TSeries {
		// header/id ...
		if dts.Header.ID != nil { // header/id as a whole is optional
			err = common.ConvertObjKeysToLower(&dts.Header.ID)
			if err != nil {
				return http.StatusInternalServerError,
					fmt.Errorf("common.ConvertObjKeysToLower() failed for dts.Header.ID: %v", err)
			}
			err = defaultTS.ValidateHdrID(dts.Header.ID)
			if err != nil {
				return http.StatusBadRequest,
					fmt.Errorf("invalid 'id' part found in time series header: %v", err)
			}
		}

		// header/extra ...
		if dts.Header.Extra != nil { // header/extra as a whole is optional
			err = common.ConvertObjKeysToLower(&dts.Header.Extra)
			if err != nil {
				return http.StatusInternalServerError,
					fmt.Errorf("common.ConvertObjKeysToLower() failed for dts.Header.Extra: %v", err)
			}
			err = defaultTS.ValidateHdrExtra(dts.Header.Extra)
			if err != nil {
				return http.StatusBadRequest,
					fmt.Errorf("invalid 'extra' part found in time series header: %v", err)
			}
		}

		// observations/body ...
		for _, obs := range dts.Observations {
			if obs.Body != nil {
				err = common.ConvertObjKeysToLower(&obs.Body)
				if err != nil {
					return http.StatusInternalServerError,
						fmt.Errorf("common.ConvertObjKeysToLower() failed for obs.Body: %v", err)
				}
				err = defaultTS.ValidateObsBody(obs.Body)
				if err != nil {
					return http.StatusBadRequest,
						fmt.Errorf("invalid 'body' part found in time series observations: %v", err)
				}
			}
			// note: obs.Body is allowed to be nil (regardless of time series type) for the
			// use case of deleting the observation at that timestamp
		}
	}

	return -1, nil // dataset successfully extracted
}

// matchesWritableHeaderIDPattern checks if tsid matches any of the patterns in whiPatterns.
//
// Returns (bool, nil) upon success, otherwise (..., error).
func matchesWritableHeaderIDPattern(
	tsid map[string]interface{}, whiPatterns *[]string) (bool, error) {

	for _, whiPattern := range *whiPatterns {
		var whiPatternIF interface{}
		if err := json.Unmarshal([]byte(whiPattern), &whiPatternIF); err != nil {
			return false, fmt.Errorf("json.Unmarshal() failed: %v", err)
		}

		match, _, err := localjson.IsJSONSubMatch(whiPatternIF, tsid)
		if err != nil {
			return false, fmt.Errorf("localjson.IsJSONSubMatch() failed: %v", err)
		}

		if match {
			return true, nil // match found
		}
	}

	return false, nil // no match found
}

type TsApplyStatus struct {
	StatusCode int `json:"statuscode"` // HTTP status code (http.StatusOK iff apply operation was
	// successful)
	Error string `json:"error"` // error summary
	Details interface{} `json:"details"` // error details (nil if n/a)
}

// MarshalJSON is a custom JSON marshaller for *tsApplyStatus.
func (tas *TsApplyStatus) MarshalJSON() ([]byte, error) {

	if tas != nil {
		type err struct {
			Error TsApplyStatus `json:"error"`
		}
		return json.Marshal(err{*tas})
	}

	return json.Marshal(nil)
}

// applyWriteOperation is a template function that applies a write operation.
// TODO: add more documentation!
func applyWriteOperation(
	opname string, defaultTS timeseries.TimeSeries, responseWriter http.ResponseWriter,
	request *http.Request, tsApply func(dataset.SingleTSeries, timeseries.Header) (int, interface{},
		error), okResponsePayload func(int, int, int, []*TsApplyStatus) interface{}) {

	tsRejected := 0
	tsAccepted := 0
	tsApplied := 0

	var whiPatterns []string // writable header ID patterns (if write restriction is generally
	// enabled, a time series in the dataset can be applied iff its header matches at least one of
	// these patterns)

	// get write restriction
	wrEnabled, whiPatterns, statusCode, err := writerestriction.GetWriteRestriction(
		defaultTS.Type(), opname, request)
	if err != nil {
		localhttp.SetErrorResponse(
			statusCode, fmt.Errorf(
				"writerestriction.GetWriteRestriction() failed: %v", err),
			responseWriter, request)
		return
	}

	// extract dataset
	var dset dataset.Dataset
	statusCode, err = extractDatasetFromRequest(defaultTS, request, &dset)
	if err != nil {
		localhttp.SetErrorResponse(
			statusCode, fmt.Errorf("extractDatasetFromRequest() failed: %v", err),
			responseWriter, request)
		return
	}

	tsApplyStatuses := make([]*TsApplyStatus, len(dset.TSeries)) // initially all nils

	// apply operation for each writable time series in the dataset
	for i, dts := range dset.TSeries {
		hdr := timeseries.Header{
			"id":    dts.Header.ID,
			"extra": dts.Header.Extra,
		}

		// check if time series is writable
		if wrEnabled { // write restriction generally enabled
			// skip dts if its header doesn't match at least one of the writable header ID patterns
			writable, err := matchesWritableHeaderIDPattern(hdr["id"], &whiPatterns)
			if err != nil {
				localhttp.SetErrorResponse(
					http.StatusInternalServerError,
					fmt.Errorf("matchesWritableHeaderIDPattern() failed: %v", err),
					responseWriter, request)
				return
			}
			if !writable {
				tsRejected++
				continue
			}
		}

		// at this point the time series is writable and can be applied
		tsAccepted++

		// apply operation
		statusCode, errDetails, err := tsApply(dts, hdr)
		if err == nil {
			tsApplied++
		} else {
			tsApplyStatuses[i] = &TsApplyStatus{statusCode, err.Error(), errDetails}
		}
	}

	// format a report in the response
	js, err := json.Marshal(
		okResponsePayload(tsRejected, tsAccepted, tsApplied, tsApplyStatuses))
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
