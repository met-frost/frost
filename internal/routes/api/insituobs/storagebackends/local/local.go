// Package obssbelocal ... TO BE DOCUMENTED.
package obssbelocal

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"gitlab.met.no/frost/frost/internal/common"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	storagebackends "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/tsregistry"
)

// Definition of a data structure for keeping observations and 'extra' part for a set
// of time series.
type time2obs map[int64]dataset.Observation // maps from time (Unix epoch UTC secs) to observation
type tsinfo struct {
	tm2obs time2obs
	extra  string // the 'extra' part as a serialized JSON structure
}

// Local is an implementation of the StorageBackend interface that keeps data in local memory.
type Local struct {
	tsmap map[string]tsinfo // maps from time series absolute id to tsinfo
	// (the absolute id is the serialized combination of type (string) and instance id
	// (JSON structure))
}

// NewLocal creates and initializes a new instance of Local.
// Returns (the initialized instance, nil) on success, otherwise (nil, error).
func NewLocal() (*Local, error) {
	// create and return backend struct
	sb := new(Local)
	sb.tsmap = make(map[string]tsinfo)
	return sb, nil
}

// Description ... (see documentation in StorageBackend interface)
func (sbe *Local) Description() string {
	return "local in-memory structure"
}

// CreateTimeSeries ... (see documentation in StorageBackend interface)
func (sbe *Local) CreateTimeSeries(tstype string, hdr timeseries.Header) (string, int, error) {
	var err error

	bstsid, err := json.Marshal(hdr["id"])
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("json.Marshal(id) failed: %v", err)
	}
	stsid := string(bstsid)
	stsaid := fmt.Sprintf(`{"type": "%s", "id": %s}`, tstype, stsid)

	if _, found := sbe.tsmap[stsaid]; found {
		return stsid, -1, nil // return successfully since time series already exists in sbe.tsmap
	}

	// add time series to sbe.tsmap

	bstsextra, err := json.Marshal(hdr["extra"])
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("json.Marshal(extra) failed: %v", err)
	}

	sbe.tsmap[stsaid] = tsinfo{
		tm2obs: make(time2obs),
		extra:  string(bstsextra),
	}

	return stsid, -1, nil // time series successfully created
}

// RemoveTimeSeries ... (see documentation in StorageBackend interface)
func (sbe *Local) RemoveTimeSeries(tstype string, hdr timeseries.Header) (int, error) {
	bsid, err := json.Marshal(hdr["id"])
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("json.Marshal(id) failed: %v", err)
	}
	stsaid := fmt.Sprintf(`{"type": "%s", "id": %s}`, tstype, string(bsid))

	if _, found := sbe.tsmap[stsaid]; !found {
		return -1, nil // return successfully since time series already exists in sbe.tsmap
	}

	// remove any existing time series from sbe.tsmap
	delete(sbe.tsmap, stsaid)
	return -1, nil // time series either successfully removed or didn't exist in the first place
}

// UpdateTimeSeries ... (see documentation in StorageBackend interface)
func (sbe *Local) UpdateTimeSeries(tstype string, hdr timeseries.Header) (int, error) {
	return http.StatusNotImplemented, fmt.Errorf(
		"UpdateTimeSeries() not implemented for Local storage backend")
}

// ReadSingleTS ... (see documentation in StorageBackend interface)
func (sbe *Local) ReadSingleTS(
	tstype string, ts0 *timeseries.TimeSeries, hdr timeseries.Header, t1, t2 int64,
	obsBodyModify func(*timeseries.TimeSeries, time.Time, *map[string]interface{}) (int, error),
	obsFilter func(*timeseries.TimeSeries, time.Time, map[string]interface{}) (bool, int, error),
	limit int, observations *[]dataset.Observation, reqInfo timeseries.RequestInfo) (
	bool, int, error) {

	bstsid, err := json.Marshal(hdr["id"])
	if err != nil {
		return false, http.StatusInternalServerError, fmt.Errorf("json.Marshal(id) failed: %v", err)
	}

	stsaid := fmt.Sprintf(`{"type": "%s", "id": %s}`, tstype, string(bstsid))

	found := false
	var k string
	var tsi tsinfo
	for k, tsi = range sbe.tsmap {
		if strings.EqualFold(stsaid, k) {
			found = true
			break
		}
	}
	if !found {
		return false, -1, nil // no observations exist in this time series
	}

	// at least one observation exists in this time series

	tm2obs := tsi.tm2obs
	var times []int64
	for t := range tm2obs {
		times = append(times, t)
	}
	slices.Sort(times)

	// try to read as many observations values within [t1, t2> as possible
	// (if limit >= 0, we will read at most limit + 1 values)

	*observations = nil // remove any existing observations

	var excess bool // exceeding limit? (false by default)

	for _, t := range times {
		if t1 <= t { // WARNING: scanning to start of valid time interval is O(n)!
			if t2 <= t { // beyond valid time interval, so stop scanning
				break
			}

			v := tm2obs[t] // read value in valid time interval

			// apply obs body modifier
			statusCode, err := obsBodyModify(ts0, *v.Time, &v.Body)
			if err != nil {
				return false, statusCode, fmt.Errorf("obsBodyModify() failed: %v", err)
			}

			if (limit < 0) || (len(*observations) < limit) { // still room for more

				// apply obs filter
				obsFilterPassed, statusCode, err := obsFilter(ts0, *v.Time, v.Body)
				if err != nil {
					return false, statusCode, fmt.Errorf("obsFilter() failed: %v", err)
				}

				if obsFilterPassed { // obs passed the obs filter, so add it
					*observations = append(*observations, v)
				}

			} else {
				//assert(len(*observations) == limit)
				// no more room, but indicate that there is now at least one more value in [t1,t2>
				excess = true
				break
			}
		}
	}

	return excess, -1, nil // zero or more values successfully retrieved
}

// ReadMultiTS ... (see documentation in StorageBackend interface)
func (sbe *Local) ReadMultiTS(
	tstype string, tsSeq *timeseries.InstanceSeq, hdrs []timeseries.Header, t1, t2 int64,
	obsBodyModify func(*timeseries.TimeSeries, time.Time, *map[string]interface{}) (int, error),
	obsFilter func(*timeseries.TimeSeries, time.Time, map[string]interface{}) (bool, int, error),
	latestLimit, itemLimit int, obs *[][]dataset.Observation,
	reqInfo timeseries.RequestInfo, ctx context.Context) (int, int, error) {

	// for now delegate to ReadMultiTSAdapter; TODO: consider if it would be faster to
	// implement by accessing sbe.tsmap directly
	return storagebackends.ReadMultiTSAdapter(
		sbe, tstype, tsSeq, hdrs, t1, t2, obsBodyModify, obsFilter, latestLimit, itemLimit, obs,
		reqInfo)
}

// Write ... (see documentation in StorageBackend interface)
func (sbe *Local) Write(
	tstype string, hdr timeseries.Header, observations *[]dataset.Observation,
	ingestHookErrors []error) (storagebackends.ObsWriteSummary, int, error) {

	bstsid, err := json.Marshal(hdr["id"])
	if err != nil {
		return storagebackends.ObsWriteSummary{}, http.StatusInternalServerError,
			fmt.Errorf("json.Marshal(id) failed: %v", err)
	}

	// ensure that the time series already exists in the registry
	if exists, reason := tsregistry.TimeSeriesExists(tstype, string(bstsid)); !exists {
		return storagebackends.ObsWriteSummary{}, http.StatusInternalServerError,
			fmt.Errorf("time series not found in internal registry: %s", reason)
	}

	stsaid := fmt.Sprintf(`{"type": "%s", "id": %s}`, tstype, string(bstsid))

	// look up time series in sbe.tsmap
	tsi, found := sbe.tsmap[stsaid]
	if !found {
		return storagebackends.ObsWriteSummary{}, http.StatusBadRequest,
			fmt.Errorf("time series not found in sbe.tsmap (stsaid: %s)", stsaid)
	}

	// delete/insert/update observations
	summary := storagebackends.ObsWriteSummary{}
	for _, obs := range *observations {
		t := obs.Time.UTC().Unix()
		_, existed := tsi.tm2obs[t]
		if obs.Body == nil {
			delete(tsi.tm2obs, t) // delete obs at t (no-op if no such obs existed)
			if existed {
				summary.Deleted++
			}
		} else {
			tsi.tm2obs[t] = obs // insert or update obs at t
			if existed {
				summary.Updated++
			} else {
				summary.Inserted++
			}
		}
	}

	return summary, -1, nil // all observations successfully inserted/updated
}

// SupportsClear ... (see documentation in StorageBackend interface)
func (sbe *Local) SupportsClear() bool {
	return true // always considered safe (i.e. assuming storage backend 'local'
	// is never used for real production)
}

// Clear ... (see documentation in StorageBackend interface)
func (sbe *Local) Clear() (int, error) {
	sbe.tsmap = make(map[string]tsinfo)
	return -1, nil
}

// LoadTimeSeries ... (see documentation in StorageBackend interface)
func (sbe *Local) LoadTimeSeries(
	optionalFeatures common.StringSet, nonFatalErrors *[]error) error {

	for tsaid := range sbe.tsmap {
		var err error

		var aid struct {
			TSType string
			ID     string
		}
		err = json.Unmarshal([]byte(tsaid), &aid)
		if err != nil {
			return fmt.Errorf("json.Unmarshal() failed: %v", err)
		}

		if optionalFeatures.ContainsMatch(aid.TSType) { // only load active ts types

			//stshdr := fmt.Sprintf(`{"id": %s, "extra": %s}`, aid.id, tsi.extra)
			stshdr := fmt.Sprintf(`{"id": %s,}`, aid.ID)
			// convert object keys to lowercase
			stshdr0, err := common.ConvertObjKeysToLower2(stshdr)
			if err != nil {
				return fmt.Errorf("common.ConvertObjKeysToLower2() failed: %v", err)
			}

			// just set fromTime and toTime as "not in use" for now (TODO?)
			fromTime := int64(math.MinInt64)
			toTime := int64(math.MinInt64)

			//_, err = tsregistry.AddTimeSeries(
			//	aid.tstype, aid.id, stshdr0, aid.id, tsi.extra, fromTime, toTime, "",
			//	nonFatalErrors)
			_, err = tsregistry.AddTimeSeries(
				aid.TSType, aid.ID, stshdr0, aid.ID, "", fromTime, toTime, nonFatalErrors)
			if err != nil {
				return fmt.Errorf("tsregistry.AddTimeSeries() failed: %v", err)
			}
		}
	}

	return nil
}

// Cleanup ... (see documentation in StorageBackend interface)
func (sbe *Local) Cleanup() {
	// nothing to clean up in this case
}

// Print ... (see documentation in StorageBackend interface)
func (sbe *Local) Print() {
	fmt.Printf("\ncurrent contents of the local storage backend:\n")
	fmt.Printf("  sbe: %v\n", sbe)
	fmt.Printf("  sbe.tsmap: %v\n", sbe.tsmap)
}
