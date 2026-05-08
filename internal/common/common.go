// Package common contains generally useful functions.
package common

// This file contains commonly used functions.

import (
	"bytes"
	"compress/gzip"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xeipuuv/gojsonschema"
)

var (
	reWhitespace *regexp.Regexp
	reWKTPolygon *regexp.Regexp
	reWKTPoint   *regexp.Regexp
)

func init() {
	reWhitespace = regexp.MustCompile(`\s+`)
	reWKTPolygon = regexp.MustCompile(`POLYGON\s*\(\s*\((.*)\s*\)\s*\)`)
	reWKTPoint = regexp.MustCompile(`POINT\s*\(\s*(\S*)\s*(\S*)\s*\)`)
}

// NormalizeWhitespace returns a version of s that has leading and trailing whitespace removed
// and each internal whitespace sequence replaced with a single space character.
// (NOTE: 'whitespace' is here defined as a character that matches the regexp \s metacharacter
// (as defined for example here: https://github.com/google/re2/wiki/Syntax))
func NormalizeWhitespace(s string) string {
	return strings.TrimSpace(reWhitespace.ReplaceAllString(s, " "))
}

// GetWKTGeoPolygon checks if a WKT polygon is valid. If lons and lats are both non-nil, the
// coordinates are copied into those arrays.
//
// Returns nil on success, otherwise error.
func GetWKTGeoPolygon(wkt string, lons *[]float64, lats *[]float64) error {

	matches := reWKTPolygon.FindStringSubmatch(wkt)

	if len(matches) != 2 {
		return fmt.Errorf("invalid WKT polygon: overall mismatch")
	}

	coords := strings.Split(matches[1], ",")

	if len(coords) < 4 {
		return fmt.Errorf(
			"invalid WKT polygon: too few points: %d (expected at least 4)", len(coords))
	}

	if (lons != nil) && (lats != nil) {
		// reset lons and lats
		*lons = make([]float64, len(coords))
		*lats = make([]float64, len(coords))
	}

	// validate coordinates and (possibly) copy them into lons and lats
	var vals0 = [2]float64{} // first coordinate
	for ci, coord := range coords {
		coord0 := NormalizeWhitespace(coord)

		comps := []string{}
		if coord0 != "" {
			comps = strings.Split(coord0, " ")
		}
		if len(comps) != 2 {
			return fmt.Errorf(
				"invalid WKT polygon: found %d component(s) in coordinate %d, expected 2: %s",
				len(comps), ci, comps)
		}

		var vals = [2]float64{}
		for i := range 2 {
			var err error
			if vals[i], err = strconv.ParseFloat(comps[i], 64); err != nil {
				return fmt.Errorf(
					"invalid WKT polygon: failed to parse %s as a float64 in coordinate %d",
					comps[i], ci)
			}
		}

		switch {
		case ci == 0:
			// save first coordinate
			vals0[0] = vals[0]
			vals0[1] = vals[1]
		case ci == len(coords)-1:
			// ensure first and last coordinate are equal
			if (vals0[0] != vals[0]) || (vals0[1] != vals[1]) {
				return fmt.Errorf(
					"invalid WKT polygon: first and last coordinate differ: (%f, %f) != (%f, %f)",
					vals0[0], vals0[1], vals[0], vals[1])
			}
		}

		if (lons != nil) && (lats != nil) {
			// copy to lons and lats
			(*lons)[ci] = vals[0]
			(*lats)[ci] = vals[1]
		}
	}

	return nil
}

// GetWKTGeoPoint checks if a WKT point is valid.
//
// Returns (lon, lat, nil) on success, otherwise (..., ..., error).
func GetWKTGeoPoint(wkt string) (float64, float64, error) {

	matches := reWKTPoint.FindStringSubmatch(wkt)

	if len(matches) != 3 {
		return -1, -1, fmt.Errorf("invalid WKT point: overall mismatch")
	}

	lon, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return -1, -1, fmt.Errorf(
			"invalid WKT point: failed to parse longitude %s as a float64: %v", err)
	}

	lat, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return -1, -1, fmt.Errorf(
			"invalid WKT point: failed to parse latitude %s as a float64: %v", err)
	}

	return lon, lat, nil
}

// MatchesAsteriskPattern returns true iff word matches pattern assuming any asterisk in pattern
// represents an arbitrary sequence of zero or more characters.
func MatchesAsteriskPattern(word, pattern string) bool {
	// ### WARNING: In order to be able to use the fast and convenient filepath.Match() for
	// glob matching of general strings (and not only file paths), we must assume the slash
	// replacement string never occurs in the input
	// (i.e. assert((!strings.Contains(word, slashReplacement)) &&
	// (!strings.Contains(pattern, slashReplacement)))), otherwise we may get a false match.
	slashReplacement := "SLASH" // assume this never occurs anywhere in neither word nor pattern!
	wordR := strings.ReplaceAll(word, "/", slashReplacement)
	patternR := strings.ReplaceAll(pattern, "/", slashReplacement)
	match, err := filepath.Match(patternR, wordR)
	return match && (err == nil)
}

// Getenv returns the value of an environment variable or a default value if
// no such environment variable has been set.
func Getenv(key string, defaultValue string) string {
	var value string
	var ok bool
	if value, ok = os.LookupEnv(key); !ok {
		value = defaultValue
	}
	return value
}

// Warning: the below layout needs to have that value exactly, otherwise it won't work!
// See https://www.pauladamsmith.com/blog/2011/05/go_time.html
const iso8601layout = "2006-01-02t15:04:05z" // note lower case!

// Iso8601ToTime converts time string ts of the form YYYY-MM-DDThh:mm:ssZ to time.Time.
// Returns (val, nil) upon success, otherwise (time.Time{}, error).
func Iso8601ToTime(ts string) (time.Time, error) {
	tm, err := Iso8601ToTimeForLayout(ts, iso8601layout)
	if err != nil {
		return time.Time{}, fmt.Errorf("Iso8601ToTimeForLayout() failed for %s: %v", ts, err)
	}
	return tm, nil
}

// Iso8601ToTimeForLayout converts time string ts of a given layout to time.Time.
// Returns (val, nil) upon success, otherwise (time.Time{}, error).
func Iso8601ToTimeForLayout(ts, layout string) (time.Time, error) {
	tm, err := time.Parse(layout, strings.ToLower(ts))
	if err != nil {
		return time.Time{}, fmt.Errorf("time.Parse() failed for %s: %v", ts, err)
	}
	return tm, nil
}

// Iso8601ToUnixEpoch converts time string ts of the form YYYY-MM-DDThh:mm:ssZ to number of seconds
// since January 1st 1970 UTC. The value may be negative.
// Returns (val, nil) upon success, otherwise (-1, error).
func Iso8601ToUnixEpoch(ts string) (secs int64, err error) {
	tm, err := Iso8601ToTime(ts)
	if err != nil {
		return -1, fmt.Errorf("Iso8601ToTime() failed for %s: %v", ts, err)
	}
	return tm.Unix(), nil
}

// Iso8601ToUnixEpochForLayout converts time string ts of a given layout to number of seconds since
// January 1st 1970 UTC, ignoring any fractional second. The value may be negative.
// Returns (val, nil) upon success, otherwise (-1, error).
func Iso8601ToUnixEpochForLayout(ts, layout string) (secs int64, err error) {
	tm, err := Iso8601ToTimeForLayout(ts, layout)
	if err != nil {
		return -1, fmt.Errorf("Iso8601ToTimeForLayout() failed for %s: %v", ts, err)
	}
	return tm.Unix(), nil
}

// UnixEpochToIso8601 converts secs, assumed to be (possibly negative) number of seconds since
// January 1st 1970 UTC, to a time string of the form YYYY-MM-DDThh:mm:ssZ.
// Returns the ISO 8601 value.
func UnixEpochToIso8601(secs int64) (ts string) {
	tm := time.Unix(secs, 0).UTC()
	return strings.ToUpper(tm.Format(iso8601layout))
}

// ConvertTimeRange converts trange into two times, t1 and t2, assuming trange is of the form
// ts1/ts2 where each of ts1 and ts2 is of the form YYYY-MM-DDThh:mm:ssZ and ts1 <= ts2.
// Returns (t1, t2, nil) upon success, where t1 is utcEpochSecs(ts1) and ditto for t2/ts2.
// Returns (nil, nil, error) upon failure.
// ### may be obsoleted by new implementation of getTimeSpecification() in get.go.
func ConvertTimeRange(trange string) (t1, t2 int64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unexpected panic occurred in convertTimeRange()")
			t1, t2 = -1, -1
		}
	}()
	ptn := regexp.MustCompile(`^(?P<t1>.*)/(?P<t2>.*)$`) // could cause panic, hence above recover()

	matches := ptn.FindAllStringSubmatch(trange, -1)
	if matches == nil {
		return -1, -1, fmt.Errorf("failed to parse %s as a valid time range [1]", trange)
	}
	if (len(matches) != 1) || (len(matches[0]) != 3) {
		return -1, -1, fmt.Errorf("failed to parse %s as a valid time range [2]", trange)
	}
	ts1 := matches[0][1]
	ts2 := matches[0][2]

	t1, err = Iso8601ToUnixEpoch(ts1)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to convert %s to UTC epoch secs: %v", ts1, err)
	}

	t2, err = Iso8601ToUnixEpoch(ts2)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to convert %s to UTC epoch secs: %v", ts2, err)
	}

	if t1 > t2 {
		return -1, -1, fmt.Errorf("negative time interval not allowed")
	}

	return t1, t2, nil
}

// GetFirstQueryParam gets the first occurrence of a given query parameter in the query string.
// Returns (val, true) if found, otherwise ("", false).
func GetFirstQueryParam(queryParams url.Values, key string) (string, bool) {
	s, found := queryParams[key]
	if found {
		return s[0], true
	}
	return "", false
}

// GetDistinctQPValuesLC adds to qpVals the distinct, lowercase values of query parameter qpName
// found in queryParams.
func GetDistinctQPValuesLC(qpVals StringSet, queryParams url.Values, qpName string) {
	for _, qpVals0 := range queryParams[qpName] {
		for _, qpVal0 := range ExtractCSVValsLC(qpVals0) {
			qpVals[qpVal0] = struct{}{}
		}
	}
}

// GetDistinctQPValuesLC2 adds to qpVals the distinct, lowercase values of query parameter qpName
// found in queryParams.
func GetDistinctQPValuesLC2(qpVals StringSet, queryParams map[string]string, qpName string) {
	for _, qpVal0 := range ExtractCSVValsLC(queryParams[qpName]) {
		qpVals[qpVal0] = struct{}{}
	}
}

// GetFloat64FromQueryParam attempts to extract a single float64 value from a query parameter.
// Returns (float64, true, nil) upon success, (..., false, nil) if query parameter is not
// specified at all, or (..., ..., error) upon error (multiple occurrences specified or single
// occurrence not a float64).
func GetFloat64FromQueryParam(queryParams url.Values, key string) (float64, bool, error) {
	svals := queryParams[key]
	if len(svals) > 0 {
		if len(svals) == 1 {
			val, err := strconv.ParseFloat(strings.TrimSpace(svals[0]), 64)
			if err != nil {
				return 0, false, fmt.Errorf(
					"failed to parse query parameter '%s' as a float64: %s", key, svals[0])
			}
			return val, true, nil // single float64 value found
		}
		return -1, false, fmt.Errorf("multiple occurrences of query parameter '%s'", key)
	}
	return -1, false, nil // single float64 value not found
}

// ExtractCSVIntVals extracts []int{i1, i2, ...} from "i1,i2,...".
//
// Returns ([]int{...}, true) on success, otherwise (..., false).
func ExtractCSVIntVals(s string) ([]int, bool) {

	if strings.TrimSpace(s) == "" {
		return []int{}, true
	}

	s1 := strings.Split(s, ",")
	res := []int{}
	for _, s2 := range s1 {
		i, err := strconv.Atoi(strings.TrimSpace(s2))
		if err != nil {
			return []int{}, false
		}
		res = append(res, i)
	}
	return res, true
}

// ExtractCSVInt64Vals extracts []int64{i1, i2, ...} from "i1,i2,...".
//
// Returns ([]int64{...}, true) on success, otherwise (..., false).
func ExtractCSVInt64Vals(s string) ([]int64, bool) {

	if strings.TrimSpace(s) == "" {
		return []int64{}, true
	}

	s1 := strings.Split(s, ",")
	res := []int64{}
	for _, s2 := range s1 {
		i, err := strconv.ParseInt(s2, 10, 64)
		if err != nil {
			return []int64{}, false
		}
		res = append(res, i)
	}
	return res, true
}

type IntSet map[int]struct{}

// Set adds value v to iset.
func (iset *IntSet) Set(v int) {
	if iset != nil {
		(*iset)[v] = struct{}{}
	}
}

// IntSetFromList creates a new IntSet from ilist.
func IntSetFromList(ilist []int) IntSet {
	iset := IntSet{}
	for _, val := range ilist {
		iset.Set(val)
	}
	return iset
}

// Contains returns true iff iset is non-nil and contains i.
func (iset *IntSet) Contains(i int) bool {
	if iset == nil {
		return false
	}
	_, found := (*iset)[i]
	return found
}

// GetValues returns the values of iset (technically the map keys!) sorted in increasing order.
func (iset *IntSet) GetValues() []int {
	keys := []int{}
	for k := range *iset {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// Intersect returns the intersection of iset and iset2.
func (iset *IntSet) Intersect(iset2 *IntSet) *IntSet {
	isct := IntSet{}
	for k := range *iset {
		if iset2.Contains(k) {
			isct.Set(k)
		}
	}
	return &isct
}

// Diff returns the difference between iset and iset2, i.e. items in iset that are not in iset2.
func (iset *IntSet) Diff(iset2 *IntSet) *IntSet {
	diff := IntSet{}
	for k := range *iset {
		if !iset2.Contains(k) {
			diff.Set(k)
		}
	}
	return &diff
}

// GetUniqueIntValues returns v with duplicates removed.
func GetUniqueIntValues(v []int) []int {
	set := IntSetFromList(v)
	return set.GetValues()
}

// ToCSVString returns iset as a CSV string.
func (iset *IntSet) ToCSVString() string {
	return strings.Trim(strings.Join(strings.Fields(fmt.Sprint(iset.GetValues())), ","), "[]")
}

// IntsAsCSVString is a convenience wrapper for IntSet.ToCSVString.
func IntsAsCSVString(i []int) string {
	x := IntSetFromList(i)
	return x.ToCSVString()
}

// ExtractCSVIntValsToSet returns IntSet{j1, j2, ...} from "i1,i2,..."
// where j1, j2, ... is the set of distinct values among i1, i2, ...
// Returns (int set, true) upon success, otherwise (..., false).
func ExtractCSVIntValsToSet(s string) (IntSet, bool) {
	s1 := strings.Split(s, ",")
	res := IntSet{}
	for _, s2 := range s1 {
		i, err := strconv.Atoi(strings.TrimSpace(s2))
		if err != nil {
			return IntSet{}, false
		}
		res[i] = struct{}{}
	}
	return res, true
}

// ExtractCSVVals returns {"v1", "v2", ...} from "v1 ,v2  , ...".
// Returns a list of all trimmed and non-empty values from the CSV string.
func ExtractCSVVals(s string) []string {
	s1 := strings.Split(s, ",")
	res := []string{}
	for _, s2 := range s1 {
		s3 := strings.TrimSpace(s2)
		if s3 != "" {
			res = append(res, s3)
		}
	}
	return res
}

// ExtractCSVValsLC returns {lowercase("v1"), lowercase("v2"), ...} from "v1 ,v2  , ...".
// Returns a list of all trimmed, lowercase and non-empty values from the CSV string.
func ExtractCSVValsLC(s string) []string {
	s1 := strings.Split(s, ",")
	res := []string{}
	for _, s2 := range s1 {
		s3 := strings.TrimSpace(s2)
		if s3 != "" {
			res = append(res, strings.ToLower(s3))
		}
	}
	return res
}

type StringSet map[string]struct{}

// Contains returns true iff sset is non-nil and contains s.
func (sset *StringSet) Contains(s string) bool {
	if sset == nil {
		return false
	}
	_, found := (*sset)[s]
	return found
}

// ContainsMatch returns true iff sset is non-nil and contains an item s0 that matches s where
// s0 is allowed to have asterisks to match zero or more characters.
func (sset *StringSet) ContainsMatch(s string) bool {
	if sset == nil {
		return false
	}
	for s0 := range *sset {
		if MatchesAsteriskPattern(s, s0) {
			return true
		}
	}
	return false
}

// ContainsMatchInSet returns true iff sset is non-nil and contains an item s0 that matches any
// s1 in sset1 where s0 is allowed to have asterisks to match zero or more characters.
func (sset *StringSet) ContainsMatchInSet(sset1 StringSet) bool {
	if sset == nil {
		return false
	}
	for s0 := range *sset {
		for s1 := range sset1 {
			if MatchesAsteriskPattern(s1, s0) {
				return true
			}
		}
	}
	return false
}

// Set adds value v to sset.
func (sset *StringSet) Set(v string) {
	if sset != nil {
		(*sset)[v] = struct{}{}
	}
}

// ToList returns sset as a string list.
func (sset *StringSet) ToList() []string {
	slist := []string{}
	for key := range *sset {
		slist = append(slist, key)
	}
	return slist
}

// ToLower returns sset with contents converted to lowercase.
func (sset *StringSet) ToLower() StringSet {
	sset2 := StringSet{}
	for key := range *sset {
		sset2.Set(strings.ToLower(key))
	}
	return sset2
}

// StringSetFromList creates a new StringSet from slist.
func StringSetFromList(slist []string) StringSet {
	sset := StringSet{}
	for _, val := range slist {
		sset.Set(val)
	}
	return sset
}

// SetFromList adds the values in slist to sset.
func (sset *StringSet) SetFromList(slist []string) {
	for _, val := range slist {
		sset.Set(val)
	}
}

// ExtractStringSetFromCSVVals returns the values in CSV string s as a StringSet.
func ExtractStringSetFromCSVVals(s string) StringSet {
	sset := StringSet{}
	for _, s0 := range ExtractCSVVals(s) {
		sset[s0] = struct{}{}
	}
	return sset
}

type StringList []string

// Contains returns true iff slist is non-nil and contains s as a substring in at least one of
// the items.
func (slist StringList) Contains(s string) bool {
	if slist == nil {
		return false
	}
	for _, s0 := range slist {
		if strings.Contains(s0, s) {
			return true
		}
	}
	return false
}

// ContainsMatch matches s against slist. If slist is empty the function returns true iff matchEmpty
// is true. Otherwise, the function returns true iff slist contains an item s0 that matches s where
// s0 is allowed to have asterisks to match zero or more characters.
func (slist StringList) ContainsMatch(s string, matchEmpty bool) bool {
	if len(slist) == 0 {
		return matchEmpty
	}
	for _, s0 := range slist {
		if MatchesAsteriskPattern(s, s0) {
			return true
		}
	}
	return false
}




// FlattenQueryParams converts uv of the general, two-level CSV form
//
//	key: {"v1,v2,...", "vi,vi+1,...", ...}
//
// into the one-level form
//
//	key: {"v1", "v2", ..., "vi", "vi+1", ...}
//
// Query parameters specified in exceptions will not be converted.
func FlattenQueryParams(kv1 url.Values, exceptions []string) url.Values {
	kv2 := url.Values{}

	for key, vals1 := range kv1 {
		if StringInStrings(key, exceptions) { // leave unmodified
			kv2[key] = vals1
		} else { // replace with flattened version
			vals2 := []string{}
			for _, val1 := range vals1 {
				svals := ExtractCSVVals(val1)
				vals2 = append(vals2, svals...)
			}
			kv2[key] = vals2
		}
	}

	return kv2
}

// EnsureQueryParamsMutex returns an error if qp1 and qp2 both exist in queryParams, otherwise nil.
func EnsureQueryParamsMutex(queryParams url.Values, qp1, qp2 string) error {
	_, found1 := queryParams[qp1]
	_, found2 := queryParams[qp2]
	if found1 && found2 {
		return fmt.Errorf("query parameters '%s' and '%s' may not be combined", qp1, qp2)
	}
	return nil
}

// StringInStrings returns true iff slist contains s.
func StringInStrings(s string, slist []string) bool {
	for _, s2 := range slist {
		if s == s2 {
			return true
		}
	}
	return false
}

// StringMatchesAny returns (true, nil) if s matches at least one string in ss, otherwise
// (false, nil) or (false, error) if an error occurred.
// Matching is case-insensitive, space-trimmed (leading and trailing whitespace), and any item in
// ss may contain asterisks for basic wildcard matching (each asterisk matches zero or more
// characters). If emptyMatchesAll is true, an empty string in ss is equivalent to "*" while an
// empty ss is equivalent to []string{"*"}.
func StringMatchesAny(s string, ss []string, emptyMatchesAll bool) (bool, error) {
	if (len(ss) == 0) && emptyMatchesAll {
		ss = []string{"*"}
	}

	s0 := strings.ToLower(strings.TrimSpace(s))
	for _, ss0 := range ss {
		ss0 = strings.ToLower(strings.TrimSpace(ss0))
		if (ss0 == "") && emptyMatchesAll {
			ss0 = "*"
		}
		match, err := filepath.Match(ss0, s0)
		if err != nil {
			return false, fmt.Errorf("filepath.Match() failed: %v", err)
		}
		if match {
			return true, nil
		}
	}
	return false, nil
}

// StringMatchesAnyInSet is a convenience wrapper around StringMatchesAny which takes a
// StringSet instead of a []string. See documentation of StringMatchesAny.
func StringMatchesAnyInSet(s string, sset StringSet, emptyMatchesAll bool) (bool, error) {
	ss := []string{}
	for s0 := range sset {
		ss = append(ss, s0)
	}
	return StringMatchesAny(s, ss, emptyMatchesAll)
}

// SchemaValidateCore validates the JSON document doc against schema schemaLoader where doc is
// not assumed to require any marshalling.
//
// Returns nil if valid, otherwise an informative error.
func SchemaValidateCore(schemaLoader gojsonschema.JSONLoader, doc []byte) error {

	result, err := gojsonschema.Validate(schemaLoader, gojsonschema.NewStringLoader(string(doc)))
	if err != nil {
		return fmt.Errorf("gojsonschema.Validate() failed: %v", err)
	}

	if !result.Valid() {
		errs := "\n"
		for _, desc := range result.Errors() {
			errs += fmt.Sprintf("- %s\n", desc)
		}
		return errors.New(errs)
	}

	return nil // valid
}

// SchemaValidate is a convenience wrapper around SchemaValidateCore that first marshals doc into
// a []byte (see documentation of SchemaValidateCore).
func SchemaValidate(schemaLoader gojsonschema.JSONLoader, doc any) error {

	mdoc, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("json.Marshal() failed: %v", err)
	}

	err = SchemaValidateCore(schemaLoader, mdoc)
	if err != nil {
		return fmt.Errorf("SchemaValidateCore() failed: %v", err)
	}

	return nil // valid
}

// IsNonEmptyJSONObject returns true iff s is a serialized, non-empty JSON object.
func IsNonEmptyJSONObject(s string) bool {
	var m map[string]any
	err := json.Unmarshal([]byte(s), &m)
	if err != nil {
		return false
	}
	return len(m) > 0
}

// GenUUID generates and returns a new UUID (see https://tools.ietf.org/html/rfc4122).
func GenUUID() string {
	return uuid.NewString()
}

// GetHostAndPort extracts host and port from environment variable evar of the form "", <host>,
// or <host>:<port>, using defaultHost and defaultPort as defaults.
// Returns (host, port, nil) upon success, otherwise ("", -1, error).
func GetHostAndPort(evar, defaultHost string, defaultPort int) (string, int, error) {
	host := defaultHost
	port := defaultPort

	expr0 := strings.TrimSpace(Getenv(evar, ""))
	if expr0 != "" {
		var err error
		parts := strings.Split(expr0, ":")
		switch len(parts) {
		case 1:
			host = parts[0]
		case 2:
			host = parts[0]
			port, err = strconv.Atoi(parts[1])
			if (err != nil) || (port <= 0) {
				return "", -1, fmt.Errorf(
					"failed to extract port as a positive integer from environment "+
						"variable %s: %s", evar, expr0)
			}
		default:
			return "", -1,
				fmt.Errorf("environment variable %s not of form <host>:<port>: %s", evar, expr0)
		}
	}

	return host, port, nil
}

// ConvertObjKeysToLower converts to lowercase all keys in obj and sub-objects at all levels.
// Returns nil upon success, otherwise error.
func ConvertObjKeysToLower(obj *map[string]any) error {
	// check precondition
	if obj == nil {
		return fmt.Errorf("obj == nil")
	}

	// *** phase 1: fix keys at this level ***

	for k, v := range *obj {
		k0 := strings.ToLower(k) // get lowercase version of key
		if k == k0 {
			continue // already lowercase
		}

		// check if lowercase version already exists
		if _, found := (*obj)[k0]; found {
			return fmt.Errorf(
				"lowercase version of key %s already exists at the same level in the object/map", k)
		}

		// move association to lowercase version
		(*obj)[k0] = v
		delete(*obj, k)
	}

	// *** phase 2: fix keys at the next level ***

	for _, v := range *obj {
		if obj1, ok := v.(map[string]any); ok {
			// value is an object -> process directly
			err := ConvertObjKeysToLower(&obj1)
			if err != nil {
				return err
			}
		} else if arr1, ok := v.([]any); ok {
			// value is an array -> process all object items
			for _, item := range arr1 {
				if obj2, ok := item.(map[string]any); ok {
					err := ConvertObjKeysToLower(&obj2)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// ConvertObjKeysToLower2 is a wrapper around ConvertObjKeysToLower that takes a serialized
// map[string]any as argument.
// Returns (serialization of converted version, nil) upon success, otherwise (..., error).
func ConvertObjKeysToLower2(s string) (string, error) {
	var m map[string]any

	err := json.Unmarshal([]byte(s), &m)
	if err != nil {
		return "", fmt.Errorf("json.Unmarshal() failed: %v", err)
	}

	err = ConvertObjKeysToLower(&m)
	if err != nil {
		return "", fmt.Errorf("common.ConvertKeysToLower() failed: %v", err)
	}

	bs0, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("json.Marshal() failed: %v", err)
	}

	return string(bs0), nil
}

// ConvertIFToFloat64 attempts to convert x to a float64 value (ensured to be !NaN).
// Returns (float64 value, true) upon success, otherwise (..., false).
func ConvertIFToFloat64(x any) (float64, bool) {

	getCandidate := func() (float64, bool) {
		switch v := x.(type) {
		case float64:
			return v, true
		case float32:
			return float64(v), true
		case int64:
			return float64(v), true
		case int32:
			return float64(v), true
		case int16:
			return float64(v), true
		case int8:
			return float64(v), true
		case int:
			return float64(v), true
		case string:
			v0, err := strconv.ParseFloat(v, 64)
			if err == nil {
				return v0, true
			}
		}
		return float64(0), false // not convertible
	}

	f, ok := getCandidate()
	return f, ok && !math.IsNaN(f)
}

// ConvertIFToInt64 attempts to convert x to an int64 value.
// WARNING: if x is a float type, any decimal portion is truncated.
// Returns (int64 value, true) upon success, otherwise (..., false).
func ConvertIFToInt64(x any) (int64, bool) {
	f64, ok := ConvertIFToFloat64(x)
	if ok {
		return int64(f64), true // convert, but truncate any decimal portion
	}
	return int64(0), false // not convertible
}

// ConvertIFToInt attempts to convert x to an int value.
// WARNING: if x is a float type, any decimal portion is truncated.
// Returns (int value, true) upon success, otherwise (..., false).
func ConvertIFToInt(x any) (int, bool) {
	i64, ok := ConvertIFToInt64(x)
	if ok {
		if (i64 >= math.MinInt32) && (i64 <= math.MaxInt32) {
			return int(i64), true
		}
	}
	return int(0), false // not convertible
}

// ExtractFloat64 attempts to extract a float64 value from m[key].
// If m[key] is successfully parsed as a float64 value, the function sets *val to that value
// and returns (true, nil).
// If m[key] is found but an error occurs (such as m[key] not being a valid float64 value),
// the function returns (..., error).
// If m[key] is not found, the function returns (false, nil).
func ExtractFloat64(m map[string]any, key string, val *float64) (bool, error) {
	if valIF, found := m[key]; found {
		if val0, ok := ConvertIFToFloat64(valIF); !ok {
			return false, fmt.Errorf(
				"failed to convert %s %v (type %T) to float64", key, valIF, valIF)
		} else {
			*val = val0
			return true, nil
		}
	}
	return false, nil
}

// ExtractOptionalFloat64 attempts to extract an optional float64 value from m[key].
// If any error occurs (such as m[key] not being a valid float64 value), the function returns error.
// If m[key] is found and successfully parsed as a float64 value, the function sets *val to
// the address of a dynamically allocated memory location containing that value and and returns nil.
// If m[key] is not found, the function sets *val to nil and returns nil.
// NOTE: optionality is expressed by setting *val to either non-nil (= set/assigned) or nil
// (= unset/unassigned) as described in the last two cases.
func ExtractOptionalFloat64(m map[string]any, key string, val **float64) error {
	var val0 float64
	if found, err := ExtractFloat64(m, key, &val0); err != nil {
		return fmt.Errorf("ExtractFloat64(%s) failed: %v", key, err)
	} else if found {
		*val = &val0 // case 1: set/assigned
	} else {
		*val = nil // case 2: unset/unassigned
	}
	return nil
}

type KeyValueFloat64 struct {
	Key   string
	Value *float64
}

// ExtractFloat64Values is a convencience wrapper around ExtractFloat64 (documented separately)
// that allows multiple values to be extracted from the same map m. The function calls
// ExtractFloat64 for each item in keyValues and returns error upon the first item that fails,
// or nil if all succeeded.
// Returns nil upon success, otherwise error.
func ExtractFloat64Values(m map[string]any, keyValues []KeyValueFloat64) error {
	for _, item := range keyValues {
		if _, err := ExtractFloat64(m, item.Key, item.Value); err != nil {
			return fmt.Errorf("ExtractFloat64(%s) failed: %v", item.Key, err)
		}
	}
	return nil
}

type KeyValueOptionalFloat64 struct {
	Key   string
	Value **float64
}

// ExtractOptionalFloat64Values is a convencience wrapper around ExtractOptionalFloat64
// (documented separately) that allows multiple values to be extracted from the same map m.
// The function calls ExtractOptionalFloat64 for each item in keyValues and returns error upon the
// first item that fails, or nil if all succeeded.
// Returns nil upon success, otherwise error.
func ExtractOptionalFloat64Values(
	m map[string]any, keyValues []KeyValueOptionalFloat64) error {
	for _, item := range keyValues {
		if err := ExtractOptionalFloat64(m, item.Key, item.Value); err != nil {
			return fmt.Errorf("ExtractOptionalFloat64(%s) failed: %v", item.Key, err)
		}
	}
	return nil
}

// extractInt64 attempts to extract an int64 value from m[key] assumed to be one of the supported
// int types.
// Returns (value, true) upon success, otherwise (..., false).
func extractInt64(m map[string]any, key string) (int64, bool) {
	if vIF, found := m[key]; found {
		switch v := vIF.(type) {
		case int64:
			return v, true
		case int32:
			return int64(v), true
		case int16:
			return int64(v), true
		case int8:
			return int64(v), true
		case int:
			return int64(v), true
		}
		return int64(-1), false // not convertible
	}
	return int64(-1), false // not found
}

// IntegersEqual compares two map entries for equality, assuming they both are integers.
// Returns true iff m1[key] and m2[key] exist as equal integers.
func IntegersEqual(m1, m2 map[string]any, key string) bool {
	if v1, ok := extractInt64(m1, key); ok {
		if v2, ok := extractInt64(m2, key); ok {
			if v1 == v2 {
				return true
			}
		}
	}
	return false
}

// ExtractString attempts to extract a string value from m[key].
// Returns (value, true) upon success, otherwise ("", false).
func ExtractString(m map[string]any, key string) (string, bool) {
	if vIF, found := m[key]; found {
		if v, ok := vIF.(string); ok {
			return v, true
		}
		return "", false // not convertible
	}
	return "", false // not found
}

// StringsEqual compares two map entries for equality, assuming they both are strings.
// Returns true iff m1[key] and m2[key] exist as equal strings.
func StringsEqual(m1, m2 map[string]any, key string) bool {
	if v1, ok := ExtractString(m1, key); ok {
		if v2, ok := ExtractString(m2, key); ok {
			if v1 == v2 {
				return true
			}
		}
	}
	return false
}

// UnmarshalIF attempts to unmarshal interface source into interface target.
// Returns nil upon success, otherwise error.
func UnmarshalIF(source, target any) error {
	if srcB, err := json.Marshal(source); err != nil {
		return fmt.Errorf("json.Marshal() failed: %v", err)
	} else if err := json.Unmarshal(srcB, target); err != nil {
		return fmt.Errorf("json.Unmarshal() failed: %v", err)
	}
	return nil
}

// StrToBool converts string s to a boolean value.
// Returns true if trimmed+lowercase s doesn't match any of ("", "0", "no", "false"),
// otherwise false.
func StrToBool(s string) bool {
	return !StringInStrings(strings.ToLower(strings.TrimSpace(s)), []string{"", "0", "no", "false"})
}

// Compress returns (compressed data, nil) or (..., error) if an error occurs.
func Compress(data string) (string, error) {
	var bz bytes.Buffer

	gzwriter := gzip.NewWriter(&bz)
	if _, err := gzwriter.Write([]byte(data)); err != nil {
		return "", fmt.Errorf("gzwriter.Write() failed: %v", err)
	}

	if err := gzwriter.Flush(); err != nil {
		return "", fmt.Errorf("gzwriter.Flush() failed: %v", err)
	}

	if err := gzwriter.Close(); err != nil {
		return "", fmt.Errorf("gzwriter.Close() failed: %v", err)
	}

	return bz.String(), nil
}

// Uncompress returns (uncompressed data, nil) or (..., error) if an error occurs.
func Uncompress(data string) (string, error) {
	breader := bytes.NewReader([]byte(data))

	gzreader, err := gzip.NewReader(breader)
	if err != nil {
		return "", fmt.Errorf("gzip.NewReader() failed: %v", err)
	}

	b, err := io.ReadAll(gzreader)
	if err != nil {
		return "", fmt.Errorf("io.ReadAll() failed: %v", err)
	}

	return string(b), nil
}

// B64Encode returns base64-encoded data.
func B64Encode(data string) string {
	return b64.StdEncoding.EncodeToString([]byte(data))
}

// B64Decode returns (base64-decoded data, nil) or (..., error) if an error occurs.
func B64Decode(data string) (string, error) {
	b, err := b64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("b64.StdEncoding.DecodeString() failed: %v", err)
	}
	return string(b), nil
}

// CompressAndB64Encode first compresses and then base64-encodes data.
// Returns (result, nil) upon success, otherwise (..., error).
func CompressAndB64Encode(data string) (string, error) {
	c, err := Compress(data)
	if err != nil {
		return "", fmt.Errorf("Compress() failed: %v", err)
	}

	return B64Encode(c), nil
}

// B64DecodeAndUncompress first base64-decodes and then uncompresses data.
// Returns (result, nil) upon success, otherwise (..., error).
func B64DecodeAndUncompress(data string) (string, error) {
	d, err := B64Decode(data)
	if err != nil {
		return "", fmt.Errorf("B64Decode() failed: %v", err)
	}

	u, err := Uncompress(d)
	if err != nil {
		return "", fmt.Errorf("Uncompress() failed: %v", err)
	}

	return u, nil
}

// TruncString returns s string truncated to maxlen characters. If truncation is needed,
// the result will indicate the number of truncated characters.
func TruncString(s string, maxlen int) string {
	if (maxlen < 0) || (len(s) <= maxlen) {
		return s
	}
	return fmt.Sprintf("%s ... [truncated %d characters]", s[:maxlen], len(s)-maxlen)
}

// TimeInRange returns true iff t is within [from, to].
func TimeInRange(t, from, to time.Time) bool {
	return (!t.Before(from)) && (!t.After(to))
}

// GetUnsupportedQueryParamsMsg returns "" if queryParams only contains query parameters in sqp,
// the set of supported query parameters (assumed to be trimmed and lower case!).
// Otherwise a descriptive error message is rerturned.
func GetUnsupportedQueryParamsMsg(sqp StringSet, queryParams url.Values) string {
	// get unsupported query parameters
	uqp := StringSet{}
	for qp := range queryParams {
		qp0 := strings.ToLower(strings.TrimSpace(qp))
		if !sqp.Contains(qp0) {
			uqp.Set(qp0)
		}
	}

	sortStrings := func(slist []string) []string {
		sort.Strings(slist)
		return slist
	}

	pluralS := func(i int) string {
		if i == 1 {
			return ""
		} else {
			return "s"
		}
	}

	if len(uqp) > 0 { // found at least one unsupported query parameter
		return fmt.Sprintf(
			"found %d unsupported query parameter%s: %s; %d supported query parameter%s: %s",
			len(uqp), pluralS(len(uqp)), strings.Join(sortStrings(uqp.ToList()), ", "),
			len(sqp), pluralS(len(sqp)), strings.Join(sortStrings(sqp.ToList()), ", "))
	}

	return "" // no unsupported query parameters found
}

// GetAbsoluteFileName gets the absolute file name of fname relative to the source file from
// which the function is called.
//
// Returns (absolute file name, nil) upon success, otherwise (..., error).
func GetAbsoluteFileName(fname string) (string, error) {

	_, currFileName, _, ok := runtime.Caller(1) // get absolute/full name of current file
	if !ok {
		return "", fmt.Errorf("runtime.Caller() failed")
	}

	return path.Join(path.Dir(currFileName), fname), nil
}

// BuildAndRunGoProgram builds and runs a Go program by performing two stages:
//   - Stage 1: Build the program with exec.Cmd.Run() (which waits for the build command to complete)
//   - Stage 2: Run the program with exec.Cmd.Start() (which relies on exec.Cmd.Wait() to be called at
//     a later point)
//
// Parameters:
// - srcPath: program path, relative to repo root
// - workingDir: working directory to be used
// - env: environment to be used for the run stage (list of "<key>=<value>" pairs)
// - stdout, stderr: buffers for collecting output
//
// Returns (<*exec.Cmd of the run stage>, nil) upon success, otherwise (..., error).
// NOTE: the caller is responsible for eventually terminating the program, typically by calling
// Wait() and Process.Kill() on the returned exec.Cmd.
func BuildAndRunGoProgram(
	srcPath, workingDir string, runEnv []string, stdout, stderr *bytes.Buffer) (*exec.Cmd, error) {

	execOp := func(cmd *exec.Cmd, op func() error) error {

		cmd.Dir = workingDir

		cmd.Stdout = stdout
		cmd.Stderr = stderr

		if err := op(); err != nil {
			return fmt.Errorf("op() failed for %s: %v", cmd.String(), err)
		}

		return nil
	}

	var err error

	execName := strings.ReplaceAll(srcPath, "/", "_")

	// --- build stage --------------------------
	buildCmd := exec.Command("go", "build", "-o", execName, srcPath)
	err = execOp(buildCmd, buildCmd.Run)
	if err != nil {
		return nil, fmt.Errorf("building %s failed: %v", srcPath, err)
	}

	// --- run stage --------------------------

	runCmd := exec.Command(fmt.Sprintf("./%s", execName))
	runCmd.Env = runEnv
	err = execOp(runCmd, runCmd.Start)
	if err != nil {
		return nil, fmt.Errorf("running %s failed: %v", srcPath, err)
	}

	return runCmd, nil
}

// StopProgram stops the program represented by cmd. It is assumed that the program was started
// by exec.Cmd.Start().
func StopProgram(cmd *exec.Cmd) {

	done := make(chan struct{})

	timer := time.AfterFunc(time.Second*3, func() { // allow process 3 secs for cleanup tasks
		cmd.Process.Kill() // [1a] trigger termination (with a SIGKILL, i.e. uncatchable)
	})

	go func() {
		err := cmd.Wait() // [2] await termination (triggered by either [1a] or [1b])
		//log.Printf("error from cmd.process.Wait(): %v", err)
		_ = err
		timer.Stop() // [4] stop timer if still active
		close(done)  // [3] close channel
	}()

	cmd.Process.Signal(os.Interrupt) // [1b] trigger termination (with SIGINT, i.e. catchable to
	// allow process to perform any cleanup tasks)

	<-done // [3] await closing of channel
}

// StringMapKeys2CSV returns the keys of m as a sorted CSV string.
func StringMapKeys2CSV[T any](m map[string]T) string {

	keys := []string{}
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return strings.Join(keys, ", ")
}
