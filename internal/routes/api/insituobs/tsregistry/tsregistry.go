// Package tsregistry implements a generic registry of time series common to all time series types.
package tsregistry

import (
	"encoding/json"
	"fmt"
	"log"
	"math"

	"gitlab.met.no/frost/frost/internal/common"
	timeseries "gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
)

// TimeSeriesTypeInfo contains the default instance and the currently available
// instances for a time series type T.
// The following representation invariant is assumed for a any given T:
//   - Default.Type() == T
//   - len(Instances) > 0 => (forall i in Instances: i.Type() == T)
type TimeSeriesTypeInfo struct {
	Default timeseries.TimeSeries
	// maps from (serialized) time series id to instances
	Instances map[string]*timeseries.TimeSeries
}

// TimeSeriesRegistry keeps currently available time series types and their instances.
type TimeSeriesRegistry map[string]TimeSeriesTypeInfo

// TSReg is the global time series registry.
var TSReg TimeSeriesRegistry

// tsEnabled contains the enabled time series types (i.e. specified in FEATURES env.var.)
var tsEnabled = common.StringSet{}

// DefaultTimeSeries finds the default time series for a time series type.
// Returns (default time series, nil) upon success, else (nil, error).
func DefaultTimeSeries(tstype string) (timeseries.TimeSeries, error) {
	if tsTypeInfo, ok := TSReg[tstype]; ok {
		return tsTypeInfo.Default, nil
	}
	return nil, fmt.Errorf("type %s not found in TSReg", tstype)
}

// Enable enables time series type tstype.
func Enable(tstype string) {
	tsEnabled.Set(tstype)
}

// EnabledTypeNames returns the names of the emabled time series types.
func EnabledTypeNames() common.StringSet {
	return tsEnabled
}

// arrayMaxSizeMap keeps the maximum array sizes for each time series type
var arrayMaxSizeMap = map[string]*map[string]int{}

// TypeNames returns the names of the currently supported time series types.
func TypeNames() []string {
	names := make([]string, len(TSReg))
	i := 0
	for k := range TSReg {
		names[i] = k
		i++
	}
	return names
}

// header ID distinct field values
var hidfv = map[string]map[string]map[any]struct{}{}

// Adds to header ID stats, i.e. for time series type tstype and header ID structure hid (assumed
// to be of type map[string]any), the value V of the field F is added to the set of
// distinct values for F (i.e. the set size increases by one upon encountering a new V). Eventually
// the overall structure will contain the set of distinct values for each field in the header ID
// of tstype.
func addToHeaderIDStats(tstype string, hid any) error {
	// find field/value combinations of this instance
	m, ok := hid.(map[string]any)
	if !ok {
		return fmt.Errorf("hid not a map[string]any: %v", hid)
	}

	// find (or create) structure for this time series type
	m0, found := hidfv[tstype]
	if !found {
		m0 = map[string]map[any]struct{}{}
		hidfv[tstype] = m0
	}

	// loop over field/value combinations of this instance
	for f, v := range m {

		// find (or create) structure for this field
		m1, found := m0[f]
		if !found {
			m1 = map[any]struct{}{}
			m0[f] = m1
		}
		m1[v] = struct{}{} // add field value to set
	}

	return nil
}

func PrintHeaderIDStats() {
	log.Printf("*** distinct header ID field values: ***\n")

	for tstype, m0 := range hidfv {
		log.Printf("    >>> %s:\n", tstype)
		for field, valueSet := range m0 {
			log.Printf("        %20s: %7d\n", field, len(valueSet))
		}
	}
}

func GetStatus(tstype string) any {

	getHeaderIDStats := func(tstype string) any {
		m0, found := hidfv[tstype]
		if !found {
			tstypes := []string{}
			for t := range hidfv {
				tstypes = append(tstypes, t)
			}
			return fmt.Sprintf(
				"no hdr/id stats found for time series type %s (available types: %s)",
				tstype, tstypes)
		}

		stats := map[string]int{}
		for field, valueSet := range m0 {
			stats[field] = len(valueSet)
		}

		return stats
	}

	stats := map[string]any{
		"hdr/id stats": getHeaderIDStats(tstype),
	}

	return stats
}

// AddTimeSeries attempts to create and add to the registry a time series instance of type tstype,
// base ID baseID, serialized header stshdr, serialized header components id and extra, optional
// fromTime, toTime, and restrictions.
//
// The base ID is assumed to be a more permanent/fundamental identifier for the time series, whereas
// the ID part of stshdr could in theory change over time (e.g. it could be modified to fix an
// error, but still be associated with the same base ID).
// NOTE: the base ID must be globally unique in the context of the storage back-end. If for example
// the storage back-end keeps open and restricted time series in separate sub-storages (typically
// databases), then which one to access must be derivable from the base ID.
//
// If both fromTime and toTime is math.MinInt64, it means that 1) these values are not in use,
// or 2) they are already present in the toplevel "available" object in stshdr. Otherwise, such an
// "available" object is inserted to the new instance with contents derived from one or both of
// these values (assumed to be number of seconds relative to 1970-01-01T00:00:00Z, i.e. negative
// values representing times *before* that date).
//
// Any non-fatal errors that will cause the time series not to be registered (typically
// resulting from invalid JSON schemas) will be recorded in nonFatalErrors.
//
// Upon success, the function returns either (time series instance (new or existing), nil) if
// both the 'id' and 'extra' part of the time series header are valid, or (nil, nil) otherwise.
//
// Upon error, the function returns (nil, error).
func AddTimeSeries(
	tstype, baseID, stshdr, id, extra string, fromTime, toTime int64, nonFatalErrors *[]error) (
	*timeseries.TimeSeries, error) {

	var err error

	// get default instance for time series type
	defaultTS, err := DefaultTimeSeries(tstype)
	if err != nil {
		return nil, fmt.Errorf("DefaultTimeSeries(%v) failed: %v", tstype, err)
	}

	// get header for the new instance
	var hdr timeseries.Header
	err = json.Unmarshal([]byte(stshdr), &hdr)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal(stshdr) failed: %v", err)
	}

	// do general validation (ensure that hdr consists of 'id' and 'extra')
	err = hdr.Validate()
	if err != nil {
		return nil, fmt.Errorf("hdr.Validate() failed: %v", err)
	}

	// convert all keys in hdr to lowercase
	err = hdr.ConvertKeysToLower()
	if err != nil {
		return nil, fmt.Errorf("hdr.ConvertKeysToLower() failed: %v", err)
	}

	// insert any fromTime and toTime into hdr to make it appear in the response
	if (fromTime != math.MinInt64) || (toTime != math.MinInt64) {
		m := map[string]any{}
		if fromTime != math.MinInt64 {
			m["from"] = common.UnixEpochToIso8601(fromTime)
		}
		if toTime != math.MinInt64 {
			m["to"] = common.UnixEpochToIso8601(toTime)
		}
		if (fromTime != math.MinInt64) && (toTime != math.MinInt64) {
			if fromTime > toTime {
				return nil, fmt.Errorf("fromTime (%d) > toTime (%d)", fromTime, toTime)
			}
		}
		hdr["available"] = m
	}

	// --- BEGIN ensure id is serialized alphabetically for consistency --------------------
	var mid map[string]any
	err = json.Unmarshal([]byte(id), &mid)
	if err != nil {
		return nil,
			fmt.Errorf("json.Unmarshal() failed (for alphabetical serialization of id): %v", err)
	}
	bid, err := json.Marshal(mid)
	if err != nil {
		return nil,
			fmt.Errorf("json.Marshal() failed (for alphabetical serialization of id): %v", err)
	}
	id = string(bid)
	// --- END ensure id is serialized alphabetically for consistency --------------------

	// create a new timeseries.TimeSeries instance from the header
	tsNew, err := defaultTS.CreateInstance(
		timeseries.BaseID(baseID), hdr, id, extra,
		timeseries.FromTime(fromTime), timeseries.ToTime(toTime))
	if err != nil {
		return nil, fmt.Errorf(
			"defaultTS.CreateInstance() failed (tstype: %s): %v", tstype, err)
	}

	tsExisting, ok := TSReg[tstype].Instances[id]
	if ok {
		return tsExisting, nil // instance already registered
	}

	// finalize the instance (e.g. add it to tstype-specific indexes and registries etc.)
	nfErr, err := defaultTS.FinalizeInstance(
		tsNew, timeseries.BaseID(baseID), hdr, id, extra, timeseries.FromTime(fromTime),
		timeseries.ToTime(toTime))
	if err != nil {
		return nil, fmt.Errorf("defaultTS.FinalizeInstance() failed: %v", err)
	}

	if nfErr != nil {
		*nonFatalErrors = append(*nonFatalErrors, nfErr)
	} else { // only add to main registry if there were no errors (including non-fatal ones)
		TSReg[tstype].Instances[id] = tsNew // add to main registry
	}

	// add to header ID stats
	err = addToHeaderIDStats(tstype, hdr["id"])
	if err != nil {
		return nil, fmt.Errorf("addToHeaderIDStats() failed: %v", err)
	}

	return tsNew, nil
}

// RemoveTimeSeries removes from the registry a time series instance of type tstype and
// (serialized) header stshdr.
// Returns nil upon success, otherwise error.
func RemoveTimeSeries(tstype, stshdr string) error {
	var err error

	// ensure the time series type exists
	_, err = DefaultTimeSeries(tstype)
	if err != nil {
		return fmt.Errorf("no such time series type: %s: %v", tstype, err)
	}

	// get header for the instance to be removed
	var hdr timeseries.Header
	err = json.Unmarshal([]byte(stshdr), &hdr)
	if err != nil {
		return fmt.Errorf("json.Unmarshal(stshdr) failed: %v", err)
	}
	err = hdr.Validate()
	if err != nil {
		return fmt.Errorf("hdr.Validate() failed: %v", err)
	}

	// get serialized id part of header
	bstsid, err := json.Marshal(hdr["id"])
	if err != nil {
		return fmt.Errorf("json.Marshal(id) failed: %v", err)
	}

	stsid := string(bstsid)

	if _, found := TSReg[tstype].Instances[stsid]; found {
		// TODO: defaultTS.RemoveInstance(...), i.e. so that e.g. the
		// lardbase and lardranked ts types can remove the instance from internal
		// indexes and registries!
		_ = found
	}
	delete(TSReg[tstype].Instances, stsid) // remove instance from main registry

	return nil
}

// UpdateTimeSeries updates the 'extra' part of a time series instance in the registry
// that is of type tstype and has an 'id' part that matches the one in (serialized) header stshdr.
// Returns nil upon success, otherwise error.
func UpdateTimeSeries(tstype, stshdr string) error {
	var err error

	// ensure the time series type exists
	_, err = DefaultTimeSeries(tstype)
	if err != nil {
		return fmt.Errorf("no such time series type: %s: %v", tstype, err)
	}

	// get header for the instance to be updated
	var hdr timeseries.Header
	err = json.Unmarshal([]byte(stshdr), &hdr)
	if err != nil {
		return fmt.Errorf("json.Unmarshal(stshdr) failed: %v", err)
	}
	err = hdr.Validate()
	if err != nil {
		return fmt.Errorf("hdr.Validate() failed: %v", err)
	}

	// get serialized 'id' part of header
	bstsid, err := json.Marshal(hdr["id"])
	if err != nil {
		return fmt.Errorf("json.Marshal(id) failed: %v", err)
	}

	// get time series instance
	ts, ok := TSReg[tstype].Instances[string(bstsid)]
	if !ok {
		return fmt.Errorf("time series not found (tstype: %s): %v", tstype, hdr["id"])
	}

	// get serialized 'extra' part of header
	bstsextra, err := json.Marshal(hdr["extra"])
	if err != nil {
		return fmt.Errorf("json.Marshal(extra) failed: %v", err)
	}

	// update in time series instance
	err = (*ts).UpdateExtra(string(bstsextra))
	if err != nil {
		return fmt.Errorf("(*ts).UpdateExtra() failed: %v", err)
	}

	return nil
}

// Clear clears the registry (but keeps the default time series types).
func Clear() {
	tsrNew := TimeSeriesRegistry{}
	for key, val := range TSReg {
		tsrNew[key] = TimeSeriesTypeInfo{
			Default:   val.Default,
			Instances: make(map[string]*timeseries.TimeSeries),
		}

		// clear any type-specific structures
		val.Default.Clear()
	}
	TSReg = tsrNew
}

// TimeSeriesExists returns (true, "") if tstype/tsid exists in the registry,
// otherwise (false, reason).
func TimeSeriesExists(tstype, tsid string) (bool, string) {
	var found bool
	tsTypeInfo, found := TSReg[tstype]
	if !found {
		return false, fmt.Sprintf("time series type not found: %s", tstype)
	}

	_, found = tsTypeInfo.Instances[tsid]
	if !found {
		return false, fmt.Sprintf("time series id not found: %s", tsid)
	}

	return true, ""
}
