package timespecification

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/senseyeio/duration"
)

var (
	re1, re2, re3, re4 *regexp.Regexp
	iso8601layout      string
)

func init() {
	re1 = regexp.MustCompile(`^R(\d+)/([^RP/]+)/([^RP/]+)/(P.+)$`)
	re2 = regexp.MustCompile(`^R(\d+)/([^RP/]+)/(P.+)$`)
	re3 = regexp.MustCompile(`^([^RP/]+)/([^RP/]+)$`)
	re4 = regexp.MustCompile(`^([^RP/]+)$`)
	iso8601layout = "2006-01-02T15:04:05Z"
}

// LatestSpec specifies items in the most recent time segment (i.e. up until the current time).
type LatestSpec struct {
	// representation invariant: MaxAge > 0 && limit > 0
	MaxAge int64 // maximum age of an item in seconds (relative to the current time)
	Limit  int   // maximum number of data items (i.e. keeping only the most recent ones)
}

// IntervalsSpec specifies one or more time intervals generated from an absolute base interval.
type IntervalsSpec struct {
	// pattern: RN/T1/T2/P
	// representation invariants:
	//   * N > 0
	//   * T1 == T1.UTC()
	//   * T2 == T2.UTC()
	//   * T1 < T2
	//   * T2 <= T1.AddDate(PY,PM,PD).Add(PS)
	//   * T1last == T1 with AddDate(PY,PM,PD).Add(PS) applied N-1 times
	//   * T2last == T2 with AddDate(PY,PM,PD).Add(PS) applied N-1 times
	//   * PY >= 0
	//   * PM >= 0
	//   * PD >= 0
	//   * PS == h*60*60 + m*60 + s, where 0<=h<=23 && 0<=m<=59 && 0<=s<=59
	N      int       // number of intervals
	T1     time.Time // start of base (i.e. first) interval
	T2     time.Time // end of base (i.e. first) interval
	T1Last time.Time // start of last interval
	T2Last time.Time // end of last interval
	// period to generate subsequent intervals:
	PY int // years
	PM int // months
	PD int // days
	PS int // total seconds in time component
}

// addPeriod adds the period of ispec to t.
// Returns (time.Time{...}, nil) on success, otherwise (time.Time{}, error).
func (ispec IntervalsSpec) addPeriod(t time.Time) (time.Time, error) {
	if (ispec.PY == 0) && (ispec.PM == 0) && (ispec.PD == 0) && (ispec.PS == 0) {
		return time.Time{}, fmt.Errorf("attempt to add empty period")
	}
	return t.AddDate(ispec.PY, ispec.PM, ispec.PD).Add(time.Duration(ispec.PS) * time.Second), nil
}

// FirstInterval returns the first interval in ispec.
func (ispec IntervalsSpec) FirstInterval() (time.Time, time.Time) {
	return ispec.T1, ispec.T2
}

// NextInterval finds in ispec any next interval after the interval [t1, t2>.
// Returns (time.Time{}, time.Time{}, false, error) if an error occurs.
// If an error does not occur, the function returns (time.Time{...}, time.Time{...}, true, nil)
// if a next interval can be found and is at or before the last possible interval,
// otherwise (time.Time{}, time.Time{}, false, nil).
func (ispec IntervalsSpec) NextInterval(t1, t2 time.Time) (time.Time, time.Time, bool, error) {
	// assert(N > 0)
	if ispec.N == 1 {
		return time.Time{}, time.Time{}, false, nil // by definition
	}

	t1next, err := ispec.addPeriod(t1)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("ispec.addPeriod(t1) failed: %v", err)
	}

	if t1next.Equal(t1) { // ### Is this case possible? Wouldn't it be caught by an error from the
		// above call to addPeriod() (adding an empty period)?
		return time.Time{}, time.Time{}, false, nil // by definition
	}

	//assert(!t1next.Before(t2))
	if t1next.After(ispec.T1Last) {
		return time.Time{}, time.Time{}, false, nil
	}

	t2next, err := ispec.addPeriod(t2)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("ispec.addPeriod(t2) failed: %v", err)
	}

	return t1next, t2next, true, nil
}

// FindInterval returns the first interval in ispec where t < T2 if such an interval exists.
//
// Returns (t1, t2, true, nil) if an interval can be found, (..., ..., false, nil)
// if not, and (..., ..., false, error) if an error occurred.
func (ispec IntervalsSpec) FindInterval(t int64) (time.Time, time.Time, bool, error) {

	tt := time.Unix(t, 0)

	if tt.Before(ispec.T2Last) {
		// an interval exists, so find the precise one
		t1, t2 := ispec.FirstInterval()
		var err error

		for i := 0; i < ispec.N; i++ {
			if tt.Before(t2) {
				return t1, t2, true, nil // found
			}
			t1, err = ispec.addPeriod(t1)
			if err != nil {
				return time.Time{}, time.Time{}, false,
					fmt.Errorf("ispec.addPeriod(t1) failed: %v", err)
			}
			t2, err = ispec.addPeriod(t2)
			if err != nil {
				return time.Time{}, time.Time{}, false,
					fmt.Errorf("ispec.addPeriod(t2) failed: %v", err)
			}
		}

		// if we get here it must be because addPeriod() doesn't behave the way
		// we would expect, so flag an error
		return time.Time{}, time.Time{}, false,
			fmt.Errorf("failed to find interval even if %v is before %v", tt, ispec.T2Last)
	}

	return time.Time{}, time.Time{}, false, nil // not found
}

// TimeSpecification ...
type TimeSpecification struct {
	LSpec *LatestSpec
	ISpec *IntervalsSpec
}

// OverallInterval returns the overall interval defined by tspec.
// Returns on success (from, to) where (from < to) and (from != math.MinInt64)
// and (to != math.MinInt64), otherwise (math.MinInt64, math.MinInt64).
func (tspec TimeSpecification) OverallInterval() (int64, int64) {
	if tspec.LSpec != nil {
		now := time.Now().Unix()
		return now - tspec.LSpec.MaxAge, now
	}

	if tspec.ISpec != nil {
		return tspec.ISpec.T1.Unix(), tspec.ISpec.T2Last.Unix()
	}

	return math.MinInt64, math.MinInt64
}

// getPeriodComponents extracts from p the six components from a pattern of the form
// P[n]Y[n]M[n]DT[n]H[n]M[n]S. Each [n] represents a non-negative integer. A component may
// be ommited if the corresponding [n] is 0. At least one component must have [n] > 0.
// Returns (years, months, days, hours, minutes, seconds, nil) on success, otherwise
// (0, 0, 0, 0, 0, 0, error).
func getPeriodComponents(p string) (int, int, int, int, int, int, error) {
	pattern := `^P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(p)
	if matches == nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("%s doesn't match pattern %s", p, pattern)
	}

	years, _ := strconv.Atoi(matches[1])
	// assert(years >= 0)

	months, _ := strconv.Atoi(matches[2])
	// assert(months >= 0)

	days, _ := strconv.Atoi(matches[3])
	// assert(days >= 0)

	hours, _ := strconv.Atoi(matches[4])
	// assert(hours >= 0)
	if hours > 23 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("hours outside valid interval [0, 23]: %d", hours)
	}

	minutes, _ := strconv.Atoi(matches[5])
	// assert(minutes >= 0)
	if minutes > 59 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("minutes outside valid interval [0, 59]: %d", minutes)
	}

	seconds, _ := strconv.Atoi(matches[6])
	// assert(seconds >= 0)
	if seconds > 59 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("seconds outside valid interval [0, 59]: %d", seconds)
	}

	if (years == 0) && (months == 0) && (days == 0) && (hours == 0) && (minutes == 0) &&
		(seconds == 0) {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("empty period not allowed: %s", p)
	}

	return years, months, days, hours, minutes, seconds, nil
}

// Extracts 'latestmaxage' from query parameters. The input value can be expressed directly
// as an integer number of seconds or indirectly as an ISO 8601 duration (e.g. PT3H). In the latter
// case, the duration is converted to a number of seconds.
// Returns (int64, nil) on success, otherwise (-1, error).
func getLatestMaxAge(queryParams url.Values) (int64, error) {
	latestMaxAges, found := queryParams["latestmaxage"]
	if !found {
		return int64(10800), nil // by default (# of secs in PT3H)
	}

	if len(latestMaxAges) > 1 {
		return -1, fmt.Errorf(
			"at most one 'latestmaxage' query parameter allowed; found %d", len(latestMaxAges))
	}

	latestMaxAge := strings.ToUpper(latestMaxAges[0])

	// try to extract directly as an integer
	secs, err := strconv.ParseInt(latestMaxAge, 10, 64)
	if err == nil {
		if secs < 1 {
			return -1, fmt.Errorf("latestmaxage not a positive integer: %d", secs)
		}
		return secs, nil
	}

	// try to extract directly as an ISO 8601 duration
	d, err := duration.ParseISO8601(latestMaxAge)
	if err == nil {
		secs = int64(d.TS)
		secs += int64(d.TM) * 60
		secs += int64(d.TH) * 3600
		secs += int64(d.D) * 86400
		if (d.W != 0) || (d.M != 0) || (d.Y != 0) {
			return -1, fmt.Errorf(
				"weeks, months, or years not supported for latestmaxage as ISO 8601 duration")
		}
		return secs, nil
	}

	return -1, fmt.Errorf(
		"latestmaxage neither an integer nor an ISO 8601 duration: %s", latestMaxAge)
}

// Extracts 'latestlimit' from query parameters.
//
// Returns (int, nil) on success, otherwise (-1, error).
func getLatestLimit(queryParams url.Values) (int, error) {
	latestLimits, found := queryParams["latestlimit"]
	if !found {
		return 1, nil // by default
	}

	if len(latestLimits) > 1 {
		return -1, fmt.Errorf(
			"at most one 'latestlimit' query parameter allowed; found %d", len(latestLimits))
	}

	latestLimit, err := strconv.Atoi(latestLimits[0])
	if err != nil {
		return -1, fmt.Errorf("failed to convert latestlimit (%s) to an int", latestLimits[0])
	}

	if latestLimit < 1 {
		return -1, fmt.Errorf("latestlimit not a positive integer: %d", latestLimit)
	}

	return latestLimit, nil
}

// GetTimeSpecification extracts a TimeSpecification object from queryParams.
//
// Returns (TimeSpecification, nil) on success, otherwise (TimeSpecification{}, error).
func GetTimeSpecification(queryParams url.Values) (TimeSpecification, error) {
	times, found := queryParams["time"]
	if !found {
		return TimeSpecification{}, nil
	}

	if len(times) > 1 {
		return TimeSpecification{},
			fmt.Errorf("at most one 'time' query parameter allowed; found %d", len(times))
	}

	time0 := strings.TrimSpace(times[0])
	if time0 == "" { // equivalent to omitting the parameter altogether
		return TimeSpecification{}, nil
	}

	// --- 'latest' mode ---------------------------------------------------------
	if strings.ToLower(time0) == "latest" {

		// extract latestmaxage and latestlimit
		maxAge, err := getLatestMaxAge(queryParams)
		if err != nil {
			return TimeSpecification{}, fmt.Errorf("getLatestMaxAge() failed: %v", err)
		}
		limit, err := getLatestLimit(queryParams)
		if err != nil {
			return TimeSpecification{}, fmt.Errorf("getLatestLimit() failed: %v", err)
		}

		return TimeSpecification{LSpec: &LatestSpec{MaxAge: maxAge, Limit: limit}, ISpec: nil}, nil
	}

	// --- 'intervals' mode ---------------------------------------------------------

	// repeating interval variants:
	// [1] Rn/ <ISO 8601 time> /<ISO 8601 time> /<ISO 8601 duration>   -> interval   with    offset
	// [2] Rn/ <ISO 8601 time>                  /<ISO 8601 duration>   -> time point with    offset
	//
	// non-repeating interval variants:
	// [3]     <ISO 8601 time> /<ISO 8601 time>                        -> interval
	// [4]     <ISO 8601 time>                                         -> time point

	var sn, st1, st2, sp string

	validPattern := true
	defaultT2 := false
	if matches := re1.FindStringSubmatch(time0); matches != nil {
		// assert(len(matches) == 5)
		sn = matches[1]
		st1 = matches[2]
		st2 = matches[3]
		sp = matches[4]
	} else if matches := re2.FindStringSubmatch(time0); matches != nil {
		// assert(len(matches) == 4)
		sn = matches[1]
		st1 = matches[2]
		defaultT2 = true
		sp = matches[3]
	} else if matches := re3.FindStringSubmatch(time0); matches != nil {
		// assert(len(matches) == 3)
		sn = "1"
		st1 = matches[1]
		st2 = matches[2]
	} else if matches := re4.FindStringSubmatch(time0); matches != nil {
		// assert(len(matches) == 2)
		sn = "1"
		st1 = matches[1]
		defaultT2 = true
	} else {
		validPattern = false
	}

	if !validPattern {
		return TimeSpecification{}, fmt.Errorf(
			"time specification not of the form [Rn/]<ISO 8601 time>[/<ISO 8601 time>]" +
				"[/<ISO 8601 duration>]")
	}

	var err error

	// --- get number of intervals -----------------
	n, err := strconv.Atoi(sn)
	if (err != nil) || (n < 1) {
		return TimeSpecification{},
			fmt.Errorf("interval occurrences not a positive integer: %s", sn)
	}
	maxIntervals := 100000 // hard code for now
	if n > maxIntervals {
		return TimeSpecification{},
			fmt.Errorf("too many interval occurrences: %d > %d", n, maxIntervals)
	}

	// --- get base interval -----------------
	t1, err := time.Parse(iso8601layout, st1)
	if err != nil {
		return TimeSpecification{}, fmt.Errorf("time.Parse() failed for %s: %v", st1, err)
	}
	var t2 time.Time
	if defaultT2 {
		t2 = t1.Add(time.Second) // by definition
	} else {
		t2, err = time.Parse(iso8601layout, st2)
		if err != nil {
			return TimeSpecification{}, fmt.Errorf("time.Parse() failed for %s: %v", st2, err)
		}
		if !t2.After(t1) {
			return TimeSpecification{}, fmt.Errorf("t2 (%s) not after t1 (%s)", st2, st1)
		}
	}
	//assert(t2.After(t1))

	// --- get period -----------------
	pyears := 0
	pmonths := 0
	pdays := 0
	pseconds := 0
	if sp != "" {
		if n == 1 {
			return TimeSpecification{},
				fmt.Errorf("a period only makes sense for more than one interval")
		}

		var hours int
		var minutes int
		var seconds int
		pyears, pmonths, pdays, hours, minutes, seconds, err = getPeriodComponents(sp)
		if err != nil {
			return TimeSpecification{}, fmt.Errorf("getPeriodComponents() failed: %v", err)
		}
		pseconds = hours*60*60 + minutes*60 + seconds

		// check that period isn't shorter than t2-t1
		t1add := t1.AddDate(pyears, pmonths, pdays).Add(time.Duration(pseconds) * time.Second)
		if t1add.Before(t2) {
			return TimeSpecification{},
				fmt.Errorf("period too short: t1+p (%v) < t2 (%v)", t1add, t2)
		}
	}

	// --- create preliminary ispec -----------------
	ispec := IntervalsSpec{
		N:      n,
		T1:     t1,
		T2:     t2,
		T1Last: t1,
		T2Last: t2,
		PY:     pyears,
		PM:     pmonths,
		PD:     pdays,
		PS:     pseconds,
	}

	// --- finalize ispec -----------------
	for i := 0; i < (n - 1); i++ {
		ispec.T1Last, err = ispec.addPeriod(ispec.T1Last)
		if err != nil {
			return TimeSpecification{}, fmt.Errorf("ispec.addPeriod(ispec.T1Last) failed: %v", err)
		}
		ispec.T2Last, err = ispec.addPeriod(ispec.T2Last)
		if err != nil {
			return TimeSpecification{}, fmt.Errorf("ispec.addPeriod(ispec.T2Last) failed: %v", err)
		}
	}

	return TimeSpecification{LSpec: nil, ISpec: &ispec}, nil
}
