// Package openapi defines an API to be used during the initialization phase of the service
// for registering the various parts of the OpenAPI specification (OAS) and for assembling
// the parts to a final OAS file.
package openapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"gitlab.met.no/frost/frost/internal/common"
)

type OAParameter struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Example     string            `json:"example"`
	In          string            `json:"in"`
	Required    bool              `json:"required"`
	Schema      map[string]string `json:"schema"`
}

type OAParameters []OAParameter

// OAResponse defines info for documenting the response of an HTTP status code.
type OAResponse struct {
	Description string         `json:"description"`
	Content     map[string]any `json:"content"`
}

type OAResponses map[string]OAResponse

// OAGet defines info for documenting an HTTP GET operation.
type OAGet struct {
	Summary     string       `json:"summary"`
	Description string       `json:"description"`
	OperationID string       `json:"operationId"`
	Tags        []string     `json:"tags"`
	Parameters  OAParameters `json:"parameters"`
	Responses   OAResponses  `json:"responses"`
}

var (
	docLevelKey string
	docGroupKey string
)

func init() {
	docLevelKey = "doclevel"
	docGroupKey = "docgroup"
}

// groupKeyRank returns the rank of any group key found in obj, or by default the highest
// possible rank.
func groupKeyRank(obj any) int {

	// NOTE: styling of the groups (like background-color) is defined in static/swaggerui/custom.css

	// return rank of first group key that matches
	keys := []string{"where", "what", "quality", "when", "output", "other"} // prioritized order
	if b, ok := obj.(map[string]any); ok {
		for i, key := range keys {
			if hasKeyValue(b, docGroupKey, key) {
				return i
			}
		}
	}

	return len(keys) // return highest possible rank by default
}

// sortArraysOnGroupKeys returns a copy of val where each object array in val is sorted on group
// key.
func sortArraysOnGroupKeys(val any) any {

	if arr, ok := val.([]any); ok {
		sort.SliceStable(arr, func(i, j int) bool {
			return groupKeyRank(arr[i]) < groupKeyRank(arr[j])
		})
	}

	if obj, ok := val.(map[string]any); ok {
		for _, v := range obj {
			sortArraysOnGroupKeys(v)
		}
	}

	return val // return val at the end, after recursively sorting
}

// hasKeyValue returns true iff obj has a specific key/value combo.
func hasKeyValue(obj map[string]any, key, val string) bool {

	if valIF, found := obj[key]; found {
		if val0, ok := valIF.(string); ok {
			return val0 == val // key found, so return whether it matches the target value
		}
	}

	return false // key not found or doesn't match target value
}

// FormatGenericErrorResponses returns a string of the form
//
//	"<HTTP error status code>": <response object>,
//	"<HTTP error status code>": <response object>,
//	...
//
// where the status codes are taken from errStatusCodes and the response objects indicate
// error details as just a generic string.
func FormatGenericErrorResponses(errStatusCodes ...int) string {

	responses := []string{}

	for _, code := range errStatusCodes {

		var descr string

		switch code {
		case
			400, // Bad Request
			401, // Unauthorized
			403, // Forbidden
			404, // Not Found
			500, // Internal Server Error
			501: // Not Implemented
			descr = http.StatusText(code)
		default:
			descr = fmt.Sprintf("Unexpected error (%s)", http.StatusText(code))
		}

		responses = append(responses, fmt.Sprintf(`
			"%d": {
				"description": "%s.",
				"content": {
					"application/json": {
						"schema": {
							"type": "object",
							"title": "Error details for %d %s.",
							"properties": {
								"error": {
									"type": "string"
								}
							}
						}
					}
				}
			}
		`, code, descr, code, descr))
	}

	return strings.Join(responses, ",")
}

// DocLevelBasicOnly returns the doc level constant to indicate that this part will be included in
// the basic documentation only.
func DocLevelBasicOnly() string {
	return "basiconly"
}

// DocLevelAdvancedOnly returns the doc level constant to indicate that this part will be included
// in the advanced documentation only.
func DocLevelAdvancedOnly() string {
	return "advancedonly"
}

// DocLevelBoth returns the doc level constant to indicate that this part will be included in both
// the basic and the advanced documentation.
func DocLevelBoth() string {
	return "both"
}

// keepBasic returns true iff obj is to be kept for the basic documentation.
func keepBasic(obj map[string]any) bool {
	return !hasKeyValue(obj, docLevelKey, DocLevelAdvancedOnly())
}

// keepAdvanced returns true obj is to be kept for the advanced documentation.
func keepAdvanced(obj map[string]any) bool {
	return !hasKeyValue(obj, docLevelKey, DocLevelBasicOnly())
}

// copySubset returns a copy of val where each object obj in val is kept only if keep(obj) == true.
// If val itself is an object for which keep() is false, the function returns nil.
func copySubset(val any, keep func(map[string]any) bool) any {

	if obj, ok := val.(map[string]any); ok {
		// object, so first check if it itself is to be kept
		if !keep(obj) {
			return nil // nope, so return an empty result
		}
		// then return a copy containing only those key/value pairs for which copySubset(value)
		// returned a non-empty result
		res := map[string]any{}
		for k, v := range obj {
			if v0 := copySubset(v, keep); v0 != nil {
				res[k] = v0
			}
		}
		return res
	}

	if arr, ok := val.([]any); ok {
		// array, so return copy containing only those items for which copySubset
		// returned a non-empty result
		res := []any{}
		for _, v := range arr {
			if v0 := copySubset(v, keep); v0 != nil {
				res = append(res, v0)
			}
		}
		return res
	}

	return val // neither object nor array, so return val directly
}

// Definitions:
// - OAS            = The OpenAPI Specification.
// - OpenAPI Object = The root document object of the OpenAPI document.

// Tag corresponds with an OpenAPI tag object to be kept in the 'tags' field in the OpenAPI Object.
// It contains metadata used for assembling a tag in the final OAS.
type Tag struct {
	// NOTE: the tag name is assumed to be implicit from the context (typically a map key).
	Description string // tag description
	Rank        *int   // control the location of the tag in the final OAS.
	// Lower values will put the tag higher up, i.e. indicate a higher importance.
	// Undefined values (= nil) will put the the tag at the bottom.
	DocLevel string // custom field to control which of the two OAS files the tag should go;
	// set to "basiconly", "advancedonly", or any other value for both
}

// Path corresponds with an OpenAPI path object to be kept in the 'paths' field in the OpenAPI
// Object. It contains metadata used for assembling a path in the final OAS.
type Path struct {
	Name   string // relative path to individual endpoint; must begin with a forward slash (/)
	Object any    // a generic object (i.e. map[string]any) to represent the
	// contents of the path object

	// NOTE: for now we assume that a given path is associated with exactly one operation (such as
	// get or post)
}

// OAPublisher is an interface that needs to be implemented by every entity that contributes
// contents to the OAS. Typically each time series type (implementing the timeseries.TimeSeries
// interface) constitutes such an entity.
type OAPublisher interface {

	// OAGetTags returns tags specific to this publisher.
	//
	// Returns (tags, nil) on success, otherwise (..., error).
	OAGetTags() (map[string]Tag, error)

	// OAGetDefs returns defs (reusable subschemas) specific to this publisher.
	//
	// Returns (defs, nil) on success, otherwise (..., error).
	OAGetDefs() (map[string]string, error)

	// OAGetPaths gets paths specific to this publisher.
	//
	// Returns (paths, nil) on success, otherwise (..., error).
	OAGetPaths() ([]Path, error)
}

// OAPublishers is the set of publishers for a given OAS.
type OAPublishers map[OAPublisher]struct{}

// AddPublisher registers a publisher in pubs.
func (pubs *OAPublishers) AddPublisher(pub OAPublisher) error {

	_, found := (*pubs)[pub]
	if found {
		return fmt.Errorf("publisher %v already registered", pub)
	}

	(*pubs)[pub] = struct{}{}

	return nil
}

// createFile writes the OAS file fname with the top-level objects version, info, tags,
// defs, and paths.
//
// Returns nil upon success, otherwise error.
func createFile(
	fname string, keepForDocLevel func(map[string]any) bool, version string,
	info map[string]any, tags any, defs map[string]string, paths any) error {

	var err error

	// --- BEGIN construct the OpenAPI Object -------------------------------------

	defs2 := map[string]any{}
	for key, val := range defs {
		var a any
		err := json.Unmarshal([]byte(val), &a)
		if err != nil {
			return fmt.Errorf("json.Unmarshal() failed: %v", err)
		}
		defs2[key] = a
	}

	oaObj := map[string]any{ // create initial object with version, info, and defs
		"openapi": version,
		"info":    info,
		"$defs":   defs2,
	}

	// set up tags specific to doc level
	dlTags := copySubset(tags, keepForDocLevel)
	if dlTags == nil {
		dlTags = []any{}
	}

	// set up paths specific to doc level
	dlPaths := copySubset(paths, keepForDocLevel)
	dlPathsSorted := sortArraysOnGroupKeys(dlPaths)
	if dlPathsSorted == nil {
		dlPathsSorted = map[string]any{}
	}

	oaObj["tags"] = dlTags         // set 'tags' field
	oaObj["paths"] = dlPathsSorted // set 'paths' field

	// --- END construct the OpenAPI Object -------------------------------------

	// --- BEGIN write the OpenAPI Object to file -------------------------------------

	bsOasObj, err := json.Marshal(oaObj)
	if err != nil {
		return fmt.Errorf("json.Marshal() failed: %v", err)
	}

	f, err := os.Create(fname)
	if err != nil {
		return fmt.Errorf("os.Create(%s) failed: %v", fname, err)
	}
	defer f.Close()

	_, err = f.WriteString(string(bsOasObj))
	if err != nil {
		return fmt.Errorf("f.WriteString() failed: %v", err)
	}

	// --- END write the OpenAPI Object to file -------------------------------------

	return nil
}

// sortTagsOnRank sorts tags on an optional 'rank' key (of *int type) so that tags with a lower
// rank end up at a lower index, while tags without any rank end up at a higher index.
func sortTagsOnRank(tags *[]any) error {

	errs := []error{}

	sort.Slice(*tags, func(i, j int) bool {
		tagIF := []any{(*tags)[i], (*tags)[j]}
		defined := []bool{false, false}
		rank := []int{-1, -1}

		for k := 0; k < 2; k++ {
			// attempt to extract any as map[]any into tag
			tag, ok := tagIF[k].(map[string]any)
			if !ok {
				errs = append(errs, fmt.Errorf(
					"tag %d not a map[string]any (type: %T)", k, tagIF[k]))
				return false
			}

			// attempt to extract tag["rank"] as *int
			if r, f := tag["rank"]; f {
				if val, ok := r.(*int); ok {
					if val != nil {
						defined[k] = true
						rank[k] = *val
					} // else undefined (i.e. defined[k] still false)
				} else {
					errs = append(errs, fmt.Errorf(
						"rank in tag[%d] not of type *int (type: %T)", k, r))
					return false
				}
			} // else undefined (i.e. defined[k] still false)
		}

		if defined[0] && defined[1] {
			return rank[0] < rank[1] // prioritize lower rank
		}

		if defined[0] {
			// assert(!defined[1])
			return true // prioritize defined over undefined
		}
		if defined[1] {
			// assert(!defined[0])
			return false // prioritize defined over undefined
		}

		// assert((!defined[0]) && (!defined  [1]))
		return true // no prioritization, so just choose the item at the first index
	})

	if len(errs) > 0 {
		// report only first error for now
		return fmt.Errorf("failed to sort tags on rank: %v", errs[0])
	}

	return nil
}

// getTagsDefsPaths gets the tags, defs, and paths objects from pubs.
//
// Returns (tags, defs, paths, nil) upon success, otherwise (..., ..., ..., error).
func (pubs *OAPublishers) getTagsDefsPaths() (
	[]any, map[string]string, map[string]any, error) {

	tagsObj := map[string]Tag{}
	defsObj := map[string]string{}
	paths := map[string]any{}

	// add publisher-specific contents
	for pub := range *pubs {

		// add tags
		tags, err := pub.OAGetTags()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("pub.OAGetTags() failed: %v", err)
		}
		//
		for tagName, tag := range tags {
			_, found := tagsObj[tagName]
			if found {
				return nil, nil, nil, fmt.Errorf("tag name %s already registered", tagName)
			}
			tagsObj[tagName] = tag
		}

		// add defs
		defs, err := pub.OAGetDefs()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("pub.OAGetDefs() failed: %v", err)
		}
		//
		for defName, def := range defs {
			_, found := defsObj[defName]
			if found {
				return nil, nil, nil, fmt.Errorf("def name %s already registered", defName)
			}
			defsObj[defName] = def
		}

		// add paths
		paths0, err := pub.OAGetPaths()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("pub.OAGetPaths() failed: %v", err)
		}
		//
		for _, path := range paths0 {
			_, found := paths[path.Name]
			if found {
				return nil, nil, nil, fmt.Errorf("path %s already registered", path.Name)
			}
			if path.Object == nil {
				return nil, nil, nil, fmt.Errorf("path %s does not cointain any object", path.Name)
			}
			paths[path.Name] = path.Object
		}
	}

	// convert tagsObj to array
	tags := []any{}
	for k, v := range tagsObj {
		tags = append(tags,
			map[string]any{
				"name":        k,
				"rank":        v.Rank,
				"description": common.NormalizeWhitespace(v.Description),
				"doclevel":    v.DocLevel,
			},
		)
	}

	err := sortTagsOnRank(&tags)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("sortTagsOnRank() failed: %v", err)
	}

	return tags, defsObj, paths, nil
}

// CreateFiles creates, for pubs, the OAS files for both basic and advanced documentation level.
// NOTE: this is applicable only to the non-EDR API.
//
// Returns nil upon  success, otherwise error.
func (pubs *OAPublishers) CreateFiles(fnameBasic, fnameAdvanced string) error {

	version := "3.0.3" // OpenAPI version
	info := map[string]any{
		"title": "Frost v1",
		"description": "Frost v1 is a RESTful Web API for storing and retrieving weather- and " +
			"climate observations.",
		"version": "1.0",
	}

	// populate tags, defs and paths
	tags, defs, paths, err := pubs.getTagsDefsPaths()
	if err != nil {
		return fmt.Errorf("getTagsDefsPaths() failed: %v", err)
	}

	// create file for basic documentation level
	err = createFile(fnameBasic, keepBasic, version, info, tags, defs, paths)
	if err != nil {
		return fmt.Errorf(
			"createFile() for basic doc level failed (fname: %s): %v", fnameBasic, err)
	}
	log.Println("created OpenAPI Specification (OAS) file for basic documentation level")

	// create file for advanced documentation level
	err = createFile(fnameAdvanced, keepAdvanced, version, info, tags, defs, paths)
	if err != nil {
		return fmt.Errorf(
			"createFile() for basic doc level failed (fname: %s): %v", fnameBasic, err)
	}
	log.Println("created OpenAPI Specification (OAS) file for advanced documentation level")

	return nil
}

// createEDRFile writes the OAS file fname for the EDR API with the top-level objects version,
// info, tags, defs, and paths.
//
// Returns nil upon success, otherwise error.
func createEDRFile(
	fname string, version string, info map[string]any, tags, defs,
	paths any) error {

	var err error

	// create the OpenAPI Object
	oaObj := map[string]any{
		"openapi": version,
		"info":    info,
		"tags":    tags,
		"defs":    defs,
		"paths":   paths,
	}

	// --- BEGIN write the OpenAPI Object to file -------------------------------------

	bsOasObj, err := json.Marshal(oaObj)
	if err != nil {
		return fmt.Errorf("json.Marshal() failed: %v", err)
	}

	f, err := os.Create(fname)
	if err != nil {
		return fmt.Errorf("os.Create(%s) failed: %v", fname, err)
	}
	defer f.Close()

	_, err = f.WriteString(string(bsOasObj))
	if err != nil {
		return fmt.Errorf("f.WriteString() failed: %v", err)
	}

	// --- END write the OpenAPI Object to file -------------------------------------

	return nil
}

// CreateEDRFile creates, for pubs, the OAS file for the EDR API.
func (pubs *OAPublishers) CreateEDRFile(fname string) error {

	version := "3.0.3" // OpenAPI version
	info := map[string]any{
		"title":       "Frost EDR API",
		"description": "The Frost EDR API",
		"version":     "1.0",
	}

	// populate tags, defs, and paths
	tags, defs, paths, err := pubs.getTagsDefsPaths()
	if err != nil {
		return fmt.Errorf("getToplevelParts() failed: %v", err)
	}

	// create file
	err = createEDRFile(fname, version, info, tags, defs, paths)
	if err != nil {
		return fmt.Errorf("createEDRFile() failed (fname: %s): %v", fname, err)
	}
	log.Println("created OpenAPI Specification (OAS) file for EDR API")

	return nil
}
