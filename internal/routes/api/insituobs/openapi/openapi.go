// Package openapi ... TO BE DOCUMENTED.
package openapi

// OpenAPI code for the /api/v1/obs/* routes.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"gitlab.met.no/frost/frost/internal/common"
	"gitlab.met.no/frost/frost/internal/openapi"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
)

// createTsObject creates the Object part of the openapi.Path for one of the
// .../obs/.../ts/* operations create, update, or delete.
//
// Returns (object, nil) upon success, otherwise (..., error).
func createTsOpObject(
	hdrIDSchema, hdrExtraSchema, summary, operationID, tagName, descr, okDescr, okSchema string) (
		interface{}, error) {

	var err error

	// --- BEGIN create toplevel object ------------------------
	obj0 := map[string]interface{}{
		"doclevel": openapi.DocLevelAdvancedOnly(),
		"summary": summary,
		"operationId": operationID,
		"tags": []string{tagName},
	}

	// add 'description'
	obj0["description"] = common.NormalizeWhitespace(descr)
	// --- END create toplevel object ------------------------

	// --- BEGIN add 'requestBody' ------------------------
	requestBodyS := fmt.Sprintf(`{
		"content": {
			"application/json": {
				"schema": %s
			}
		}
	}`, dataset.DatasetSchemaTemplateTsANY())

	requestBodyS = strings.ReplaceAll(
		requestBodyS, dataset.HdrIDPlaceholder(), hdrIDSchema)
	requestBodyS = strings.ReplaceAll(
		requestBodyS, dataset.HdrExtraPlaceholder(), hdrExtraSchema)

	var requestBody interface{}
	err = json.Unmarshal([]byte(requestBodyS), &requestBody)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal(requestBody) failed: %v", err)
	}

	obj0["requestBody"] = requestBody
	// --- END add 'requestBody' ------------------------

	// --- BEGIN add 'responses' ------------------------
	responsesS := fmt.Sprintf(`{
		"200": {
			"description": "%s",
			"content": {
				"application/json": {
					"schema": %s
				}
			}
		},
		%s
	}`, okDescr, okSchema, openapi.FormatGenericErrorResponses(
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusInternalServerError,
	))

	var responses interface{}
	err = json.Unmarshal([]byte(responsesS), &responses)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal(responses) failed: %v", err)
	}

	obj0["responses"] = responses
	// --- END add 'responses' ------------------------

	return map[string]interface{}{"post": obj0}, nil
}

// createTsOkSchema creates the schema for a 200 Ok response from one of the
// .../obs/.../ts/* operations create, update, or delete.
func createTsOkSchema(titleText, appliedDescrText string) string {

	return fmt.Sprintf(`{
        "type": "object",
        "title": "Time series successfully rejected/accepted/applied for the purpose of %s.",
        "properties": {
           "accepted": {
              "description": "number of time series accepted, i.e. not rejected",
              "example": 1,
              "type": "integer"
           },
           "applied": {
              "description": "number of time series applied, i.e. %s",
              "example": 1,
              "type": "integer"
           },
           "rejected": {
              "description": "number of time series rejected, e.g. due to write restrictions",
              "example": 0,
              "type": "integer"
           }
        },
        "required": [
           "rejected",
           "accepted",
           "applied"
        ]
    }`, titleText, appliedDescrText)
}

// CreateTsCreateObject is a convenience wrapper around createTsOpObject.
func CreateTsCreateObject(
	tsType, hdrIDSchema, hdrExtraSchema, summary, operationID, tagName string) (interface{}, error) {

	descr := fmt.Sprintf(`
		This method creates a set of time series of type '%s'. Nothing happens for any time series
    	that already exists. The 'observations' array in the dataset is irrelevant and can be empty
    	(i.e. [] or null).
	`, tsType)

	okDescr := "One or more new time series were either successfully created or already existing."

	return createTsOpObject(
		hdrIDSchema, hdrExtraSchema, summary, operationID, tagName, descr, okDescr,
		createTsOkSchema("creation", "created (or ensured already existing)"))
}

// CreateTsDeleteObject is a convenience wrapper around createTsObject.
func CreateTsDeleteObject(
	tsType, hdrIDSchema, hdrExtraSchema, summary, operationID, tagName string) (
		interface{}, error) {

	descr := fmt.Sprintf(`
		This method deletes a set of time series of type '%s' (along with all observations,
		so be careful!). Nothing happens for any time series that doesn't exist. The 'observations'
		array in the dataset is irrelevant and can be empty (i.e. [] or null).
	`, tsType)

	okDescr := "One or more time series were either successfully deleted or already non-existing."

	return createTsOpObject(
		hdrIDSchema, hdrExtraSchema, summary, operationID, tagName, descr, okDescr,
		createTsOkSchema("deletion", "deleted"))
}

// CreateTsUpdateObject is a convenience wrapper around createTsObject.
func CreateTsUpdateObject(
	tsType, hdrIDSchema, hdrExtraSchema, summary, operationID, tagName string) (
		interface{}, error) {

	descr := fmt.Sprintf(`
		This method updates the 'extra' part of a set of time series of type '%s'.
		Nothing happens for any time series that doesn't exist. The 'observations' array in
		the dataset is irrelevant and can be empty (i.e. [] or null).
	`, tsType)

	okDescr := "One or more time series were either successfully updated or did not exist."

	return createTsOpObject(
		hdrIDSchema, hdrExtraSchema, summary, operationID, tagName, descr, okDescr,
		createTsOkSchema("update", "updated"))
}

// CreatePutObject creates the Object part of the openapi.Path for an .../obs/.../put operation.
//
// Returns (object, nil) upon success, otherwise (..., error).
func CreatePutObject(
	tsType, obsBodyDescr, dsetExample, hdrIDSchema, hdrExtraSchema, obsBodySchema, operationID,
	tagName string) (interface{}, error) {

	var err error

	// --- BEGIN create toplevel object ------------------------
	obj0 := map[string]interface{}{
		"doclevel": openapi.DocLevelAdvancedOnly(),
		"summary": "Insert, update, or delete observations in existing time series",
		"operationId": operationID,
		"tags": []string{tagName},
	}

	// add 'description'
	descr := fmt.Sprintf(`
		<html>
		This method allows a dataset of time series type '%s' to be applied to the Frost service.
		Depending on its time <em>T</em> and body <em>B</em>, each observation in the dataset has
		the following effect on the storage:
		<table>
			<tr><th>Condition</th><th>Effect</th></tr>
			<tr><td>No observation at <em>T</em> exists.</td>
			    <td>The observation is inserted.</td></tr>
			<tr><td>An observation at <em>T</em> exists already.</td>
			    <td>The observation body is updated with <em>B</em>.</td></tr>
			<tr><td><em>B</em> is empty (i.e. [] or null).</td>
			    <td>The observation at <em>T</em> (if any) is deleted.</td></tr>
		</table>
		%s
		</html>
	`, tsType, obsBodyDescr)
	obj0["description"] = common.NormalizeWhitespace(descr)
	// --- END create toplevel object ------------------------

	// --- BEGIN add 'requestBody' ------------------------
	requestBodyS := common.NormalizeWhitespace(fmt.Sprintf(`{
		"content": {
			"application/json": {
				"schema": %s,
				"example": %s
			}
		}
	}`, dataset.DatasetSchemaTemplatePutGet(), dsetExample))

	requestBodyS = strings.ReplaceAll(requestBodyS, dataset.HdrIDPlaceholder(), hdrIDSchema)
	requestBodyS = strings.ReplaceAll(requestBodyS, dataset.HdrExtraPlaceholder(), hdrExtraSchema)
	requestBodyS = strings.ReplaceAll(requestBodyS, dataset.ObsBodyPlaceholder(), obsBodySchema)

	var requestBody interface{}
	err = json.Unmarshal([]byte(requestBodyS), &requestBody)
	if err != nil {
		return fmt.Errorf("json.Unmarshal(requestBody) failed: %v", err), nil
	}

	obj0["requestBody"] = requestBody
	// --- END add 'requestBody' ------------------------

	// --- BEGIN add 'responses' ------------------------
	okDescr := `Observations were successfully inserted/updated/deleted in existing time series.`
	responsesS := fmt.Sprintf(`{
		"200": {
			"description": "%s",
			"content": {
				"application/json": {
					"schema": {
						"title": "Observations successfully uploaded.",
						"type": "object",
						"properties": {
							"inserted": {
								"description": "number of observations successfully inserted",
								"example": 123,
								"type": "integer"
							},
							"updated": {
								"description": "number of observations successfully updated",
								"example": 123,
								"type": "integer"
							},
							"deleted": {
								"description": "number of observations successfully deleted",
								"example": 123,
								"type": "integer"
							}
						},
						"required": [
							"inserted",
							"updated",
							"deleted"
						]
					}
				}
			}
		},
		%s
	}`, okDescr, openapi.FormatGenericErrorResponses(
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusInternalServerError,
	))

	var responses interface{}
	err = json.Unmarshal([]byte(responsesS), &responses)
	if err != nil {
		return fmt.Errorf("json.Unmarshal(responses) failed: %v", err), nil
	}

	obj0["responses"] = responses
	// --- END add 'responses' ------------------------

	return map[string]interface{}{"post": obj0}, nil
}

// getGetPathCommonParameters returns the subset of the 'parameters' part that is common to all
// time series types in /api/v1/obs/<time series type>/get operations.
func getGetPathCommonParameters() string {

	return common.NormalizeWhitespace(`[
		{
			"name": "time",
			"docgroup": "when",
			"required": false,
			"in": "query",
			"tags": [
				"when"
			],
			"schema": {
				"type": "string"
			},
			"example": "latest",
			"description": "A [time specification](/docs/parameters#time) to select relevant
				observation times. Either a time range formated as
				\"2020-01-01T00:00:00Z/2020-01-02T23:59:59Z\", or the keyword 'latest' can be used
				(for the latter case, see also latestmaxage and latestlimit)."
		},
		{
			"name": "latestmaxage",
			"doclevel": "advancedonly",
			"docgroup": "when",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "Used for a [time specification](/docs/parameters#latestmaxage) in
				'latest' mode, this is the maximum age (relative to the time of the request) that
				the most recent observation can have in order to contribute to the result. Specify
				either a number of seconds or an [ISO 8601 duration]
				(https://en.wikipedia.org/wiki/ISO_8601#Durations).
				__Default value__: PT3H"
		},
		{
			"name": "latestlimit",
			"doclevel": "advancedonly",
			"docgroup": "when",
			"required": false,
			"in": "query",
			"schema": {
				"type": "integer",
				"format": "int32"
			},
			"description": "Used for a [time specification](/docs/parameters#latestlimit) in
				'latest' mode, this is the maximum number of observations that may be included
				(if incobs==true) in the result for each contributing time series.
				__Default value__: 1"
		},
		{
			"name": "itemlimit",
			"doclevel": "advancedonly",
			"docgroup": "output",
			"required": false,
			"in": "query",
			"schema": {
				"type": "integer"
			},
			"description": "The maximum number of time series headers and individual observations
				that the response can contain. The effective limit to use will be
				min(itemlimit, &lt;limit defined by server&gt;) if itemlimit is specified, and
				&lt;limit defined by server&gt; if not."
		},
		{
			"name": "nearest",
			"docgroup": "where",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "A [geo search](/docs/parameters#nearest) parameter to look for
				observations around a geographic point.
				__Example__: {\"maxdist\":7.5,\"maxcount\":3,\"points\":[{\"lon\":10.72,
				\"lat\":59.94275}]}"
		},
		{
			"name": "polygon",
			"doclevel": "both",
			"docgroup": "where",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "A [geo search](/docs/parameters#polygon) parameter to look for
				observations inside a polygon.
				__Example__: [{\"lat\":59.93,\"lon\":10.05},{\"lat\":59.93,\"lon\":11},
				{\"lat\":60.25,\"lon\":10.77}]"
		},
		{
			"name": "inside",
			"doclevel": "advancedonly",
			"docgroup": "where",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "A [geo search](/docs/parameters#inside) parameter used for selecting
				observations inside a region.
				__Example__: [{\"type\":\"circle\",\"lon\":10.05,\"lat\":59.93,\"radius\":7.5}]"
		},
		{
			"name": "outside",
			"doclevel": "advancedonly",
			"docgroup": "where",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "A [geo search](/docs/parameters#outside) parameter used for selecting
				observations outside a region.
				__Example__: [{\"type\":\"circle\",\"lon\":10.05,\"lat\":59.93,\"radius\":7.5}]"
		},
		{
			"name": "geopostype",
			"doclevel": "advancedonly",
			"docgroup": "where",
			"required": false,
			"in": "query",
			"schema": {
				"type": "string"
			},
			"description": "A [geo search](/docs/parameters#geopostype) parameter used to decide
				whether to consider (e.g. apply geo filtering to) geo positions in time series
				headers (\"stationary\") or observation bodies (\"mobile\").
				__Default value__: stationary"
		},
		{
			"name": "incobs",
			"docgroup": "output",
			"required": false,
			"in": "query",
			"schema": {
				"type": "boolean",
				"default": "false"
			},
			"description": "Specify false if you only want time series headers (= metadata that
				don't vary with observation time) in the response. Specify true to additionally get
				observation values and metadata that vary with observation time."
		}
	]`)
}

// getGetPathResponses returns the 'responses' part of an /api/v1/obs/<time series type>/get
// operation.
func getGetPathResponses(hdrIDSchema, hdrExtraSchema, obsBodySchema string) string {

	okSchema := dataset.DatasetSchemaTemplatePutGet()
	okSchema = strings.ReplaceAll(okSchema, dataset.HdrIDPlaceholder(), hdrIDSchema)
	okSchema = strings.ReplaceAll(okSchema, dataset.HdrExtraPlaceholder(), hdrExtraSchema)
	okSchema = strings.ReplaceAll(okSchema, dataset.ObsBodyPlaceholder(), obsBodySchema)

	return common.NormalizeWhitespace(fmt.Sprintf(`{
		"200": {
			"description": "Time series headers and (optionally) observations were successfully
				downloaded.",
			"content": {
				"application/json": {
					"schema": %s
				}
			}
		},
		%s
	}`, okSchema, openapi.FormatGenericErrorResponses(
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
	)))
}

// CreateGetObject creates the Object part of the openapi.Path for an .../obs/.../get operation.
//
// Returns (object, nil) upon success, otherwise (..., error).
func CreateGetObject(
	tsType, params, hdrIDSchema, hdrExtraSchema, obsBodySchema, operationID,
	tagName, docLevel string) (interface{}, error) {

	var err error

	// --- BEGIN define 'parameters' ------------------------
	var parameters []interface{}
	err = json.Unmarshal([]byte(params), &parameters)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal(parameters) failed: %v", err)
	}

	var commonParameters []interface{}
	err = json.Unmarshal([]byte(getGetPathCommonParameters()), &commonParameters)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal(commonParameters) failed: %v", err)
	}

	parameters = append(parameters, commonParameters...) // merge
	// --- END define 'parameters' ------------------------

	// --- BEGIN define 'responses' ------------------------
	var responses interface{}
	err = json.Unmarshal(
		[]byte(getGetPathResponses(hdrIDSchema, hdrExtraSchema, obsBodySchema)), &responses)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal(responses) failed: %v", err)
	}
	// --- END define 'responses' ------------------------

	// --- BEGIN create toplevel object ------------------------
	obj0 := map[string]interface{}{
		"doclevel": docLevel,
		"summary": "Get time series of observations.",
		"description": fmt.Sprintf(
			"Retrieve time series headers and (optionally) observations from time series type "+
			"'%s'.",
			tsType),
		"operationId": operationID,
		"tags": []string{tagName},
		"parameters": parameters,
		"responses": responses,
	}
	// --- END create toplevel object ------------------------

	return map[string]interface{}{"get": obj0}, nil
}

// createStatusOkSchema creates the schema for a 200 Ok response from an .../obs/.../status
// operation.
func createStatusOkSchema(tstype, descrText string) string {

	return fmt.Sprintf(`{
        "type": "object",
        "title": "Internal status relevant for time series type '%s' successfully downloaded.",
        "properties": {
            "status": {
				"type": "object",
			    "properties": {
					"description": {
						"type": "string",
						"description": "%s"
					}
			    },
		        "required": [
			        "description"
        		]
            }
        },
        "required": [
           "status"
        ]
    }`, tstype, descrText)
}

// CreateStatusObject creates the Object part of the openapi.Path for an .../obs/.../status
// operation.
//
// Returns (object, nil) upon success, otherwise (..., error).
func CreateStatusObject(tsType, operationID, tagName, okDescr string) (interface{}, error) {

	var err error

	// --- BEGIN create toplevel object ------------------------
	obj0 := map[string]interface{}{
		"doclevel": openapi.DocLevelAdvancedOnly(),
		"summary": "Get internal status",
		"operationId": operationID,
		"tags": []string{tagName},
	}

	// add 'description'
	descr := fmt.Sprintf(`
		<html>
		This method gets status relevant for time series type '%s'.
		<br/>
		<b>NOTE:</b> this is intended for internal use only.
		</html>
	`, tsType)
	obj0["description"] = common.NormalizeWhitespace(descr)
	// --- END create toplevel object ------------------------

	// --- BEGIN add 'responses' ------------------------
	responsesS := fmt.Sprintf(`{
		"200": {
			"description": "Status was successfully downloaded.",
			"content": {
				"application/json": {
					"schema": %s
				}
			}
		}
	}`, createStatusOkSchema(tsType, okDescr))

	var responses interface{}
	err = json.Unmarshal([]byte(responsesS), &responses)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal(responses) failed: %v", err)
	}

	obj0["responses"] = responses
	// --- END add 'responses' ------------------------

	return map[string]interface{}{"get": obj0}, nil
}
