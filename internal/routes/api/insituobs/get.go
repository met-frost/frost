package obs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gitlab.met.no/frost/frost/internal/common"
	localhttp "gitlab.met.no/frost/frost/internal/http"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/readrestriction"
	obssbends "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends"
	timeseries "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
	"gitlab.met.no/frost/frost/internal/routes/api/timespecification"
	"gitlab.met.no/frost/frost/pkg/middleware"
)

// getDefaultItemLimit derives the maximum number of items (time series names or observations) a response
// can contain by default.
//
// Returns (<default limit>, <whether query param 'itemlimit' was specified> nil) on success,
// otherwise (..., ..., error).
func getDefaultItemLimit(queryParams url.Values, serverItemLimit int) (int, bool, error) {

	if serverItemLimit < 1 {
		return -1, false, fmt.Errorf("serverItemLimit not a positive integer: %d", serverItemLimit)
	}

	qpItemLimit := strings.TrimSpace(queryParams.Get("itemlimit"))
	if qpItemLimit == "" {
		return serverItemLimit, false, nil // since itemlimit is infinity by default
	}

	// at this point itemlimit was specified, so 2nd return value is true from now on

	clientItemLimit, err := strconv.Atoi(qpItemLimit)
	if err != nil {
		return -1, true, fmt.Errorf("failed to convert itemlimit (%s) to an int", qpItemLimit)
	}

	if clientItemLimit < 1 {
		return -1, true, fmt.Errorf("itemlimit not a positive integer: %s", qpItemLimit)
	}

	// return min(serverItemLimit, clientItemLimit)
	if serverItemLimit < clientItemLimit {
		return serverItemLimit, true, nil
	}
	return clientItemLimit, true, nil
}

// getIncObs extracts 'incobs' from query parameters.
// Returns (true/false, nil) upon success, otherwise (false, error).
func getIncObs(queryParams url.Values) (bool, error) {
	switch incObs := strings.TrimSpace(queryParams.Get("incobs")); incObs {
	case "":
		return false, nil // default
	case "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, fmt.Errorf(
			"invalid value for incobs: %s (expected 'true' or 'false')", incObs)
	}
}

// responseTooBigErr returns an error that states that the response would be too big to be
// returned to the client, and also, in limReason, any descriptive hints about what the client
// can do to fix the situation. If limReason is "", it means that unlimited response is not
// applicable since 1) the query parameter 'itemlimit' has been set, or 2) the time series type
// in question doesn't support unlimited response.
func responseTooBigErr(limReason string) error {

	var extraPageFitReason string
	if limReason != "" {
		extraPageFitReason = fmt.Sprintf(
			" It might also help to increase/remove the 'itemlimit' query parameter, and "+
				"possibly modify the request to qualify for an unlimited page/response size. "+
				"The current request doesn't qualify for the following reason: %s", limReason)
	}

	return fmt.Errorf("The response is too big. Try to ask for less data.%s", extraPageFitReason)
}

// readItems reads from storage backend sbe as many observations as possible in UTC epoch
// seconds interval [t1, t2> for the time series in tsSeq; first reading observations from
// oldest to newest from (*tsSeq)[0], then from (*tsSeq)[1] etc.
//
// reqInfo may be used by the sbe implementation and also more generally to filter out observations
// once they are retrieved from the sbe.
//
// If itemLimit is negative, there is no limit to how many items can be read and the function will
// thus always return (<item count>, -1, nil).
// Otherwise, if itemLimit is positive, then:
//
//   - If all available observations were successfully read before or at the point where itemLimit
//     is reached (counting both observations and time series headers as items), the function
//     returns (<item count >= 0>, ..., nil).
//
//   - If more observations are available at the point where itemLimit is reached, the function
//     returns (-1, ..., nil).
//
// If latestLimit >= 0, at most the latestLimit newest observations are read for each time
// series. NOTE: if itemLimit > 0 and latestLimit >= 0 and more observations were available before
// itemLimit was reached, then no observations are included (aggregated into dset) for the current
// time series as it is assumed that all relevant observations for that time series will be read
// (some of them over again if necessary) on the request for the next page (assuming all relevant
// observations for that time series will fit in a single page!). In this case the second return
// value (time of next obs in current time series) is n/a and typically set to 0.
//
// If an error occurs, the function returns (..., HTTP status code, error).
//
// On successful return, the observations have been aggregated into dset (i.e. appended to their
// respective time series).
func readItems(
	defaultTS timeseries.TimeSeries, sbe obssbends.StorageBackend, reqInfo timeseries.RequestInfo,
	latestLimit, itemLimit int, tsSeq *timeseries.InstanceSeq, t1, t2 int64,
	dset *dataset.Dataset) (int, int, error) {

	// check overall preconditions
	if itemLimit == 0 {
		return -1, http.StatusInternalServerError,
			fmt.Errorf("itemLimit must be either negative or positive")
	}
	if len(*tsSeq) == 0 {
		return -1, http.StatusInternalServerError, fmt.Errorf("len(*tsSeq) == 0")
	}
	if t1 >= t2 {
		return -1, http.StatusInternalServerError, fmt.Errorf("t1 (%d) >= t2 (%d)", t1, t2)
	}

	tstype := (*(*tsSeq)[0]).Type() // find time series type (assuming same type for all items
	// in tsSeq)

	// extract time series headers
	sizeTSHdrs := len(*tsSeq) // assert sizeTSHdrs > 0
	tsHdrs := make([]timeseries.Header, sizeTSHdrs)
	for i, ts0 := range *tsSeq {
		hdr, err := (*ts0).GetHeader()
		if err != nil {
			return -1, http.StatusInternalServerError,
				fmt.Errorf("(*ts0).GetHeader() failed: %v", err)
		}
		tsHdrs[i] = *hdr
	}

	obs := make([][]dataset.Observation, len(tsHdrs))

	obsBodyModify := func(
		ts0 *timeseries.TimeSeries, t time.Time, body *map[string]any) (int, error) {
		// delegate to obs body modifier specific to time series type
		return (*ts0).ObsBodyModify(t, body)
	}

	obsFilter := func(
		ts0 *timeseries.TimeSeries, t time.Time, body map[string]any) (bool, int, error) {
		// delegate to obs filter specific to time series type
		return (*ts0).ObsFilter(t, body, reqInfo)
	}

	// call back-end function
	itemCount, statusCode, err := sbe.ReadMultiTS(
		tstype, tsSeq, tsHdrs, t1, t2, obsBodyModify, obsFilter, latestLimit, itemLimit,
		&obs, reqInfo, context.Background())
	if err != nil {
		return -1, statusCode, fmt.Errorf("sbe.ReadMultiTS() failed: %v", err)
	}

	if itemCount < 0 { // no room for all requested observations (assert itemLimit > 0)
		return -1, -1, nil
	}

	if len(obs) != len(tsHdrs) {
		return -1, statusCode,
			fmt.Errorf("len(obs) (%d) != len(tsHdrs) (%d)", len(obs), len(tsHdrs))
	}

	// aggregate from obs into dset
	for i, obs0 := range obs { // loop over time series
		hdr := tsHdrs[i]

		// skip if no observations were retrieved for this time series
		if len(obs0) == 0 {
			continue
		}

		// assert(len(obs0) > 0)

		// ensure that obs times in obs0 are in chronological order and without duplicates
		for i := 1; i < len(obs0); i++ {
			t01 := obs0[i-1].Time
			if t01 == nil {
				return -1, http.StatusInternalServerError, fmt.Errorf("t01 == nil")
			}
			t02 := obs0[i].Time
			if t02 == nil {
				return -1, http.StatusInternalServerError, fmt.Errorf("t02 == nil")
			}
			if !t01.Before(*t02) {
				return -1, http.StatusInternalServerError,
					fmt.Errorf("t01 (%v) >= t02 (%v)", *t01, *t02)
			}
		}

		bstsid, err := json.Marshal(hdr["id"])
		if err != nil {
			return -1, http.StatusInternalServerError,
				fmt.Errorf("json.Marshal(id) failed: %v", err)
		}

		var hif any
		err = json.Unmarshal(bstsid, &hif)
		if err != nil {
			return -1, http.StatusInternalServerError,
				fmt.Errorf("json.Unmarshal(id) failed: %v", err)
		}
		hif0, ok := hif.(map[string]any)
		if !ok {
			return -1, http.StatusInternalServerError,
				fmt.Errorf("hif not a map[string]any: %v (type: %T)", hif, hif)
		}

		foundExisting := false
		for _, sts := range dset.TSeries {
			hdrIDsEqual, err := defaultTS.HeaderIDsEqual(sts.Header.ID, hif0)
			if err != nil {
				return -1, http.StatusInternalServerError,
					fmt.Errorf("defaultTS.HeaderIDsEqual() failed: %v", err)
			}
			if hdrIDsEqual { // use existing
				// ensure that the first obs time in obs0 is after any last obs time in sts
				if (len(sts.Observations) > 0) &&
					!(sts.Observations[len(sts.Observations)-1].Time.Before(*obs0[0].Time)) {
					return -1, http.StatusInternalServerError,
						fmt.Errorf(
							"first obs time in obs0 (%v) is not after last obs time in sts (%v)",
							*obs0[0].Time, sts.Observations[len(sts.Observations)-1].Time)
				}

				// append obs0 to sts.Observations
				sts.Observations = append(sts.Observations, obs0...)

				foundExisting = true
				break
			}
		}
		if !foundExisting { // create new
			sts := dataset.SingleTSeries{
				Header: dataset.Header{
					ID:        hdr["id"],
					Extra:     hdr["extra"],
					Available: hdr["available"],
				},
				Observations: obs0,
			}
			dset.TSeries = append(dset.TSeries, sts)
		}
	}

	return itemCount, -1, nil
}

// readTSHeaders reads as many time series headers as possible from tsSeq into dset.
// itemLimit is the maximum allowed number of headers to add to dset (unlimited if itemLimit is
// negative).
//
// Returns (..., nil) on success, otherwise (HTTP status code, error).
func readTSHeaders(
	tsSeq *timeseries.InstanceSeq, itemLimit int, limReason string, dset *dataset.Dataset) (
	int, error) {

	var lastIndex int
	switch {
	case itemLimit > 0:
		lastIndex = itemLimit - 1
	case itemLimit < 0:
		lastIndex = len(*tsSeq) - 1 // i.e. effectively unlimited
	default:
		return http.StatusInternalServerError,
			fmt.Errorf("itemLimit must be either negative or positive")
	}

	// check if the time series in tsSeq fit in this page ...
	if lastIndex >= (len(*tsSeq) - 1) { // ... yes
		lastIndex = len(*tsSeq) - 1
	} else { // ... no
		return http.StatusForbidden, responseTooBigErr(limReason)
	}

	// fill page with time series headers in range [0, lastIndex]
	for tsindex := 0; tsindex <= lastIndex; tsindex++ {
		hdr, err := (*(*tsSeq)[tsindex]).GetHeader()
		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("(*tsSeq[tsindex]).GetHeader() failed: %v", err)
		}
		tseries := dataset.SingleTSeries{
			Header: dataset.Header{
				ID:        (*hdr)["id"],
				Extra:     (*hdr)["extra"],
				Available: (*hdr)["available"],
			},
		}

		// add tseries (containing the header only!) to the dataset
		dset.TSeries = append(dset.TSeries, tseries)
	}

	return -1, nil
}

// readItemsInLatestMode reads, for a time specification in 'latest' mode, as many time series
// headers (from tsSeq) and observations (from sbe) as possible into dset.
// lspec is the 'latest' time specification, itemLimit is the maximum allowed number of items
// (time series headers or observations) to add to dset (unlimited if negative).
//
// On success, the result is written to dset and the function returns (..., nil), otherwise the
// the function returns (HTTP status code, error).
func readItemsInLatestMode(
	defaultTS timeseries.TimeSeries, sbe obssbends.StorageBackend, reqInfo timeseries.RequestInfo,
	tsSeq *timeseries.InstanceSeq, lspec *timespecification.LatestSpec, itemLimit int,
	limReason string, dset *dataset.Dataset) (int, error) {

	// check overall preconditions
	if itemLimit == 0 {
		return http.StatusInternalServerError,
			fmt.Errorf("itemLimit must be either negative or positive")
	}

	now := time.Now().Unix()

	itemCount, statusCode, err := readItems(
		defaultTS, sbe, reqInfo, lspec.Limit, itemLimit, tsSeq, now-lspec.MaxAge, now+1, dset)
	if err != nil {
		return statusCode, fmt.Errorf("readItems() failed: %v", err)
	}
	if itemCount == -1 { // assert itemLimit > 0
		return http.StatusForbidden, responseTooBigErr(limReason)
	}

	return -1, nil
}

// readItemsInIntervalsMode reads, for a time specification in 'intervals' mode, as many time
// series headers (from tsSeq) and observations (from sbe) as possible into dset.
// ispec is the 'intervals' time specification, itemLimit is the maximum allowed number of items
// (time series headers or observations) to add to dset (unlimited if itemLimit is negative).
//
// On success, the result is written to dset and the function returns (-1, nil), otherwise the
// function returns (HTTP status code, error).
func readItemsInIntervalsMode(
	defaultTS timeseries.TimeSeries, sbe obssbends.StorageBackend, reqInfo timeseries.RequestInfo,
	tsSeq *timeseries.InstanceSeq, ispec *timespecification.IntervalsSpec, itemLimit int,
	limReason string, dset *dataset.Dataset) (int, error) {

	// check overall preconditions
	if itemLimit == 0 {
		return http.StatusInternalServerError,
			fmt.Errorf("itemLimit must be either negative or positive")
	}

	// initialize t1 and t2 to first time interval
	t1 := ispec.T1
	t2 := ispec.T2

	// initialize total item count
	totItemCount := 0

	// assert(t1.Before(t2))
	// assert(!t2.After(ispec.T2Last))

	// loop over time intervals
	for {

		var effectiveItemLimit int
		switch {
		case itemLimit > 0:
			effectiveItemLimit = itemLimit - totItemCount
			if effectiveItemLimit <= 0 { // no room left for the current interval
				return http.StatusForbidden, responseTooBigErr(limReason)
			}
		default: // assert itemLimit < 0
			effectiveItemLimit = -1 // i.e. effectively unlimited
		}

		itemCount, statusCode, err := readItems(
			defaultTS, sbe, reqInfo, -1, effectiveItemLimit, tsSeq, t1.Unix(), t2.Unix(), dset)
		if err != nil {
			return statusCode, fmt.Errorf("readItems() failed: %v", err)
		}
		if itemCount == -1 { // too many observations in the current interval
			// assert effectiveItemLimit > 0
			return http.StatusForbidden, responseTooBigErr(limReason)
		}

		// if the current interval was the last one, we're done
		if t2.Equal(ispec.T2Last) {
			break
		}

		// move to first time series in next interval
		t1next, t2next, found, err := ispec.NextInterval(t1, t2)
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf(
				"ispec.NextInterval() failed: %v", err)
		}
		if !found {
			return http.StatusInternalServerError, fmt.Errorf(
				"failed to find a next interval after [%v, %v>", t1, t2)
		}
		t1 = t1next
		t2 = t2next

		// update total item count
		totItemCount += itemCount
	}

	return -1, nil
}

// readPage reads a page of items consisting of time series headers and (possibly) observations.
//
// Assumptions about observation times:
//   - no two observations within the same time series may share the same observation time
//   - all observation times are represented as 64-bit UTC epoch seconds (negative values
//     for observations before 1970-01-01T00:00:00Z)
//
// __________________________________________________________________________________________
// *** MAIN CASE 1: Observations are not requested (i.e. incObs==false) ***
//
// The page will simply be filled up with time series from tsSeq.
// NOTE: time series whose valid period doesn't overlap with the overall time period defined in
// tspec (in 'latest' or 'intervals' mode) are assumed to have been filtered out from tsSeq,
// hence no new checks need to be made against tspec in this case.
//
// __________________________________________________________________________________________
// *** MAIN CASE 2: Observations are requested (i.e. incObs==true) ***
//
// Calls are made to sbe.ReadMultiTS() to read as many observations into the page as possible
// (observations are organized chronologically after their respective time series headers - see
// below for more details).
//
// The tspec parameter defines what information the result will include for each time series TS:
//
//	*** 'latest' mode (tspec.LSpec != nil):
//	  Include observations for the most recent times according to restrictions in tspec.LSpec
//	  (lspec.MaxAge and lspec.Limit).
//
//	- include TS header + as many observations as possible (starting from the newest one)
//	  that:
//
//	   o don't exceed lspec.Limit in number
//
//	   o are in the interval [now-lspec.MaxAge, now]
//
//	   o can be accommodated in the overall page (i.e. wrt. itemLimit)
//
//	*** 'intervals' mode (tspec.ISpec != nil):
//	  Include observations inside a sequence of open-ended intervals defined by tspec.ISpec
//	  (and partially based on the concept of 'repeating intervals' in ISO 8601).
//
//	- include TS header + as many observations as possible (starting from the earliest one)
//    that:
//
//	  o are inside the intervals
//
//	  o can be accommodated in the overall page (i.e. wrt. itemLimit)
//
//	  (the N intervals in tspec.ISpec are defined like this:
//	  interval 1:     [T1, T2>
//	  interval i > 1: [T1(i), T2(i)>, where Tx(i) is the period (PY,PM,PD,PS) added i-1
//	  times to Tx for x = 1 or 2.)
//
//	*** Neither 'intervals' nor 'latest' mode ((tspec.ISpec == nil) && (tspec.LSpec == nil)):
//	  This is considered an error condition (i.e. including observations without a time
//	  specification is not supported) and is assumed to be handled already.
//
// __________________________________________________________________________________________
// If itemLimit is negative, an unlimited number of items are read into dset, otherwise, if the
// total number of items exceeds itemLimit, dset is left empty and (403 Forbidden, “...”) is
// returned.
//
// On success, the result is written to dset and the function returns (-1, nil), otherwise the
// function returns (HTTP status code, error).
func readPage(
	defaultTS timeseries.TimeSeries, reqInfo timeseries.RequestInfo,
	sbe obssbends.StorageBackend, tsSeq *timeseries.InstanceSeq,
	tspec timespecification.TimeSpecification, incObs bool, itemLimit int, limReason string,
	dset *dataset.Dataset) (int, error) {

	// check overall preconditions
	if itemLimit == 0 {
		return http.StatusInternalServerError,
			fmt.Errorf("itemLimit must be either negative or positive")
	}

	// remove any existing values in dset
	dset.TSeries = nil

	if !incObs {
		// *** MAIN CASE 1: Observations are not requested (i.e. incObs==false) ***

		statusCode, err := readTSHeaders(tsSeq, itemLimit, limReason, dset)
		if err != nil {
			return statusCode, fmt.Errorf("readTSHeaders() failed: %v", err)
		}

		return -1, nil
	}

	// *** MAIN CASE 2: Observations are requested (i.e. incObs==true) ***

	// (NOTE: we assume that the representation invariants of tspec have been validated already,
	// i.e. that at most one of tspec.LSpec and tspec.ISpec can be non-nil and so on ...)

	lspec := tspec.LSpec
	latestMode := lspec != nil
	ispec := tspec.ISpec
	intervalsMode := ispec != nil

	// ensure that a time is specified
	if (!latestMode) && (!intervalsMode) {
		return http.StatusInternalServerError,
			fmt.Errorf("can't include observations without a time specification " +
				"(programming error, since this should have been checked already!)")
	}

	if latestMode {
		// assert(!intervalsMode)
		statusCode, err := readItemsInLatestMode(
			defaultTS, sbe, reqInfo, tsSeq, lspec, itemLimit, limReason, dset)
		if err != nil {
			return statusCode, fmt.Errorf("readItemsInLatestMode() failed: %v", err)
		}

		// apply any read restriction rules to dset by replacing affected observation bodies
		// with null
		err = readrestriction.Apply(dset, reqInfo.Request)
		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("readrestriction.Apply()) failed: %v", err)
		}

		return -1, nil
	}

	// assert(intervalsMode)
	statusCode, err := readItemsInIntervalsMode(
		defaultTS, sbe, reqInfo, tsSeq, ispec, itemLimit, limReason, dset)
	if err != nil {
		return statusCode, fmt.Errorf("readItemsInIntervalsMode() failed: %v", err)
	}

	// apply any read restriction rules to dset by replacing affected observation bodies
	// with null
	err = readrestriction.Apply(dset, reqInfo.Request)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("readrestriction.Apply() failed: %v", err)
	}

	return -1, nil
}

// getTimeSeriesInstances retrieves into tsSeq the total set of time series (of a given type)
// requested according to reqInfo (tspec is assumed to be derived from reqInfo.QueryParams
// already) and roles (assumed to be extracted from the request already).
// tsSeq will be sorted (typically on serialized hdr/id and other criteria) to support the
// pagination protocol (if active).
// Returns (-1, nil) upon success, otherwise (HTTP status code, error).
func getTimeSeriesInstances(
	defaultTS timeseries.TimeSeries, reqInfo timeseries.RequestInfo,
	tspec timespecification.TimeSpecification, tsSeq *timeseries.InstanceSeq, roles []string) (
	int, error) {

	// get initial set of matching time series instances
	statusCode, err := defaultTS.GetInstances(reqInfo.QueryParams, roles, tsSeq)
	if err != nil {
		return statusCode, fmt.Errorf("defaultTS.GetInstances() failed: %v", err)
	}

	// remove from tsSeq time series whose validity period doesn't overlap with the overall time
	// period in tspec
	if (tspec.LSpec != nil) || (tspec.ISpec != nil) {
		t1, t2 := tspec.OverallInterval()
		//assert !((t1 == math.MinInt64) && (t2 == math.MinInt64))

		// collect the overlapping time series into olapTsSeq
		olapTsSeq := timeseries.InstanceSeq{}
		for _, ts := range *tsSeq {
			ft := (*ts).GetFromTime()
			tt := (*ts).GetToTime()
			if ((*tt == math.MinInt64) || (int64(*tt) >= t1)) &&
				((*ft == math.MinInt64) || (int64(*ft) < t2)) {
				olapTsSeq = append(olapTsSeq, ts) // overlapping, so keep
			}
		}

		*tsSeq = olapTsSeq // replace tsSeq with olapTss
	}

	// perform further tstype-dependent filtering on tsSeq (e.g. geo search and other filtering
	// that can not be expressed in terms of a struct selector)
	// ### TODO: do this as part of defaultTS.GetInstances() instead!
	statusCode, err = defaultTS.HeaderFilterSpecial(reqInfo, tsSeq)
	if err != nil {
		return statusCode, fmt.Errorf("defaultTS.HeaderFilterSpecial() failed: %v", err)
	}

	// filter tsSeq according to header geo points and proximity geo search
	statusCode, err = defaultTS.HeaderPxmtyFilter(reqInfo, tsSeq)
	if err != nil {
		return statusCode, fmt.Errorf("defaultTS.HeaderPxmtyFilter() failed: %v", err)
	}

	// finalize tsSeq order
	statusCode, err = defaultTS.FinalizeInstanceOrder(tsSeq)
	if err != nil {
		return statusCode, fmt.Errorf("defaultTS.FinalizeInstanceOrder() failed: %v", err)
	}

	return -1, nil
}

// HandleGetCore is the core implementation of handling a request to the
// obs/<time series type>/get route.
//
// On success, the function copies any matching time series to tsSeq and any matching data
// (ts headers + possibly observations) to dset and returns (-1, nil).
//
// On failure, the function returns (HTTP status code, error).
func HandleGetCore(
	defaultTS timeseries.TimeSeries, request *http.Request, queryParams url.Values,
	sbe obssbends.StorageBackend, serverItemLimit int, tsSeq *timeseries.InstanceSeq,
	dset *dataset.Dataset) (int, error) {

	// ensure that all query parameters are supported for this time series type
	unsupQPMsg := timeseries.GetUnsupportedQueryParamsMsg(&defaultTS, queryParams)
	if unsupQPMsg != "" {
		return http.StatusBadRequest, errors.New(unsupQPMsg)
	}

	// set up a request info object
	customReqInfo, err := defaultTS.CreateCustomReqInfo(queryParams)
	if err != nil {
		return http.StatusBadRequest,
			fmt.Errorf("defaultTS.CreateCustomReqInfo() failed: %v", err)
	}
	reqInfo := timeseries.RequestInfo{
		Request:     request,
		QueryParams: queryParams,
		Custom:      customReqInfo,
	}

	// get any roles used for filtering out restricted time series
	roles := make([]string, 0)
	if roles0 := reqInfo.Request.Context().Value(middleware.ContextRolesKey); roles0 != nil {
		if roles1, ok := roles0.(string); ok {
			roles = common.ExtractCSVVals(roles1)
		} else {
			return http.StatusInternalServerError,
				fmt.Errorf("roles0 not a string: %v (type: %T)", roles0, roles0)
		}
	}

	// get the time specification
	tspec, err := timespecification.GetTimeSpecification(queryParams)
	if err != nil {
		return http.StatusBadRequest,
			fmt.Errorf("timespecification.GetTimeSpecification() failed: %v", err)
	}

	// extract value of the 'incobs' query parameter
	incObs, err := getIncObs(queryParams)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("getIncObs() failed: %v", err)
	}

	// validate tspec/incobs combo
	if (tspec.ISpec == nil) && (tspec.LSpec == nil) && incObs {
		return http.StatusBadRequest,
			fmt.Errorf("can't include observations without a time specification")
	}

	// get requested time series
	statusCode, err := getTimeSeriesInstances(defaultTS, reqInfo, tspec, tsSeq, roles)
	if err != nil {
		return statusCode, fmt.Errorf("getTimeSeriesInstances() failed: %v", err)
	}

	if len(*tsSeq) == 0 {
		return -1, nil // no matching time series found
	}

	// derive max total item count in a single response
	itemLimit, itemLimitSpecified, err := getDefaultItemLimit(queryParams, serverItemLimit)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("getDefaultItemLimit() failed: %v", err)
	}

	// check if this request qualifies for an unlimited response
	var unlimResp bool   // response is limited by default
	var limReason string // no limReason by default
	if !incObs {
		unlimResp = true
	} else {
		// assert (tspec.ISpec == nil) || (tspec.LSpec == nil)
		unlimResp, limReason, statusCode, err = defaultTS.UnlimitedResponse(tsSeq, tspec)
		if err != nil {
			return statusCode, fmt.Errorf("defaultTS.UnlimitedResponse() failed: %v", err)
		}
	}

	if unlimResp && !itemLimitSpecified {
		// at this point: 1) the request qualifies for an unlimited response and 2) the request
		// doesn't specify a limited response, so ensure that the response will indeed be
		// unlimited:
		itemLimit = -1
		// note that limReason is n/a in this case
	} /*else {
		// at this point, either ...
		// 1) unlimResp==false, in which case incObs==true, defaultTS.UnlimitedResponse()
		//    was called, and limReason was set (to either "" if the time series type in question
		//    doesn't support unlimited response, or to a non-empty string if it does), or
		// 2) itemLimitSpecified==true, in which case itemLimit is still > 0 and limReason is "",
		//    which is fine since the request isn't prepared for an unlimited response anyway.
	}*/

	// set the time series type
	dset.TSeriesType = defaultTS.Type()

	// read page
	statusCode, err = readPage(
		defaultTS, reqInfo, sbe, tsSeq, tspec, incObs, itemLimit, limReason, dset)
	if err != nil {
		return statusCode, fmt.Errorf("readPage() failed: %v", err)
	}

	return -1, nil
}

// formatResponseBodyAsJSON formats dset as JSON.
//
// Returns (serialized JSON object, content type, nil) on success, otherwise (..., ..., error).
func formatResponseBodyAsJSON(dset *dataset.Dataset) ([]byte, string, error) {
	rmap := make(map[string]any)
	rmap["data"] = *dset
	body, err := json.Marshal(rmap)
	if err != nil {
		return nil, "", fmt.Errorf("json.Marshal() failed: %v", err)
	}
	return body, "application/json;charset=UTF-8", nil
}

// HandleGet handles a request to the obs/<time series type>/get route. The time series
// type is represented by defaultTS.
func HandleGet(
	defaultTS timeseries.TimeSeries, responseWriter http.ResponseWriter, request *http.Request,
	sbe obssbends.StorageBackend, serverItemLimit int) {

	var tsSeq = timeseries.InstanceSeq{} // matching time series
	var dset = dataset.Dataset{}         // dataset for keeping the retrieved data

	queryParams, err := localhttp.ExtractQueryParameters(request)
	if err != nil {
		localhttp.SetErrorResponse(
			http.StatusInternalServerError,
			fmt.Errorf("localhttp.ExtractQueryParameters() failed: %v", err),
			responseWriter, request)
		return
	}

	// call core function
	statusCode, err := HandleGetCore(
		defaultTS, request, queryParams, sbe, serverItemLimit, &tsSeq, &dset)
	if err != nil {
		localhttp.SetErrorResponse(
			statusCode, fmt.Errorf("HandleGetCore() failed: %v", err), responseWriter, request)
		return
	}

	// check if no time series were found
	if len(tsSeq) == 0 {
		localhttp.SetErrorResponse(
			http.StatusNotFound, fmt.Errorf("no matching time series found"),
			responseWriter, request)
		return
	}

	// check if dataset is still completely empty
	if len(dset.TSeries) == 0 {
		localhttp.SetErrorResponse(
			http.StatusNotFound, fmt.Errorf(
				"found %d matching time series, but no data", len(tsSeq)),
			responseWriter, request)
		return
	}

	// format non-empty dataset in the response

	responseBody, contentType, err := formatResponseBodyAsJSON(&dset)
	if err != nil {
		localhttp.SetErrorResponse(
			http.StatusInternalServerError, fmt.Errorf("formatResponseBody() failed: %v", err),
			responseWriter, request)
		return
	}
	responseWriter.Header().Set("Content-Type", contentType)
	localhttp.SetOkResponse(responseBody, responseWriter, request)
}
