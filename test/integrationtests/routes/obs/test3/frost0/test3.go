package testobstest3frost0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	testobscommon "gitlab.met.no/frost/frost/test/integrationtests/routes/obs/common"
)

type id struct {
	Source      string `json:"source"`
	SensorLevel string `json:"sensorLevel"`
	Element     string `json:"element"`
}

type header struct {
	ID    id          `json:"id"`
	Extra interface{} `json:"extra"`
}

type pos struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

type body struct {
	Pos     pos    `json:"pos"`
	Quality string `json:"quality"`
	Value   string `json:"value"`
}

type observation struct {
	Time string `json:"time"`
	Body *body  `json:"body"` // declare as pointer to allow 'null' in JSON representation
}

type tseries struct {
	Header       header        `json:"header"`
	Observations []observation `json:"observations"`
}

type dataset struct {
	Tstype  string    `json:"tstype"`
	Tseries []tseries `json:"tseries"`
}

func createNewUniqueTimeSeries(urlBase string) (id, error) {
	buuid, err := exec.Command("uuidgen").Output()
	if err != nil {
		return id{}, fmt.Errorf("exec.Command(\"uuidgen\") failed: %v", err)
	}
	uuid := strings.TrimSpace(string(buuid))

	tsid := id{
		Source:      fmt.Sprintf("source_%s", uuid),
		SensorLevel: fmt.Sprintf("sensorLevel_%s", uuid),
		Element:     fmt.Sprintf("element_%s", uuid),
	}

	dset := dataset{
		Tstype:  "frost0",
		Tseries: []tseries{{Header: header{ID: tsid}, Observations: []observation{}}},
	}
	mdset, err := json.Marshal(dset)
	if err != nil {
		return id{}, fmt.Errorf("json.Marshal() failed: %v", err)
	}

	var statusCode int
	var respBody string

	statusCode, respBody, err = testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/ts/create", string(mdset))
	if err != nil {
		return id{}, fmt.Errorf("testobscommon.UploadDataset(.../ts/create) failed: %v", err)
	}

	if statusCode != http.StatusOK {
		return id{}, fmt.Errorf(fmt.Sprintf("(.../ts/create): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
	}

	return tsid, nil
}

// Tests deletion of individual observations.
func testDeleteIndividualObservations(t *testing.T, urlBase string) {
	tsType := "frost0"

	// STEP 1: create dataset for a new time series (guaranteed not to exist already!)
	tsid, err := createNewUniqueTimeSeries(urlBase)
	if err != nil {
		t.Errorf(fmt.Sprintf("[STEP 1] createNewUniqueTimeSeries() failed: %v", err))
		return
	}

	// STEP 2: store a few observations in the new time series
	observations := []observation{
		{
			Time: "2020-01-01T00:00:00Z",
			Body: &body{Pos: pos{Lon: "10.0", Lat: "20.0"}, Value: "0.0", Quality: ""},
		},
		{
			Time: "2020-01-01T00:00:01Z",
			Body: &body{Pos: pos{Lon: "10.1", Lat: "20.1"}, Value: "0.1", Quality: ""},
		},
		{
			Time: "2020-01-01T00:00:02Z",
			Body: &body{Pos: pos{Lon: "10.2", Lat: "20.2"}, Value: "0.2", Quality: ""},
		},
	}

	dset := dataset{
		Tstype:  tsType,
		Tseries: []tseries{{Header: header{ID: tsid}, Observations: observations}},
	}
	mdset, err := json.Marshal(dset)
	if err != nil {
		t.Errorf("[STEP 2] json.Marshal() failed: %v", err)
		return
	}

	statusCode, respBody, err := testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/put", string(mdset))
	if err != nil {
		t.Errorf("[STEP 2] testobscommon.UploadDataset(.../put) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("[STEP 2] (.../put): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
		return
	}

	// STEP 3: verify that we can retrieve the same observations
	statusCode, respBody, err = testobscommon.DownloadDataset(
		urlBase, "api/v1/obs/frost0/get", fmt.Sprintf(
			"time=2020-01-01T00:00:00Z/2020-01-02T00:00:00Z&incobs=true"+
				"&sources=%s&sensorlevels=%s&elements=%s",
			tsid.Source, tsid.SensorLevel, tsid.Element))
	if err != nil {
		t.Errorf("[STEP 3] testobscommon.DownloadDataset(.../get) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("[STEP 3] (.../get): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
		return
	}

	var respObj struct {
		Data dataset `json:"data"`
	}
	err = json.Unmarshal([]byte(respBody), &respObj)
	if err != nil {
		t.Errorf("[STEP 3] json.Unmarshal() failed: %v", err)
		return
	}

	data := respObj.Data
	if data.Tstype != "frost0" {
		t.Errorf("[STEP 3] data.Tstype (%s) != frost0", data.Tstype)
		return
	}

	if len(data.Tseries) != 1 {
		t.Errorf("[STEP 3] len(data.Tseries) (%d) != 1", len(data.Tseries))
		return
	}

	hdr := data.Tseries[0].Header
	if hdr.ID.Source != tsid.Source {
		t.Errorf("[STEP 3] header.ID.Source (%s) != tsid.Source (%s)", hdr.ID.Source, tsid.Source)
		return
	}
	if hdr.ID.SensorLevel != tsid.SensorLevel {
		t.Errorf("[STEP 3] header.ID.SensorLevel (%s) != tsid.SensorLevel (%s)",
			hdr.ID.SensorLevel, tsid.SensorLevel)
		return
	}
	if hdr.ID.Element != tsid.Element {
		t.Errorf("[STEP 3] header.ID.Element (%s) != tsid.Element (%s)",
			hdr.ID.Element, tsid.Element)
		return
	}

	obs := data.Tseries[0].Observations
	if len(obs) != 3 {
		t.Errorf("[STEP 3] len(observations) (%d) != 3", len(obs))
		return
	}
	if obs[0].Body.Value != "0.0" {
		t.Errorf("[STEP 3] obs[0].Body.Value (%s) != \"0.0\"", obs[0].Body.Value)
		return
	}
	if obs[1].Body.Value != "0.1" {
		t.Errorf("[STEP 3] obs[1].Body.Value (%s) != \"0.1\"", obs[1].Body.Value)
		return
	}
	if obs[2].Body.Value != "0.2" {
		t.Errorf("[STEP 3] obs[2].Body.Value (%s) != \"0.2\"", obs[2].Body.Value)
		return
	}

	// STEP 4: upload a dataset that deletes two observations (one already existing) and modifies
	// the value of an existing observation
	observations = []observation{
		{
			Time: "2020-01-01T00:00:00Z",
			Body: &body{Pos: pos{Lon: "50.0", Lat: "60.0"}, Value: "20.0", Quality: ""},
		},
		{
			Time: "2020-01-01T00:00:01Z",
			Body: nil,
		},
		{
			Time: "2020-01-01T00:00:02Z",
			Body: &body{Pos: pos{Lon: "10.2", Lat: "20.2"}, Value: "0.2", Quality: ""},
		},
	}

	dset = dataset{
		Tstype:  tsType,
		Tseries: []tseries{{Header: header{ID: tsid}, Observations: observations}},
	}
	mdset, err = json.Marshal(dset)
	if err != nil {
		t.Errorf("[STEP 4] json.Marshal() failed: %v", err)
		return
	}

	statusCode, respBody, err = testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/put", string(mdset))
	if err != nil {
		t.Errorf("[STEP 4] testobscommon.UploadDataset(.../put) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("[STEP 4] (.../put): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
		return
	}

	// STEP 5: verify that the modifications are reflected in the downloaded dataset
	statusCode, respBody, err = testobscommon.DownloadDataset(
		urlBase, "api/v1/obs/frost0/get", fmt.Sprintf(
			"time=2020-01-01T00:00:00Z/2020-01-02T00:00:00Z&incobs=true"+
				"&sources=%s&sensorlevels=%s&elements=%s",
			tsid.Source, tsid.SensorLevel, tsid.Element))
	if err != nil {
		t.Errorf("[STEP 5] testobscommon.DownloadDataset(.../get) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("[STEP 5] (.../get): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
		return
	}

	err = json.Unmarshal([]byte(respBody), &respObj)
	if err != nil {
		t.Errorf("[STEP 5] json.Unmarshal() failed: %v", err)
		return
	}

	data = respObj.Data

	if len(data.Tseries) != 1 {
		t.Errorf("[STEP 5] len(data.Tseries) (%d) != 1", len(data.Tseries))
		return
	}

	obs = data.Tseries[0].Observations
	if len(obs) != 2 {
		t.Errorf("[STEP 5] len(observations) (%d) != 2", len(obs))
		return
	}
	if obs[0].Body.Value != "20.0" {
		t.Errorf("[STEP 5] obs[0].Body.Value (%s) != \"20.0\"", obs[0].Body.Value)
		return
	}
	if obs[1].Body.Value != "0.2" {
		t.Errorf("[STEP 5] obs[1].Body.Value (%s) != \"0.2\"", obs[1].Body.Value)
		return
	}
}

// Tests chronological retrieval.
func testGetObservationsSortedOnTime(t *testing.T, urlBase string) {
	// STEP 1: create dataset for a new time series (guaranteed not to exist already!)
	tsid, err := createNewUniqueTimeSeries(urlBase)
	if err != nil {
		t.Errorf(fmt.Sprintf("[STEP 1] createNewUniqueTimeSeries() failed: %v", err))
		return
	}

	// mtsid, err := json.Marshal(tsid)
	// if err != nil {
	// 	t.Errorf("[STEP 1] json.Marshal() failed: %v", err)
	// 	return
	// }
	// _ = mtsid

	// STEP 2: store 9 observations in the new time series, and ensure they are given
	// in non-chronological order
	observations := []observation{
		{Time: "2020-01-01T00:00:06Z", Body: &body{Value: "0.6"}},
		{Time: "2020-01-01T00:00:00Z", Body: &body{Value: "0.0"}},
		{Time: "2020-01-01T00:00:04Z", Body: &body{Value: "0.4"}},
		{Time: "2020-01-01T00:00:05Z", Body: &body{Value: "0.5"}},
		{Time: "2020-01-01T00:00:07Z", Body: &body{Value: "0.7"}},
		{Time: "2020-01-01T00:00:01Z", Body: &body{Value: "0.1"}},
		{Time: "2020-01-01T00:00:03Z", Body: &body{Value: "0.3"}},
		{Time: "2020-01-01T00:00:02Z", Body: &body{Value: "0.2"}},
		{Time: "2020-01-01T00:00:08Z", Body: &body{Value: "0.8"}},
	}

	dset := dataset{
		Tstype:  "frost0",
		Tseries: []tseries{{Header: header{ID: tsid}, Observations: observations}},
	}
	mdset, err := json.Marshal(dset)
	if err != nil {
		t.Errorf("[STEP 2] json.Marshal() failed: %v", err)
		return
	}

	statusCode, respBody, err := testobscommon.UploadDataset(
		urlBase, "api/v1/obs/frost0/put", string(mdset))
	if err != nil {
		t.Errorf("[STEP 2] testobscommon.UploadDataset(.../put) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("[STEP 2] (.../put): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
		return
	}

	// STEP 3: verify that we can retrieve the same observations in chronological order
	statusCode, respBody, err = testobscommon.DownloadDataset(
		urlBase, "api/v1/obs/frost0/get", fmt.Sprintf(
			"time=2020-01-01T00:00:00Z/2020-01-02T00:00:00Z&incobs=true"+
				"&sources=%s&sensorlevels=%s&elements=%s",
			tsid.Source, tsid.SensorLevel, tsid.Element))
	if err != nil {
		t.Errorf("[STEP 3] testobscommon.DownloadDataset(.../get) failed: %v", err)
		return
	}
	if statusCode != http.StatusOK {
		t.Errorf(fmt.Sprintf("[STEP 3] (.../get): expected status code %d, got %d\n"+
			"    response body: %s\n", http.StatusOK, statusCode, respBody))
		return
	}

	var respObj struct {
		Data dataset `json:"data"`
	}
	err = json.Unmarshal([]byte(respBody), &respObj)
	if err != nil {
		t.Errorf("[STEP 3] json.Unmarshal() failed: %v", err)
		return
	}

	data := respObj.Data
	if data.Tstype != "frost0" {
		t.Errorf("[STEP 3] data.Tstype (%s) != frost0", data.Tstype)
		return
	}

	if len(data.Tseries) != 1 {
		t.Errorf("[STEP 3] len(data.Tseries) (%d) != 1", len(data.Tseries))
		return
	}

	obs := data.Tseries[0].Observations
	if len(obs) != 9 {
		t.Errorf("[STEP 3] len(observations) (%d) != 9", len(obs))
		return
	}
	for i := 0; i < 9; i++ {
		actTime := obs[i].Time
		expTime := fmt.Sprintf("2020-01-01T00:00:0%dZ", i)
		if actTime != expTime {
			t.Errorf("[STEP 3] got obs time %s; expected %s", actTime, expTime)
		}

		actValue := obs[i].Body.Value
		expValue := fmt.Sprintf("0.%d", i)
		if actValue != expValue {
			t.Errorf("[STEP 3] got obs value %s; expected %s", actValue, expValue)
		}
	}
}

// Test tests various scenarios.
func Test(t *testing.T, urlBase string, internalFrost bool) {
	t.Run("delete individual observations", func(t *testing.T) {
		testDeleteIndividualObservations(t, urlBase)
	})

	t.Run("retrieve observations in chronological order", func(t *testing.T) {
		testGetObservationsSortedOnTime(t, urlBase)
	})

	// TODO: add more tests ...
}
