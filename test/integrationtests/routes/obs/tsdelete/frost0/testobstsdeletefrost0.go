package testobstsdeletefrost0

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	testobscommon "gitlab.met.no/frost/frost/test/integrationtests/routes/obs/common"
)

// Tests deletion of a non-existent time series.
func testDeleteWithMalformedDataset(t *testing.T, urlBase string) {
	dset := `malformed dataset`

	statusCode, respBody, err := testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/ts/delete", dset)
	if err != nil {
		t.Errorf("testobscommon.UploadDataset(.../ts/delete) failed: %v", err)
		return
	}
	if statusCode != http.StatusBadRequest {
		t.Errorf(fmt.Sprintf("expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusBadRequest, statusCode, respBody))
	}
}

// Tests deletion of no time series.
func testDeleteNoTimeSeries(t *testing.T, urlBase string) {
	dset := `{
		"tstype": "frost0",
		"tseries": []
	}
	`
	statusCode, respBody, err := testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/ts/delete", dset)
	if err != nil {
		t.Errorf("testobscommon.UploadDataset(.../ts/delete) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
	}
}

// Tests deletion of a non-existent time series.
func testDeleteNonExistingTimeSeries(t *testing.T, urlBase string) {
	dset := `{
		"tstype": "frost0",
		"tseries": [
			{
				"header": {
					"id": {
						"source": "no such source",
						"sensorLevel": "no such sensor level",
						"element": "no such element"
					}
				},
				"observations": []
			}
		]
	}
	`
	statusCode, respBody, err := testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/ts/delete", dset)
	if err != nil {
		t.Errorf("testobscommon.UploadDataset(.../ts/delete) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
	}
}

// Tests deletion of an existent time series without any observations.
func testDeleteExistingTimeSeriesWithoutObs(t *testing.T, urlBase string) {
	var err error

	// create dataset for a new time series (guaranteed not to exist already!)
	buuid, err := exec.Command("uuidgen").Output()
	if err != nil {
		t.Errorf("exec.Command(\"uuidgen\") failed: %v", err)
		return
	}
	uuid := strings.TrimSpace(string(buuid))
	dset := fmt.Sprintf(`{
		"tstype": "frost0",
		"tseries": [
			{
				"header": {
					"id": {
						"source": "source_%s",
						"sensorLevel": "sensorLevel_%s",
						"element": "element_%s"
					}
				},
				"observations": []
			}
		]
	}
	`, uuid, uuid, uuid)

	var statusCode int
	var respBody string

	// create the time series
	statusCode, respBody, err = testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/ts/create", dset)
	if err != nil {
		t.Errorf("testestobscommon.UploadDataset(.../ts/create) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("(.../ts/create): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
	}

	// delete the time series
	statusCode, respBody, err = testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/ts/delete", dset)
	if err != nil {
		t.Errorf("testestobscommon.UploadDataset(.../ts/delete) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("(.../ts/delete): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
	}
}

// Tests deletion of an existent time series with observations.
func testDeleteExistingTimeSeriesWithObs(t *testing.T, urlBase string) {
	var err error

	// create dataset for a new time series (guaranteed not to exist already!)
	buuid, err := exec.Command("uuidgen").Output()
	if err != nil {
		t.Errorf("exec.Command(\"uuidgen\") failed: %v", err)
		return
	}
	uuid := strings.TrimSpace(string(buuid))
	dset := fmt.Sprintf(`{
		"tstype": "frost0",
		"tseries": [
			{
				"header": {
					"id": {
						"source": "source_%s",
						"sensorLevel": "sensorLevel_%s",
						"element": "element_%s"
					}
				},
				"observations": []
			}
		]
	}
	`, uuid, uuid, uuid)

	var statusCode int
	var respBody string

	// create the time series
	statusCode, respBody, err = testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/ts/create", dset)
	if err != nil {
		t.Errorf("testestobscommon.UploadDataset(.../ts/create) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("(.../ts/create): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
	}

	// delete the time series
	statusCode, respBody, err = testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/ts/delete", dset)
	if err != nil {
		t.Errorf("testestobscommon.UploadDataset(.../ts/delete) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("(.../ts/delete): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
	}
}

// Test tests the /ts/delete endpoint in different scenarios for time series type 'frost0'.
func Test(t *testing.T, urlBase string, internalFrost bool) {
	t.Run("delete with malformed dataset", func(t *testing.T) {
		testDeleteWithMalformedDataset(t, urlBase)
	})
	t.Run("delete no time series", func(t *testing.T) {
		testDeleteNoTimeSeries(t, urlBase)
	})
	t.Run("delete non-existing time series", func(t *testing.T) {
		testDeleteNonExistingTimeSeries(t, urlBase)
	})
	t.Run("delete existing time series without observations", func(t *testing.T) {
		testDeleteExistingTimeSeriesWithoutObs(t, urlBase)
	})
	t.Run("delete existing time series with observations", func(t *testing.T) {
		testDeleteExistingTimeSeriesWithObs(t, urlBase)
	})
}
