// Package storagebackends ... TO BE DOCUMENTED.
package storagebackends

// ### TODO: rename 'package storagebackends' to 'package storagebackend' ??

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"gitlab.met.no/frost/frost/internal/common"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
)

// ObsWriteSummary keeps a count of the number of observations that were respectively
// inserted, updated, or deleted by a write operation.
type ObsWriteSummary struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Deleted  int `json:"deleted"`
}

// StorageBackend is the interface for an observation storage back-end.
// Examples of specific back-ends: LARD server, Postgres server, local memory structure.
type StorageBackend interface {

	// Description returns the description of the back-end.
	Description() string

	// CreateTimeSeries creates in the back-end a time series of type tstype and header hdr.
	// The function also generates the internal base ID of the time series (often this will be
	// the same as the ID part of hdr (hdr/id), but in some contexts it is convenient to keep a
	// separate base ID, such as when hdr/id can be modified during the lifetime of the time
	// series).
	// Upon success, the function returns (base ID, -1, nil),
	// otherwise (..., HTTP status code, error).
	CreateTimeSeries(tstype string, hdr timeseries.Header) (string, int, error)

	// RemoveTimeSeries deletes from the back-end a time series of type tstype and 'id' part
	// matching the one in header hdr.
	// Upon success, the function returns (-1, nil), otherwise (HTTP status code, error).
	RemoveTimeSeries(tstype string, hdr timeseries.Header) (int, error)

	// UpdateTimeSeries updates in the back-end the 'extra' part of a time series of type tstype
	// and 'id' part matching the one in header hdr.
	// Upon success, the function returns (-1, nil), otherwise (HTTP status code, error).
	UpdateTimeSeries(tstype string, hdr timeseries.Header) (int, error)

	// ReadSingleTS reads from the back-end as many observations as possible in the first part of
	// time range [t1, t2> in a time series of type tstype and 'id' part matching the
	// one in header hdr. A pointer to the corresponding timeseries.TimeSeries instance is passed
	// in ts0.
	//
	// Observations will be modified according to obsBodyModify.
	//
	// Observations for which obsFilter returns false will be skipped. The rest of the
	// documentation applies only to observations that passed obsFilter.
	//
	// If limit is > 0, then at most limit observations are read in total.
	// Otherwise, if limit < 0, then an unlimited number of observations can be read.
	//
	// reqInfo contains additional information that could be useful to the implementation.
	//
	// On success, the function stores the result in chronological order (oldest observation
	// first) in 'observations' and returns (excess, ..., nil), where excess indicates whether
	// adding all available observations before t2 would have caused the item limit to be exceeded.
	// If limit < 0, excess is not applicable (since the function will always read all available
	// observations in [t1, t2>). In that case, excess is returned as false.
	//
	// On failure, the function returns (..., HTTP status code, error).
	//
	ReadSingleTS(
		tstype string, ts0 *timeseries.TimeSeries, hdr timeseries.Header, t1, t2 int64,
		obsBodyModify func(*timeseries.TimeSeries, time.Time, *map[string]any) (int, error),
		obsFilter func(*timeseries.TimeSeries, time.Time, map[string]any) (
			bool, int, error),
		limit int, observations *[]dataset.Observation, reqInfo timeseries.RequestInfo) (
		bool, int, error)

	// ReadMultiTS reads from the back-end, and into obs, as many observations as possible for the
	// time series (all of type tstype) defined by (i.e. matching the 'id' part in) hdrs and the
	// time range [t1, t2>. Pointers to the corresponding timeseries.TimeSeries instances are passed
	// in tsSeq.
	//
	// Observations will then first be modified according to obsBodyModify.
	//
	// Observations for which obsFilter returns false will be skipped. The rest of the
	// documentation applies only to observations that passed obsFilter.
	//
	// Observations will be appended to the output structure obs one time series at a time
	// according to the order defined in tsSeq/hdrs. Within each time series, observations are
	// appended in chronological order, starting with the oldest one.
	//
	// NOTE: both observations and time series headers count as *items* (as an example: a dataset
	// with 3 time series, each with 10 observations, contains a total of 33 items).
	//
	// If latestLimit >= 0, at most the latestLimit newest observations are read for each time
	// series.
	//
	// If itemLimit is > 0, then at most itemLimit items are read in total.
	// Otherwise, if itemLimit < 0, then an unlimited number of items can be read.
	//
	// reqInfo contains additional information that could be useful to the implementation.
	//
	// ctx is the context to be used for the function call (use e.g. a context.WithCancel(...) to
	// have the option of cancelling operations that support this feature, such as a read
	// operations that has ctx passed as an argument)
	//
	// On success, the function returns:
	//
	//   - (<item count >= 0>, -1, nil) if all available observations in [t1, t2> were successfully
	//     retrieved for all time series in tsSeq/hdrs, and not exceeding a positive itemLimit
	//
	//   - (-1, -1, nil) if a positive itemLimit was exceeded
	//
	// On failure, the function returns (..., HTTP status code, error)
	ReadMultiTS(
		tstype string, tsSeq *timeseries.InstanceSeq, hdrs []timeseries.Header, t1, t2 int64,
		obsBodyModify func(*timeseries.TimeSeries, time.Time, *map[string]any) (
			int, error),
		obsFilter func(*timeseries.TimeSeries, time.Time, map[string]any) (
			bool, int, error),
		latestLimit, itemLimit int, obs *[][]dataset.Observation,
		reqInfo timeseries.RequestInfo, ctx context.Context) (int, int, error)

	// Write to the back-end a time series of observations (obs) into an existing time series of
	// type tstype and 'id' part matching the one in hdr.
	//
	// A non-nil element in ingestHookErrors (of the same size as obs) indicates that for the
	// corresponding observation, the ingest hook encountered an error that is not fixable by the
	// ingest client, but potentially at a later point internally (e.g. once some temporary system
	// failure has been fixed). In this case the error is passed to the storage back-end for
	// registration as appropriate (e.g. somehow associating the error with this observation in a
	// database in order to be dealt with later by a fixup routine).
	//
	// Returns (WriteSummary{...}, -1, nil) upon success, otherwise (..., HTTP status code, error).
	Write(
		tstype string, hdr timeseries.Header, obs *[]dataset.Observation,
		ingestHookErrors []error) (ObsWriteSummary, int, error)

	// Returns true iff the 'clear' operation is supported.
	// WARNING: a back-end used in production should normally make sure to return false!
	SupportsClear() bool

	// Clear deletes all data from the back-end.
	Clear() (int, error)

	// LoadTimeSeries loads currently available time series instances into the global
	// registry (obs.TSReg). An implementation may use optionalFeatures to avoid loading
	// time series for inactive ts types.
	// Any non-fatal errors will be recorded in nonFatalErrors.
	// Returns nil upon success, otherwise error.
	LoadTimeSeries(optionalFeatures common.StringSet, nonFatalErrors *[]error) error

	// Cleanup frees resources used by the back-end.
	Cleanup()

	// Prints the current contents of the back-end.
	Print()
}

// KeepLatestOnly removes from obs observations older than the limit newest ones.
// NOTE: the function assumes that limit >= 0 but will not check for this.
func KeepLatestOnly(obs *[][]dataset.Observation, limit int) {
	// assert(limit >= 0)
	for i, obs0 := range *obs {
		if len(obs0) > limit {
			// remove observations older than the limit newest ones
			(*obs)[i] = (*obs)[i][len(obs0)-limit:]
		}
	}
}

// ReadMultiTSAdapter ... TODO: document!
func ReadMultiTSAdapter(
	sbe StorageBackend, tstype string, tsSeq *timeseries.InstanceSeq, hdrs []timeseries.Header, t1,
	t2 int64,
	obsBodyModify func(*timeseries.TimeSeries, time.Time, *map[string]any) (int, error),
	obsFilter func(*timeseries.TimeSeries, time.Time, map[string]any) (bool, int, error),
	latestLimit, itemLimit int, obs *[][]dataset.Observation, reqInfo timeseries.RequestInfo) (
	int, int, error) {

	if itemLimit == 0 {
		return -1, http.StatusInternalServerError,
			fmt.Errorf("itemLimit must be either negative or positive")
	}

	var excess bool // exceeding itemLimit? (false by default)

	itemCount := 0   // total item count

	limit := itemLimit

	for i, hdr := range hdrs {

		if itemLimit > 0 { // 'limited' mode
			limit-- // subtract 1 to make room for the time series as such
			if limit < 1 {
				limit = 1 // ensure we satisfy the pre-condition
			}
		}

		var err error
		var statusCode int

		// read as much as possible
		excess, statusCode, err = sbe.ReadSingleTS(
			tstype, (*tsSeq)[i], hdr, t1, t2, obsBodyModify, obsFilter, limit, &(*obs)[i], reqInfo)
		if err != nil {
			return -1, statusCode, fmt.Errorf("sbe.ReadSingleTS() failed: %v", err)
		}
		nread := len((*obs)[i])
		if (itemLimit > 0) && (nread > limit) {
			return -1, http.StatusInternalServerError,
				fmt.Errorf(
					"sbe.ReadSingleTS(): (itemLimit (%d) > 0) && (nread (%d) > limit (%d))",
					itemLimit, nread, limit)
		}
		// assert((itemLimit < 0) || (nread <= limit))

		// check if we're filled up
		if excess {
			break
		}

		// update item count and limit
		obsCount := nread
		if (latestLimit >= 0) && (latestLimit < obsCount) {
			obsCount = latestLimit
		}
		if obsCount > 0 {
			addCount := obsCount + 1 // add 1 for the time series as such
			itemCount += addCount
			limit -= addCount
			// assert((itemLimit < 0) || (limit > 0))
		}
	}

	if latestLimit >= 0 {
		// remove superfluous observations from obs so that each time series contains
		// at most the latestLimit newest observations (i.e. those that have been
		// included in itemCount!)
		KeepLatestOnly(obs, latestLimit)
	}

	if excess {
		return -1, -1, nil
	}

	return itemCount, -1, nil
}
