package testobstest4frost0

import (
	"fmt"
	"net/http"
	"testing"

	testobscommon "gitlab.met.no/frost/frost/test/integrationtests/routes/obs/common"
)

// Tests accessing an array of objects.
func testArrayOfObjects(t *testing.T, urlBase string) {

	// STEP 1: /ts/create a time series with an array of objects (organization history)
	dset := `{
		"tstype": "frost0",
		"tseries": [
			{
				"header": {
					"id": {
						"source": "source0",
						"sensorlevel": "sensorlevel0",
						"element": "element0"
					},
					"extra": {
						"organizations": [
							{
								"name": "org-name-0",
								"from": "org-from-0",
								"to": "org-to-0"
							},
							{
								"name": "org-name-1",
								"from": "org-from-1",
								"to": "org-to-1"
							}
						]
					}
				},
				"observations": []
			}
		]
	}`

	statusCode, respBody, err := testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/ts/create", dset)
	if err != nil {
		t.Errorf("testobscommon.UploadDataset(.../ts/create) failed: %v", err)
		return
	}

	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("(.../ts/create): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
	}

	// more steps ...
}

// Test tests various scenarios.
func Test(t *testing.T, urlBase string, internalFrost bool) {
	t.Run("test array of objects (organization history)", func(t *testing.T) {
		testArrayOfObjects(t, urlBase)
	})

	// TODO: add more tests ...
}
