package testobscommon

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// UploadDataset uploads a dataset in an HTTP POST request to the Frost service at urlBase/path.
// The dataset 'dset' is passed in multipart/form field "dataset".
// Upon success (though not necessarily 200 Ok!), the function returns
// (status code, response body, nil), otherwise (-1, "", error).
func UploadDataset(urlBase, path, dset string) (int, string, error) {

	// assert(!strings.HasSuffix(urlBase, "/")) --- assumed to be checked already
	if strings.HasPrefix(path, "/") {
		return -1, "", fmt.Errorf("'/' prefix not allowed in path: %s", path)
	}
	url := fmt.Sprintf("%s/%s", urlBase, path)

	var requestBody bytes.Buffer

	_, err := requestBody.Write([]byte(dset))
	if err != nil {
		return -1, "", fmt.Errorf("requestBody.Write() failed: %v", err)
	}

	client := &http.Client{}
	resp, err := client.Post(url, "application/json", &requestBody)
	defer func() {
		if (resp != nil) && (resp.Body != nil) {
			resp.Body.Close()
		}
	}()
	if err != nil {
		return -1, "", fmt.Errorf("failed to send POST request: %v", err)
	}
	if resp == nil {
		return -1, "", fmt.Errorf("unexpected empty response from POST request (err == nil!)")
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, "", fmt.Errorf("io.readAll(resp.Body) failed: %v", err)
	}

	return resp.StatusCode, string(respBody), nil
}

// UploadTsTypeDatasetFromFile uploads to the Frost instance a dataset of time series useful for
// testing. The dataset is extracted as JSON from file fname. The time series headers and
// observations are uploaded by calling respectively the endpoints '.../ts/create' and
// '.../put' for the time series type tsType.
//
// Returns nil upon success, otherwise error.
func UploadTsTypeDatasetFromFile(urlBase, tsType, fname string) error {

	extractDataset := func() (string, error) {

		file, err := os.Open(fname)
		if err != nil {
			return "", fmt.Errorf("os.Open(%s) failed: %v", fname, err)
		}
		defer file.Close()

		jsonData, err := io.ReadAll(file)
		if err != nil {
			return "", fmt.Errorf("io.ReadAll() failed: %v", err)
		}

		return string(jsonData), nil
	}

	dset, err := extractDataset()
	if err != nil {
		return fmt.Errorf("extractDataset() failed: %v", err)
	}

	doUploadDataset := func(pathPart string) error {

		statusCode, respBody, err := UploadDataset(
			urlBase, fmt.Sprintf("api/v1/obs/%s/%s", tsType, pathPart), dset)
		if err != nil {
			return fmt.Errorf(
				"UploadDataset() failed: %v (status code: %d; response body: %s)",
					err, statusCode, respBody)
		}
		if statusCode != http.StatusOK {
			return fmt.Errorf(
				"UploadDataset() returned status code != 200 OK: %d (response body: %s)",
					statusCode, respBody)
		}

		return nil
	}

	for _, pathPart := range []string{"ts/create", "put"} {
		err = doUploadDataset(pathPart)
		if err != nil {
			return fmt.Errorf("doUploadDataset(%s) failed: %v", pathPart, err)
		}
	}

	return nil
}

// DownloadDataset downloads a dataset from the Frost service at urlBase/path.
// Upon success (though not necessarily 200 Ok!), the function returns
// (status code, response body, nil), otherwise (-1, "", error).
func DownloadDataset(urlBase, path, query string) (int, string, error) {

	// assert(!strings.HasSuffix(urlBase, "/")) --- assumed to be checked already
	if strings.HasPrefix(path, "/") {
		return -1, "", fmt.Errorf("'/' prefix not allowed in path: %s", path)
	}
	if strings.HasPrefix(query, "?") {
		return -1, "", fmt.Errorf("'?' prefix not allowed in query: %s", query)
	}

	url := fmt.Sprintf("%s/%s?%s", urlBase, path, query)

	newReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return -1, "", fmt.Errorf("http.NewRequest() failed: %v", err)
	}

	//newReq.Header = http.Header{}

	client := &http.Client{}
	resp, err := client.Do(newReq)
	if err != nil {
		return -1, "", fmt.Errorf("client.Do() failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, "", fmt.Errorf("io.readAll(resp.Body) failed: %v", err)
	}

	return resp.StatusCode, string(body), nil
}

// TestGetRequest issues an HTTP GET request against a Frost service, using the URL defined by
// urlBase, path and query, and validates the actual response code against expStatusCode.
//
// Returns nil upon success, otherwise error (including mismatch between actual and expected
// status code).
func TestGetRequest(urlBase, path, query string, expStatusCode int) error {

	// assert(!strings.HasSuffix(urlBase, "/")) --- assumed to be checked already
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("'/' prefix not allowed in path: %s", path)
	}
	if strings.HasPrefix(query, "?") {
		return fmt.Errorf("'?' prefix not allowed in query: %s", query)
	}

	url := fmt.Sprintf("%s/%s?%s", urlBase, path, query)

	client := &http.Client{}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("client.Get() failed: %v", err)
	}
	defer resp.Body.Close()

	getRespBodyMsg := func() string {
		const maxRBSize = 10000
		body := make([]byte, maxRBSize)
		var rbMsg string
		n, err := resp.Body.Read(body)
		if (err != nil) && (err != io.EOF) {
			rbMsg = fmt.Sprintf(
				": ((resp.Body.Read() failed with other error than EOF: %v))", err)
		} else {
			rbMsg = fmt.Sprintf(" (%d of max %d bytes): %s", n, maxRBSize, string(body))
		}
		return rbMsg
	}

	if resp.StatusCode != expStatusCode {
		return fmt.Errorf(
			"actual status code (%d %s) != expected status code (%d %s)\n"+
			"\tURL base:             %s\n"+
			"\tpath:                 %s\n"+
			"\tquery:                %s\n"+
			"\texpected status code: %d\n"+
			"\tresponse body%s",
			resp.StatusCode, http.StatusText(resp.StatusCode), expStatusCode,
			http.StatusText(expStatusCode), urlBase, path, query, expStatusCode, getRespBodyMsg())
	}

	return nil
}

// GetRequestTestDef defines a basic GET request made against a Frost instance.
// The result of the request is validated wrt. expected response code.
// TODO: validate result against other properties like response body, response header etc.
type GetRequestTestDef struct {
	Name string // test name
	Path string // path part of URL
	Query string // query part of URL
	StatusCode int // expected HTTP response code
}

// RunGetRequestTests runs the tests defined by testDefs against the Frost instance
// identified by urlBase. Test results are reflected through t.
func RunGetRequestTests(t *testing.T, urlBase string, testDefs []GetRequestTestDef) {

	for _, td := range testDefs {
		err := TestGetRequest(urlBase, td.Path, td.Query, td.StatusCode)
		if err != nil {
			t.Errorf("%s failed: %v", td.Name, err)
		} else {
			t.Logf("%s passed", td.Name)
		}
	}
}
