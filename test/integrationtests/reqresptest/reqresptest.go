package reqresptest

// This package implements a simple framework for testing that requests
// to a REST API produce the expected responses.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	localjson "gitlab.met.no/frost/frost/internal/common/json"
)

// GetRequest represents an HTTP GET request.
type GetRequest struct {
	Path   string
	Query  string
	Header http.Header
}

// PostRequest represents an HTTP POST request.
type PostRequest struct {
	Path   string
	Body   string
	Header http.Header
}

// Response represents the response to an HTTP GET/POST request.
type Response struct {
	StatusCode int // HTTP status code
	Body       string
	Header     http.Header
}

// CanonicalHeader converts header into a http.Header where the keys are in canonical form.
func CanonicalHeader(header map[string][]string) http.Header {
	cheader := http.Header{}
	for key, values := range header {
		cheader[http.CanonicalHeaderKey(key)] = values
	}
	return cheader
}

// InitInfo defines how to reset the service for a particular test.
// If ClearPath is non-empty, a GET request for ClearPath is sent.
// If TsCreatePath is non-empty, a POST request for TsCreatePath and Dataset as payload is sent
//
//	to create time series (NOTE: in this case, the observations array may be emptied from Dataset
//	before uploading since it is irrelevant for creating time series).
//
// If PutPath is non-empty, a POST request for PutPath and Dataset as payload is sent to
//
//	write observations.
type InitInfo struct {
	ClearPath    string
	TsCreatePath string
	PutPath      string
	Dataset      string
}

// ResponseValidator is a general validation function that decides if a response for some reason
// is not valid with respect to its corresponding request.
// Returns empty string if no problems are found, otherwise a descriptive summary of what went
// wrong.
type ResponseValidator func(GetRequest, Response) string

// GetTest represents a test of a specific HTTP GET request.
type GetTest struct {
	Name             string
	Init             InitInfo
	Request          GetRequest
	ExpectedResponse Response
	Validate         ResponseValidator
}

// ReqRespPair represent an HTTP GET request and its expected response.
type ReqRespPair struct {
	Description      string
	Request          GetRequest
	ExpectedResponse Response
	Validate         ResponseValidator
}

// GetTestSequence represents a test of a specific sequence of HTTP GET requests.
type GetTestSequence struct {
	Name         string
	Init         InitInfo
	ReqRespPairs []ReqRespPair
}

// sendGetRequest sends an HTTP GET request and returns the response.
func sendGetRequest(urlBase string, req GetRequest) (Response, error) {
	url := fmt.Sprintf("%s/%s%s", urlBase, req.Path, req.Query)

	newReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Response{}, fmt.Errorf("http.NewRequest() failed: %v", err)
	}

	newReq.Header = req.Header

	client := &http.Client{}
	resp, err := client.Do(newReq)
	if err != nil {
		return Response{}, fmt.Errorf("client.Do() failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("io.readAll(resp.Body) failed: %v", err)
	}

	return Response{
		StatusCode: resp.StatusCode,
		Body:       string(body),
		Header:     resp.Header,
	}, nil
}

// sendPostRequest sends an HTTP POST request and returns the response.
func sendPostRequest(urlBase string, req PostRequest) (Response, error) {
	url := fmt.Sprintf("%s/%s", urlBase, req.Path)
	_ = url
	// TODO: send a POST request and receive response (req.Body must be passed in the
	// request body ...) ...

	return Response{}, nil // FOR NOW
}

// matchResponses returns (<non-empty reason>, nil) if expected and actual responses differ
// (different status codes OR expected body not a JSON subset of actual body OR
// at least one of the expected header values not found), otherwise ("", nil) if they were
// equivalent, or ("", error) if an error occurred.
func matchResponses(expected, actual Response) (string, error) {

	// STEP 1: match status codes

	if expected.StatusCode != actual.StatusCode {
		return fmt.Sprintf(
			"different status codes (expected: %d != actual: %d)",
			expected.StatusCode, actual.StatusCode), nil
	}

	// STEP 2: match bodies

	var err error
	var val1, val2 map[string]interface{}

	err = json.Unmarshal([]byte(expected.Body), &val1)
	if err != nil {
		return "", fmt.Errorf(
			"failed to unmarshal expected.Body into map[string]interface{}: %v", err)
	}

	err = json.Unmarshal([]byte(actual.Body), &val2)
	if err != nil {
		return "", fmt.Errorf(
			"failed to unmarshal actual.Body into map[string]interface{}: %v", err)
	}

	isSubset, reason, err := localjson.IsJSONSubMatch(val1, val2)
	if err != nil {
		return "", fmt.Errorf("isJSONSubMatch() failed: %v", err)
	}
	if !isSubset {
		return fmt.Sprintf(
			"expected response body not a JSON subset of actual "+
				"response body:\nreason: %s", reason), nil
	}

	// STEP 3: match headers

	//fmt.Printf("\n----->expected.Header: >%v<\n", expected.Header)
	for expKey, expValues := range expected.Header {
		//fmt.Printf("\n\n>>>expKey: >%s<; expValues: >%v<\n\n", expKey, expValues)
		expValue := expected.Header.Get(expKey)
		// if expValue == "" {
		// 	return fmt.Sprintf(
		// 		"expected response header key '%s' not found "+
		// 			"(values for this key: %v; full header: %v)",
		// 		expKey, expValues, expected.Header), nil
		// }
		_ = expValues

		actValue := actual.Header.Get(expKey)
		// if actValue == "" {
		// 	return fmt.Sprintf("actual response header key '%s' not found", expKey), nil
		// }

		if expValue != actValue {
			return fmt.Sprintf(
				"expected value '%s' not found for response header key '%s' (actual value: '%s')",
				expValue, expKey, actValue), nil
		}
	}

	return "", nil // responses matched according to expectation
}

// uploadDataset uploads a dataset as content type application/json using HTTP POST.
func uploadDataset(urlBase, path string, dataset []byte) error {

	requestBody := bytes.NewBuffer(dataset)

	url := fmt.Sprintf("%s%s", urlBase, path)
	req, err := http.NewRequest("POST", url, requestBody)
	if err != nil {
		return fmt.Errorf("failed to create POST request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		resp.Body.Close()
		return fmt.Errorf("failed to send POST request: %v", err)
	}

	if resp == nil {
		return fmt.Errorf("unexpected empty response from POST request (err == nil!)")
	}

	if resp.StatusCode != http.StatusOK {
		if resp.Body == nil {
			return fmt.Errorf("empty response body from POST request")
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("io.readAll(resp.Body) failed: %v", err)
		}

		return fmt.Errorf("non-200 status code from POST request: %d"+
			"\nresponse body: %s", resp.StatusCode, body)
	}

	return nil
}

// reset clears the service (if requested) and uploads an initial dataset (if requested)
func reset(urlBase string, initInfo InitInfo) error {

	if strings.TrimSpace(initInfo.ClearPath) != "" { // clear service
		resp, err := sendGetRequest(urlBase, GetRequest{
			Path: strings.TrimSpace(initInfo.ClearPath),
		})
		if err != nil {
			return fmt.Errorf("sendGetRequest() failed for clearing service: %v", err)
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf(
				"clearing service failed with non-200 status code: %d"+
					"\nresponse body: %s", resp.StatusCode, resp.Body)
		}
	} else { // skip clearing service
		//
	}

	if strings.TrimSpace(initInfo.TsCreatePath) != "" { // create time series
		// ### FOR NOW: don't replace any 'observations' array in the dataset with
		//   an empty array. It should work all the same (Frost will simply ignore any
		//   observations).
		path := strings.TrimSpace(initInfo.TsCreatePath)
		err := uploadDataset(urlBase, path, []byte(initInfo.Dataset))
		if err != nil {
			return fmt.Errorf("uploadDataset(...%s...) failed: %v", path, err)
		}
	} else { // skip creating time series
		//
	}

	if strings.TrimSpace(initInfo.PutPath) != "" { // write observations
		path := strings.TrimSpace(initInfo.PutPath)
		err := uploadDataset(urlBase, path, []byte(initInfo.Dataset))
		if err != nil {
			return fmt.Errorf("uploadDataset(...%s...) failed: %v", path, err)
		}
	} else { // skip writing observations
		//
	}

	return nil
}

// RunGetTest runs the test of a specific HTTP GET request.
func RunGetTest(t *testing.T, urlBase string, gtest GetTest) {

	err := reset(urlBase, gtest.Init)
	if err != nil {
		t.Errorf("reset() failed: %v", err)
		return
	}

	req := gtest.Request
	expResp := gtest.ExpectedResponse

	actResp, err := sendGetRequest(urlBase, req)
	if err != nil {
		t.Errorf("sendGetRequest() failed: %v", err)
		return
	}

	reason, err := matchResponses(expResp, actResp)
	if err != nil {
		t.Errorf("matchResponses() failed: %v", err)
		return
	}

	if reason != "" {
		t.Errorf("expected response doesn't match actual response")
		t.Errorf("\treason: %s", reason)
		t.Errorf("\texpected response: %+v", expResp)
		t.Errorf("\tactual response: %+v", actResp)
	}

	if gtest.Validate != nil {
		if reason := gtest.Validate(gtest.Request, actResp); reason != "" {
			t.Errorf("gtest.Validate() failed: %v", reason)
		}
	}
}

// RunGetTestSequence runs the test of a specific sequence of HTTP GET requests.
func RunGetTestSequence(t *testing.T, urlBase string, gtests GetTestSequence) {

	if len(gtests.ReqRespPairs) == 0 {
		t.Logf("WARNING: no request/response pairs found!")
		return
	}

	err := reset(urlBase, gtests.Init)
	if err != nil {
		t.Errorf("reset() failed: %v", err)
		return
	}

	formatDescr := func(prefix, descr string) string {
		trimmedDescr := strings.TrimSpace(descr)
		if trimmedDescr != "" {
			return fmt.Sprintf("%s%s", prefix, trimmedDescr)
		}
		return ""
	}

	for i, rrp := range gtests.ReqRespPairs {

		req := rrp.Request
		expResp := rrp.ExpectedResponse
		fmtDescr := formatDescr(": ", rrp.Description)

		actResp, err := sendGetRequest(urlBase, req)
		if err != nil {
			t.Errorf(
				"sendGetRequest() failed\n"+
					"\t(request %d:%d)%s\n"+
					"\terror: %v", i+1, len(gtests.ReqRespPairs), fmtDescr, err)
			return
		}

		reason, err := matchResponses(expResp, actResp)
		if err != nil {
			t.Errorf(
				"matchResponses() failed\n"+
					"\t(request %d:%d)%s\n"+
					"\terror: %v", i+1, len(gtests.ReqRespPairs), fmtDescr, err)
			return
		}

		if reason != "" {
			t.Errorf(
				"expected response doesn't match actual response\n"+
					"\t(request %d:%d)%s", i+1, len(gtests.ReqRespPairs), fmtDescr)
			t.Errorf("\treason: %s", reason)
			t.Errorf("\texpected response: %+v", expResp)
			t.Errorf("\tactual response: %+v", actResp)
		}

		if rrp.Validate != nil {
			if reason := rrp.Validate(rrp.Request, actResp); reason != "" {
				t.Errorf("rrp.Validate() failed: %v", reason)
			}
		}
	}
}
