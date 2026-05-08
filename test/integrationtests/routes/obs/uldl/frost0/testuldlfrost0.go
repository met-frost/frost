package testobsuldlfrost0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// Warning: the below layout needs to have that value exactly, otherwise it won't work!
// See https://www.pauladamsmith.com/blog/2011/05/go_time.html
var iso8601layout = "2006-01-02T15:04:05Z"

func iso8601ToTime(s string) (time.Time, error) {
	tm, err := time.Parse(iso8601layout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("time.Parse(%s) failed: %v", s, err)
	}
	return tm, nil
}

type pos struct {
	Lon float64 `json:"lon"`
	Lat float64 `json:"lat"`
}

type tsID struct {
	Source      string `json:"source"`
	SensorLevel string `json:"sensorLevel"`
	Element     string `json:"element"`
}

type tsExtra struct {
	Organizations []string `json:"organizations"`
	Pos           *pos     `json:"pos,omitempty"`
}

type tsHeader struct {
	ID    tsID    `json:"id"`
	Extra tsExtra `json:"extra"`
}

type obsBody struct {
	Pos     *pos   `json:"pos,omitempty"` // omit from serialization if nil
	Value   string `json:"value"`
	Quality string `json:"quality,omitempty"` // omit from serialization if ""
}

type observation struct {
	Time time.Time `json:"time"`
	Body obsBody   `json:"body"`
}

type singleTimeSeries struct {
	Header       tsHeader      `json:"header"`
	Observations []observation `json:"observations"`
}

type dataset struct {
	Tstype  string             `json:"tstype"`
	Tseries []singleTimeSeries `json:"tseries"`
}

type responseBody struct {
	Data     dataset `json:"data"`
	PTSIndex string  `json:"ptsindex"`
	PTime    string  `json:"ptime"`
}

func (dset *dataset) getTimeRangePlusEpsilon() (string, error) {
	var tlo, thi time.Time

	tlo = time.Time{}
	thi = time.Time{}

	for _, sts := range dset.Tseries {
		for _, val := range sts.Observations {
			t := val.Time
			if tlo == (time.Time{}) { // note parentheses to avoid parsing ambiguity!
				tlo = t
				thi = t
			} else if t.Before(tlo) {
				tlo = t
			} else if t.After(thi) {
				thi = t
			}
		}
	}

	if tlo == (time.Time{}) { // note parentheses to avoid parsing ambiguity!
		return "", fmt.Errorf("no times found")
	}

	// add the smallest possible time difference (epsilon) to ensure that the
	// resulting trange can be used as an open-ended interval, i.e. [tlo, thi + 1 sec>
	// includes all times in [tlo, thi]
	thi = thi.Add(time.Second)

	trange := fmt.Sprintf("%s/%s", tlo.Format(iso8601layout), thi.Format(iso8601layout))

	return trange, nil
}

// Tests that we're able to clear all data from the storage backend.
func clear(t *testing.T, urlBase string) {
	url := fmt.Sprintf("%s/api/v1/obs/clear", urlBase)
	resp, err := http.Get(url)
	if err != nil {
		t.Errorf("failed to send GET request: %v", err)
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("reading response body from GET request failed: %v", err)
		return
	}

	if resp.StatusCode != 200 {
		t.Errorf(
			"request failed with status code %d; body: %s",
			resp.StatusCode, respBody)
		return
	}
}

// Tests that we're able to upload a dataset of time series type 'frost0'.
func upload(t *testing.T, urlBase, path string, dset dataset) {

	jsonData, err := json.Marshal(dset)
	if err != nil {
		t.Errorf("failed to serialize dataset: %v", err)
		return
	}

	var requestBody bytes.Buffer

	_, err = requestBody.Write(jsonData)
	if err != nil {
		t.Errorf("requestBody.Write() failed: %v", err)
	}

	tstype := "frost0"
	url := fmt.Sprintf("%s/api/v1/obs/%s/%s", urlBase, tstype, path)
	req, err := http.NewRequest("POST", url, &requestBody)
	if err != nil {
		t.Errorf("failed to create POST request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("failed to send POST request: %v", err)
		return
	}

	if resp.StatusCode != 200 {
		t.Errorf("non-200 status code from POST request: %d", resp.StatusCode)
		return
	}

	if resp == nil {
		t.Errorf("empty response from POST request")
		return
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	log.Printf("response from POST request: %v", result)

	fmt.Printf("test passed\n")
}

// Tests that we're able to download (the equivalent of) dset of time series of type 'frost0'.
func download(t *testing.T, urlBase string, dset dataset) {

	equal := func(dset1, dset2 *dataset) bool {

		if dset1.Tstype != dset2.Tstype {
			return false
		}

		if len(dset1.Tseries) != len(dset2.Tseries) {
			return false
		}

		for i := 0; i < len(dset1.Tseries); i++ {
			sts1 := dset1.Tseries[i]
			sts2 := dset2.Tseries[i]

			if sts1.Header.ID != sts2.Header.ID {
				return false
			}

			// NOTE: in this case the sts1.Header.Extra is allowed to differ from
			// sts2.Header.Extra !

			if !cmp.Equal(sts1.Observations, sts2.Observations) {
				return false
			}
		}

		return true // no significant differences found
	}

	tstype := dset.Tstype
	if len(dset.Tseries) != 1 {
		t.Errorf("len(dset.Tseries) == %d; expected 1", len(dset.Tseries))
		return
	}

	trange, err := dset.getTimeRangePlusEpsilon()
	if err != nil {
		t.Errorf("failed to get time range: %v", err)
		return
	}

	url := fmt.Sprintf(
		"%s/api/v1/obs/%s/get?time=%s&incobs=true",
		urlBase, tstype, trange)
	resp, err := http.Get(url)
	if err != nil {
		t.Errorf("failed to send GET request: %v", err)
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("reading response body from GET request failed: %v", err)
		return
	}

	if resp.StatusCode != 200 {
		t.Errorf(
			"request failed with status code %d; body: %s",
			resp.StatusCode, respBody)
		return
	}

	var rbody responseBody
	err = json.Unmarshal(respBody, &rbody)
	if err != nil {
		t.Errorf("failed to deserialize response body: %v", err)
		return
	}

	dset2 := rbody.Data

	if !equal(&dset, &dset2) {
		t.Errorf("dataset in response body differs from uploaded dataset")
		t.Errorf("dataset in response body:\n%+v\n", dset2)
		t.Errorf("uploaded dataset:\n%+v\n", dset)
		return
	}

	fmt.Printf("test passed\n")
}

func createFrost0Dataset() (dataset, error) {

	// create an arbitrary time series ID+extra of type 'frost0'
	tsid := tsID{
		Source:      "dummy-source",
		SensorLevel: "dummy-sensorLevel",
		Element:     "dummy-element",
	}
	tsextra := tsExtra{
		Organizations: []string{},
		Pos:           nil,
	}

	observations := []observation{}
	for i := 0; i < 3; i++ {
		t, err := iso8601ToTime(fmt.Sprintf("2020-01-01T0%d:00:00Z", i))
		if err != nil {
			return dataset{}, fmt.Errorf("iso8601ToTime() failed: %v", err)
		}
		v := fmt.Sprintf("%d", i)
		observations = append(observations, observation{
			Time: t,
			Body: obsBody{
				Value: v,
			},
		})
	}

	dset := dataset{
		Tstype: "frost0",
		Tseries: []singleTimeSeries{
			{
				Header: tsHeader{
					ID:    tsid,
					Extra: tsextra,
				},
				Observations: observations,
			},
		},
	}

	return dset, nil
}

// emptyObservations returns a version of dset where the observations part is replaced with
// an empty array.
func emptyObservations(dset dataset) dataset {
	dset2 := dataset{
		Tstype:  dset.Tstype,
		Tseries: make([]singleTimeSeries, len(dset.Tseries)),
	}
	for i, ts := range dset.Tseries {
		dset2.Tseries[i] = singleTimeSeries{Header: ts.Header, Observations: []observation{}}
	}

	return dset2
}

// Test tests uploading and downloading a dataset of time series type 'frost0'.
func Test(t *testing.T, urlBase string, internalFrost bool) {

	// create an arbitrary dataset of time series type 'frost0'
	dset, err := createFrost0Dataset()
	if err != nil {
		t.Errorf("failed to create dataset: %v", err)
		return
	}

	// attempt to successfully upload and download the dataset
	t.Run("clear", func(t *testing.T) { clear(t, urlBase) })
	t.Run("create time series", func(t *testing.T) {
		upload(t, urlBase, "ts/create", emptyObservations(dset))
	})
	t.Run("write observations", func(t *testing.T) { upload(t, urlBase, "put", dset) })
	t.Run("read observations", func(t *testing.T) { download(t, urlBase, dset) })
}
