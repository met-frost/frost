package writerestriction

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	"gitlab.met.no/frost/frost/internal/auth"
	"gitlab.met.no/frost/frost/internal/common"
)

func tsWriteAccessSchema() string {
	return `{
		"type": "object",
		"properties": {
		    "rules": {
		        "type": "array",
		        "items": {
			        "properties": {
						"descr": {
							"type": "array",
							"items": {
								"type": "string"
							}
						},
			        	"token": {
							"type": "string"
				        },
				        "tstype": {
					        "type": "string"
				        },
				        "tscreate": {
					        "type": "boolean"
				        },
				        "tsupdate": {
					        "type": "boolean"
				        },
				        "tsdelete": {
					        "type": "boolean"
				        },
				        "put": {
					        "type": "boolean"
				        },
				        "hdridmatch": {
					        "type": "string"
				        }
			        },
			        "required": [
						"token", "tstype", "tscreate", "tsupdate", "tsdelete", "put", "hdridmatch"],
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
	schemaLoader = gojsonschema.NewStringLoader(tsWriteAccessSchema())
}

// explicit rule object
type rule struct {
	Token      string `json:"token"`      // token to be matched against X-Frost-Writetoken
	TsType     string `json:"tstype"`     // time series type
	TsCreate   bool   `json:"tscreate"`   // whether the ts/create operation is allowed
	TsUpdate   bool   `json:"tsupdate"`   // whether the ts/update operation is allowed
	TsDelete   bool   `json:"tsdelete"`   // whether the ts/delete operation is allowed
	Put        bool   `json:"put"`        // whether the put operation is allowed
	HdrIDMatch string `json:"hdridmatch"` // time series header ID match expression
}

// array of explicit rule objects (in order of appearance in file)
var rules []rule

// Load loads write restriction rules from any TSWRITEACCESS environment variable.
// Returns nil upon success, otherwise error.
func Load() error {
	// get file name
	fname := strings.TrimSpace(common.Getenv("TSWRITEACCESS", ""))
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
	err = json.Unmarshal(bsobj2, &rules)
	if err != nil {
		return fmt.Errorf("json.Unmarshal() #2 failed (file: %s): %v", fname, err)
	}

	return nil
}

// GetWriteRestriction gets write restriction info for a particular combination of
// time series type, operation, and request.
// If an error occurs, the function returns (..., ..., HTTP status code, error).
// If write restriction is generally disabled, the function returns (false, ..., ..., ...).
// Otherwise, the function returns (true, whiPatterns, -1, nil), where whiPatterns is
// the set of header ID patterns of time series to which the request is allowed to apply
// the operation.
func GetWriteRestriction(
	tstype, operation string, request *http.Request) (bool, []string, int, error) {

	writeKey := common.Getenv("WRITEKEY", "")
	if writeKey == "" {
		return false, []string{}, -1, nil // write restriction generally disabled
	}

	whiPatterns := []string{}

	hdrName := "X-Frost-Writetoken"
	tokenB64Enc, tokenB64Dec, statusCode, err := auth.ExtractToken(request, hdrName)
	if err != nil {
		return false, []string{}, statusCode, fmt.Errorf("auth.ExtractToken() failed: %v", err)
	}

	if tokenB64Enc == "" {
		return false, []string{}, http.StatusUnauthorized,
			fmt.Errorf("no write token header found in request: %s", hdrName)
	}

	// do a general validation of the token
	err = auth.ValidateWriteToken(tokenB64Dec, writeKey, hdrName)
	if err != nil {
		return false, []string{}, http.StatusUnauthorized,
			fmt.Errorf("auth.ValidateWriteToken() failed: %v", err)
	}

	for _, rule := range rules { // search for a matching rule

		// skip to next rule upon a mismatch of any component
		if tokenB64Enc != rule.Token {
			continue
		}
		if tstype != rule.TsType {
			continue
		}
		if (operation == "tscreate") && (!rule.TsCreate) {
			continue
		}
		if (operation == "tsupdate") && (!rule.TsUpdate) {
			continue
		}
		if (operation == "tsdelete") && (!rule.TsDelete) {
			continue
		}
		if (operation == "put") && (!rule.Put) {
			continue
		}

		// rule matched completely, so add this header ID pattern to the result
		whiPatterns = append(whiPatterns, rule.HdrIDMatch)
	}

	return true, whiPatterns, -1, nil
}
