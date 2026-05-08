package readrestriction

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xeipuuv/gojsonschema"
	"gitlab.met.no/frost/frost/internal/auth"
	"gitlab.met.no/frost/frost/internal/common"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/tsregistry"
)

func tsReadAccessSchema() string {
	return `{
		"type": "object",
		"properties": {
		    "rules": {
		        "type": "array",
		        "items": {
			        "properties": {
			        	"type": {
							"enum": ["drop_all", "keep_in_period", "drop_in_period"]
				        },
				        "tstype": {
					        "type": "string"
				        },
				        "hdridmatch": {
					        "type": "string"
				        },
				        "from": {
					        "type": "string"
				        },
				        "to": {
					        "type": "string"
				        },
				        "from_relative_to_now": {
					        "type": "boolean"
				        },
				        "to_relative_to_now": {
					        "type": "boolean"
				        },
				        "exempted_tokens": {
					        "type": "array",
					        "items": {
						        "type": "string"
					        }
				        }
			        },
			        "required": ["type", "tstype", "hdridmatch"],
			        "additionalProperties": false
		        }
			}
		},
		"required": ["rules"],
		"additionalProperties": false
	}`
}

var schemaLoader gojsonschema.JSONLoader

func init() {
	schemaLoader = gojsonschema.NewStringLoader(tsReadAccessSchema())
}

// timePeriod represents a time timePeriod that can be open-ended
type timePeriod struct {
	from     time.Time // finite beginning of period; n/a if fromOpen is true
	to       time.Time // finite end of period; n/a if toOpen is true
	fromOpen bool      // whether from is to be considered infinitely early
	toOpen   bool      // whether to is to be considered infinitely early
}

// explicit rule object
type rule struct {
	rtype          string     // rule type, one of "drop_all", "keep_in_period", or "drop_in_period"
	tstype         string     // time series type
	hdridmatch     string     // time series header ID match expression
	tp             timePeriod // applicable for rule types "keep_in_period" and "drop_in_period"
	exemptedTokens []string   // read tokens exempted from the rule
}

// array of explicit rule objects (in order of appearance in file)
var rules []rule

// tstampStringToTime converts tstamp to time.Time.
// If addNow is true, the timestamp of the current time is added to the result.
// Returns (time.Time, nil) upon success, otherwise (time.Time{}, error).
func tstampStringToTime(tstamp string, addNow bool) (time.Time, error) {
	tstamp0, err := strconv.ParseInt(tstamp, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to convert %s to an int64", tstamp)
	}
	if addNow {
		tstamp0 += time.Now().Unix()
	}
	return time.Unix(tstamp0, 0), nil
}

// extractOptionalTime extracts from rule and key a time.Time (UTC) and a flag that tells if
// rule contained key at all. The rule[key] value is assumed to be a stringified UTC epoch
// timestamp. If rule contained key and rule[relToNowKey] exists as the boolean value true, the
// timestamp of the current time is added to the result.
// Upon success, the function returns (time.Time (UTC), true, nil) if found and
// (time.Time{}, false, nil) if not.
// Upon failure, the function returns (time.Time{}, ..., error).
func extractOptionalTime(
	rule *map[string]interface{}, key, relToNowKey string) (time.Time, bool, error) {
	t0, found := (*rule)[key]
	if found {
		t0s := strings.TrimSpace(t0.(string)) // assumed to be validated already
		if t0s == "" {
			return time.Time{}, false, nil // not found (by definition)
		}
		t, err := tstampStringToTime(t0s, (*rule)[relToNowKey] == true)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("tstampStringToTime() failed: %v", err)
		}
		return t.UTC(), true, nil // found
	}
	return time.Time{}, false, nil // not found
}

// Load loads read restriction rules from any TSREADACCESS environment variable.
// Returns nil upon success, otherwise error.
func Load() error {
	// get file name
	fname := strings.TrimSpace(common.Getenv("TSREADACCESS", ""))
	if fname == "" {
		return nil // no rules defined
	}

	// read object from file
	bsobj, err := os.ReadFile(fname)
	if err != nil {
		return fmt.Errorf("os.ReadFile(%s) failed: %v", fname, err)
	}
	var obj map[string]interface{}
	err = json.Unmarshal(bsobj, &obj)
	if err != nil {
		return fmt.Errorf("json.Unmarshal() #1 failed (file: %s): %v", fname, err)
	}

	// validate against schema
	err = common.SchemaValidate(schemaLoader, obj)
	if err != nil {
		return fmt.Errorf("common.SchemaValidate() failed (file: %s): %v", fname, err)
	}

	// extract array of rules
	bsobj2, err := json.Marshal(obj["rules"])
	if err != nil {
		return fmt.Errorf("json.Marshal() failed (file: %s): %v", fname, err)
	}

	// extract implicit rule objects
	var rules0 []map[string]interface{}
	err = json.Unmarshal(bsobj2, &rules0)
	if err != nil {
		return fmt.Errorf("json.Unmarshal() #2 failed (file: %s): %v", fname, err)
	}

	// convert from implicit to explicit rule objects (while doing additional validation)
	for _, r0 := range rules0 {
		r := rule{
			rtype:      r0["type"].(string),
			tstype:     r0["tstype"].(string),
			hdridmatch: r0["hdridmatch"].(string),
		}

		if r.rtype != "drop_all" {
			// assert((r.rtype == "keep_in_period") || (r.rtype == "drop_in_period"))

			// extract time period
			from, fromSet, err := extractOptionalTime(&r0, "from", "from_relative_to_now")
			if err != nil {
				return fmt.Errorf("extractOptionalTime(from) failed: %v", err)
			}
			to, toSet, err := extractOptionalTime(&r0, "to", "to_relative_to_now")
			if err != nil {
				return fmt.Errorf("extractOptionalTime(to) failed: %v", err)
			}
			if fromSet && toSet && from.After(to) {
				return fmt.Errorf("negative time period: from (%v) > to (%v)", from, to)
			}

			r.tp = timePeriod{
				from:     from,
				to:       to,
				fromOpen: !fromSet,
				toOpen:   !toSet,
			}
		}

		// extract exempted tokens
		if ets, found := r0["exempted_tokens"]; found {
			for _, et := range ets.([]interface{}) {
				r.exemptedTokens = append(r.exemptedTokens, et.(string))
			}
		}

		rules = append(rules, r)
	}

	return nil
}

// objMatch decides if obj1 matches obj2 by the following definition:
//   1: each key in obj2 is also a key in obj1 (i.e. the obj2 keys is a subset of the obj1 keys)
//   2: for each key k in obj2, the serialization of obj2[k], which is allowed to contain
//      asterisks to match any sequence of zero or more characters, must match the serialization
//      of obj1[k]
// Returns (bool, nil) if no error occurs, otherwise (..., error).
func objMatch(obj1, obj2 map[string]interface{}) (bool, error) {

	for k, v2 := range obj2 {
		if v1, found := obj1[k]; found {
			sv1, err := json.Marshal(v1)
			if err != nil {
				return false, fmt.Errorf("json.Marshal(v1) failed: %v", err)
			}
			sv2, err := json.Marshal(v2)
			if err != nil {
				return false, fmt.Errorf("json.Marshal(v2) failed: %v", err)
			}

			if !common.MatchesAsteriskPattern(string(sv1), string(sv2)) {
				return false, nil // mismatch found
			}

		} else {
			return false, fmt.Errorf("key >%s< not found in obj1: %v", k, obj1)
		}
	}

	return true, nil // no mismatches found
}

// findRule tries to find a rule that matches the time series defined by tstype/tsid.
// Upon success, the function returns (the first matching rule, true, nil) or
// (rule{}, false, nil) if no matching rule could be found.
// Upon failure, the function returns (rule{}, ..., error).
func findRule(tstype string, tsid map[string]interface{}) (rule, bool, error) {
	for _, rule0 := range rules {
		if rule0.tstype != tstype {
			continue // tstype mismatch
		}

		// --- STEP 1: find ts (if any) that matches tsid ---------------------------------
		defaultTS, err := tsregistry.DefaultTimeSeries(tstype)
		if err != nil {
			return rule{}, false,
				fmt.Errorf("tsregistry.DefaultTimeSeries(%s) failed: %v", tstype, err)
		}

		stsid, err := json.Marshal(tsid)
		if err != nil {
			return rule{}, false,
				fmt.Errorf("json.Marshal() failed: %v", err)
		}

		ts, err := defaultTS.FindInstanceFromID(stsid)
		if err != nil {
			return rule{}, false,
				fmt.Errorf("defaultTS.FindInstanceFromID() failed: %v", err)
		}

		if ts == nil {
			continue // tsid mismatch
		}

		// --- STEP 2: check if ts matches rule0.hdridmatch ---------------------------------

		id, err := (*ts).GetHeaderID()
		if err != nil {
			return rule{}, false,
				fmt.Errorf("(*ts).GetHeaderID() failed: %v", err)
		}

		var obj map[string]interface{}
		err = json.Unmarshal([]byte(rule0.hdridmatch), &obj)
		if err != nil {
			return rule{}, false, fmt.Errorf("json.Unmarshal(%s) failed: %v", rule0.hdridmatch, err)
		}

		match, err := objMatch(id, obj)
		if err != nil {
			return rule{}, false, fmt.Errorf("objMatch() failed: %v", err)
		}

		if match {
			return rule0, true, nil // match found, so this rule is applicable to tsid
		}
	}

	return rule{}, false, nil // no match, so no rule is applicable to tsid
}

// insidePeriod returns true iff t is inside p.
func insidePeriod(t time.Time, p timePeriod) bool {
	if p.fromOpen && p.toOpen {
		return true
	}

	if p.fromOpen {
		// assert(!p.toOpen)
		return !t.After(p.to) // t <= to
	}

	if p.toOpen {
		// assert(!p.fromOpen)
		return !p.from.After(t) // from <= t
	}

	// assert(!p.fromOpen && !p.toOpen)
	// assert(!p.from.After(p.to)) // from <= to; should be checked already in Load()

	return (!t.Before(p.from)) && (!p.to.Before(t)) // (from <= t) && (t <= to)
}

// restricted checks if obs is restricted according to rule r.
// Returns (true/false, nil) upon success, otherwise (..., error).
func restricted(obs dataset.Observation, r rule) (bool, error) {
	if r.rtype == "drop_all" {
		return true, nil // restrict regardless of any other rule settings
	}

	// assert((r.rtype == "keep_in_period") || (r.rtype == "drop_in_period"))
	keepInPeriod := true
	if r.rtype == "drop_in_period" {
		keepInPeriod = false
	}

	// decide if the observation time is inside the time period defined by the rule
	inside := insidePeriod(*obs.Time, r.tp)

	// decide final restriction
	if keepInPeriod && inside {
		return false, nil
	}
	if keepInPeriod && !inside {
		return true, nil
	}
	if !keepInPeriod && inside {
		return true, nil
	}
	//assert(!keepInPeriod && !inside)
	return false, nil
}

// exempted checks if a request is exempted from the read restriction rule defined by r.
// Returns (bool, nil) upon success, otherwise (..., error).
func exempted(r rule, request *http.Request) (bool, error) {
	for _, et := range r.exemptedTokens {
		exm, err := auth.ExemptedFromReadRestriction(request, et)
		if err != nil {
			return false, fmt.Errorf("auth.ExemptedFromReadRestriction() failed: %v", err)
		}
		if exm {
			return true, nil // match found => exempted
		}
	}
	return false, nil // no match found => not exempted
}

// Apply applies, for a specific request, any relevant read restriction rules to dset by replacing
// affected observation bodies with null. A rule will be applied only if the request does not
// contain a read token that matches one of the exempted read tokens of the rule.
// Returns nil upon success, otherwise error.
func Apply(dset *dataset.Dataset, request *http.Request) error {
	tstype := dset.TSeriesType

	// loop over time series in dataset
	for i, sts := range dset.TSeries {

		// find the first rule (if any) that matches this time series
		r, found, err := findRule(tstype, sts.Header.ID)
		if err != nil {
			return fmt.Errorf("findRule() failed: %v", err)
		}
		if !found { // sts didn't match any rules
			continue
		}

		// check if request is exempted from the rule
		exm, err := exempted(r, request)
		if err != nil {
			return fmt.Errorf("exempted() failed: %v", err)
		}
		if exm {
			continue // don't apply the rule for this readToken
		}

		// apply rule

		modObs := []dataset.Observation{} // restricted version of obs array
		for _, obs := range sts.Observations {
			var modBody map[string]interface{}
			restr, err := restricted(obs, r)
			if err != nil {
				return fmt.Errorf("restricted() failed: %v", err)
			}
			if restr {
				modBody = nil // restrict by setting obs body to nil
			} else {
				modBody = obs.Body // leave unmodified
			}
			modObs = append(modObs, dataset.Observation{
				Time: obs.Time,
				Body: modBody,
			})
		}

		// replace dset.TSeries[i] with restricted version of obs array
		sts.Observations = modObs
		dset.TSeries[i] = sts
	}

	return nil
}
